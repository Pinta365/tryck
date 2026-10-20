// Package render loads layout templates and executes them for pages.
package render

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/markdown"
	"github.com/Pinta365/tryck/pkg/plugin"
)

//go:embed layouts
var embedded embed.FS

//go:embed static
var embeddedStatic embed.FS

// Static returns the built-in theme's static files, such as
// css/tryck.css. A file with the same path in the theme's or site's
// static directory replaces it.
func Static() fs.FS {
	sub, err := fs.Sub(embeddedStatic, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

// SiteInfo is the site-wide data available to templates.
type SiteInfo = plugin.SiteInfo

// PageInfo is the per-page data available to templates.
type PageInfo = plugin.PageInfo

// TemplateData is the dot value of every layout.
type TemplateData struct {
	Site        SiteInfo
	Page        *PageInfo
	Content     template.HTML     // rendered body, already trusted
	Nav         *plugin.NavNode   // full tree with Active/InPath set for this page
	TOC         []plugin.TOCEntry // from the toc plugin, nil if disabled
	Prev, Next  *PageInfo         // nil at the ends
	Stylesheets []string          // site paths of plugin CSS to link in <head>
	Theme       ThemeData         // theme settings from the config
	SearchIndex string            // site path of the search index, "" if search is off
	Dev         bool              // true in tryck dev
}

// ThemeData is the theme section of the config, ready for templates.
type ThemeData struct {
	Accent      template.CSS  // validated CSS color, or ""
	AccentDark  template.CSS  // accent in dark mode
	Logo        string        // site path
	ColorScheme string        // auto, light or dark
	Nav         []NavLink     // header links
	Footer      template.HTML // rendered inline Markdown
	CSS         []string      // extra stylesheets
	Params      map[string]any
}

// NavLink is a header link.
type NavLink struct {
	Title string
	URL   string
}

// ErrMissingLayout is returned by Execute for an unknown layout name.
var ErrMissingLayout = errors.New("layout not found")

// Templates holds one executable template set per layout, and one per
// directive template (directives/<name>.html).
type Templates struct {
	layouts    map[string]*template.Template
	directives map[string]*template.Template
}

// DirectiveData is the dot value of a directive template.
type DirectiveData struct {
	Name    string            // the directive name
	Title   string            // from :::name[title]
	Attrs   map[string]string // from {key=value .class #id}; class and id included
	Content template.HTML     // the rendered body
	Site    SiteInfo
}

type layoutFile struct {
	name    string // slash path relative to the layouts root, e.g. "partials/header.html"
	label   string // for diagnostics: project-relative path or "(embedded)/..."
	content string
}

// Dir is a layouts directory and how it is named in diagnostics
// (relative to the project root).
type Dir struct {
	Path  string
	Label string
}

// Load parses the embedded default layouts, overridden file by file by
// those in dirs (lowest priority first: the theme's, then the site's).
// Directories that do not exist are skipped.
func Load(dirs []Dir, funcs template.FuncMap) (*Templates, []diag.Diagnostic) {
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

	for _, d := range dirs {
		if err := readLayouts(d, files); err != nil {
			dc.Errorf(d.Label, 0, 0, "template.parse", "cannot read layouts: %v", err)
			return &Templates{layouts: map[string]*template.Template{}, directives: map[string]*template.Template{}}, dc.All()
		}
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

	t := &Templates{layouts: map[string]*template.Template{}, directives: map[string]*template.Template{}}
	for _, n := range names {
		f := files[n]
		if name, ok := strings.CutPrefix(n, "directives/"); ok {
			name = strings.TrimSuffix(name, ".html")
			switch {
			case !directiveNameRE.MatchString(name):
				dc.Errorf(f.label, 0, 0, "template.parse", "%q is not a directive name: use letters, digits, - and _, starting with a letter", name)
				continue
			case markdown.IsBuiltin(name):
				dc.Errorf(f.label, 0, 0, "template.parse", "%q is a built-in directive and cannot be replaced by a template", name)
				continue
			}
			set, err := base.Clone()
			if err != nil {
				dc.Add(parseDiag(f, err))
				continue
			}
			set.Funcs(template.FuncMap{"partial": partialFunc(set)})
			if _, err := set.New(n).Parse(f.content); err != nil {
				dc.Add(parseDiag(f, err))
				continue
			}
			t.directives[name] = set.Lookup(n)
			continue
		}
		if n == "baseof.html" || strings.Contains(n, "/") {
			continue
		}
		set, err := base.Clone()
		if err != nil {
			dc.Add(parseDiag(f, err))
			continue
		}
		set.Funcs(template.FuncMap{"partial": partialFunc(set)})
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

// readLayouts adds the .html files under d to files, replacing those
// with the same name.
func readLayouts(d Dir, files map[string]layoutFile) error {
	return filepath.WalkDir(d.Path, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if p == d.Path && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if e.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		rel, err := filepath.Rel(d.Path, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		files[name] = layoutFile{name: name, label: path.Join(d.Label, name), content: string(b)}
		return nil
	})
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

// directiveNameRE matches the names the ::: syntax accepts.
var directiveNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// DirectiveNames lists the directive templates, sorted.
func (t *Templates) DirectiveNames() []string {
	return slices.Sorted(maps.Keys(t.directives))
}

// ExecuteDirective renders data with the directive template for name.
func (t *Templates) ExecuteDirective(w io.Writer, name string, data DirectiveData) error {
	tmpl, ok := t.directives[name]
	if !ok {
		return fmt.Errorf("no template for directive %q", name)
	}
	return tmpl.Execute(w, data)
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

// FuncOptions configures the template functions of a site.
type FuncOptions struct {
	BaseURL     string
	DateFormat  string                       // default layout for dateFormat
	Markdownify func(string) (string, error) // inline Markdown to HTML
}

// Funcs returns the template functions. "partial" is bound
// to each layout's template set by Load.
func Funcs(o FuncOptions) template.FuncMap {
	return template.FuncMap{
		"relURL": func(p string) string { return RelURL(o.BaseURL, p) },
		"absURL": func(p string) string { return AbsURL(o.BaseURL, p) },
		"dateFormat": func(layout string, t time.Time) string {
			if t.IsZero() {
				return ""
			}
			if layout == "" {
				layout = o.DateFormat
			}
			return t.Format(layout)
		},
		"markdownify": func(s string) (template.HTML, error) {
			if o.Markdownify == nil {
				return template.HTML(template.HTMLEscapeString(s)), nil
			}
			html, err := o.Markdownify(s)
			return template.HTML(html), err // rendered by goldmark, raw HTML stripped unless unsafeHTML
		},
		"slugify": markdown.Slugify,
		"default": func(def, val any) any {
			if isZero(val) {
				return def
			}
			return val
		},
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, errors.New("dict needs key/value pairs")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict key %v is not a string", kv[i])
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"jsonify": func(v any) (template.HTML, error) {
			b, err := json.Marshal(v) // escapes <, > and &, so safe in HTML
			return template.HTML(b), err
		},
		"partial": func(string, ...any) (template.HTML, error) {
			return "", errors.New("partial is not bound to a template set")
		},
	}
}

// partialFunc executes "partials/<name>.html" from set. The data
// argument is optional; without it the partial gets nil.
func partialFunc(set *template.Template) func(string, ...any) (template.HTML, error) {
	return func(name string, data ...any) (template.HTML, error) {
		if len(data) > 1 {
			return "", fmt.Errorf("partial %q: takes at most one data argument", name)
		}
		if !strings.HasSuffix(name, ".html") {
			name += ".html"
		}
		t := set.Lookup("partials/" + name)
		if t == nil {
			return "", fmt.Errorf("partial %q not found in partials/", name)
		}
		var arg any
		if len(data) == 1 {
			arg = data[0]
		}
		var buf strings.Builder
		if err := t.Execute(&buf, arg); err != nil {
			return "", err
		}
		return template.HTML(buf.String()), nil // output of an html/template, already escaped
	}
}

func isZero(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.String:
		return rv.Len() == 0
	}
	return rv.IsZero()
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
