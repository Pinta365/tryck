// Package render loads layout templates and executes them for pages.
package render

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Pinta365/tryck/internal/diag"
)

//go:embed layouts
var embedded embed.FS

// SiteInfo is the site-wide data available to templates.
type SiteInfo struct {
	Title       string
	Description string
	BaseURL     string
	Language    string
}

// PageInfo is the per-page data available to templates.
type PageInfo struct {
	Title       string
	Description string
	URL         string
	Date        time.Time
	Draft       bool
	Params      map[string]any
}

// TemplateData is the dot value of every layout (spec §9.2).
type TemplateData struct {
	Site    SiteInfo
	Page    *PageInfo
	Content template.HTML // rendered body, already trusted
	Dev     bool
}

// ErrMissingLayout is returned by Execute for an unknown layout name.
var ErrMissingLayout = errors.New("layout not found")

// Templates holds one executable template set per layout.
type Templates struct {
	layouts map[string]*template.Template
}

type layoutFile struct {
	name    string // slash path relative to the layouts root, e.g. "partials/header.html"
	label   string // for diagnostics: project-relative path or "(embedded)/..."
	content string
}

// Load parses the embedded default layouts, overridden file by file by
// those in userDir (which may not exist). label is how userDir is named
// in diagnostics, relative to the project root.
func Load(userDir, label string, funcs template.FuncMap) (*Templates, []diag.Diagnostic) {
	files := map[string]layoutFile{}
	var dc diag.Collector

	_ = fs.WalkDir(embedded, "layouts", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := embedded.ReadFile(p)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(p, "layouts/")
		files[name] = layoutFile{name: name, label: "(embedded)/" + name, content: string(b)}
		return nil
	})

	err := filepath.WalkDir(userDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == userDir && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		rel, err := filepath.Rel(userDir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		files[name] = layoutFile{name: name, label: path.Join(label, name), content: string(b)}
		return nil
	})
	if err != nil {
		dc.Errorf(label, 0, 0, "template.parse", "cannot read layouts: %v", err)
		return &Templates{layouts: map[string]*template.Template{}}, dc.All()
	}

	names := slices.Sorted(func(yield func(string) bool) {
		for n := range files {
			if !yield(n) {
				return
			}
		}
	})

	// The base set holds baseof.html and all partials; each layout is
	// parsed into its own clone so their block definitions do not clash.
	base := template.New("").Funcs(funcs)
	for _, n := range names {
		f := files[n]
		if n == "baseof.html" || strings.HasPrefix(n, "partials/") {
			if _, err := base.New(n).Parse(f.content); err != nil {
				dc.Add(parseDiag(f, err))
			}
		}
	}

	t := &Templates{layouts: map[string]*template.Template{}}
	for _, n := range names {
		f := files[n]
		if n == "baseof.html" || strings.Contains(n, "/") {
			continue
		}
		set, err := base.Clone()
		if err != nil {
			dc.Add(parseDiag(f, err))
			continue
		}
		if _, err := set.New(n).Parse(f.content); err != nil {
			dc.Add(parseDiag(f, err))
			continue
		}
		entry := n
		if definesMain(f.content, funcs) && set.Lookup("baseof.html") != nil {
			entry = "baseof.html"
		}
		t.layouts[strings.TrimSuffix(n, ".html")] = set.Lookup(entry)
	}
	return t, dc.All()
}

// definesMain reports whether a layout fills the "main" block of
// baseof.html, as opposed to being a complete page on its own.
func definesMain(content string, funcs template.FuncMap) bool {
	t, err := template.New("probe").Funcs(funcs).Parse(content)
	return err == nil && t.Lookup("main") != nil
}

var tmplLineRE = regexp.MustCompile(`^(?:html/)?template: [^:]+:(\d+):(?:(\d+):)? ?`)

func parseDiag(f layoutFile, err error) diag.Diagnostic {
	d := diag.Diagnostic{Severity: diag.Error, File: f.label, Code: "template.parse", Message: err.Error()}
	if m := tmplLineRE.FindStringSubmatch(err.Error()); m != nil {
		d.Line, _ = strconv.Atoi(m[1])
		d.Message = strings.TrimPrefix(err.Error(), m[0])
	}
	return d
}

// Has reports whether a layout exists.
func (t *Templates) Has(layout string) bool {
	_, ok := t.layouts[layout]
	return ok
}

// Execute renders data with the named layout (file name without .html).
func (t *Templates) Execute(w io.Writer, layout string, data *TemplateData) error {
	tmpl, ok := t.layouts[layout]
	if !ok {
		return fmt.Errorf("%w: %q", ErrMissingLayout, layout)
	}
	return tmpl.Execute(w, data)
}

// Funcs returns the template functions for a site with the given baseUrl.
func Funcs(baseURL string) template.FuncMap {
	return template.FuncMap{
		"relURL": func(p string) string { return RelURL(baseURL, p) },
		"absURL": func(p string) string { return AbsURL(baseURL, p) },
	}
}

// RelURL prefixes a site path with the path part of baseURL, so sites
// deployed under a subpath (GitHub Pages project sites) link correctly.
// Absolute URLs are returned unchanged.
func RelURL(baseURL, p string) string {
	if isAbsURL(p) {
		return p
	}
	basePath := "/"
	if u, err := url.Parse(baseURL); err == nil && u.Path != "" {
		basePath = u.Path
	}
	return strings.TrimSuffix(basePath, "/") + "/" + strings.TrimPrefix(p, "/")
}

// AbsURL is RelURL with baseURL's scheme and host. If baseURL has no
// host it behaves like RelURL.
func AbsURL(baseURL, p string) string {
	if isAbsURL(p) {
		return p
	}
	rel := RelURL(baseURL, p)
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return rel
	}
	return u.Scheme + "://" + u.Host + rel
}

func isAbsURL(p string) bool {
	u, err := url.Parse(p)
	return err == nil && u.Scheme != ""
}
