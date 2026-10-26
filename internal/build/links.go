package build

import (
	"fmt"
	"image"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/markdown"
	"github.com/Pinta365/tryck/internal/render"
)

// linkResolver rewrites Markdown link and image destinations to final
// site URLs.
//
// Relative destinations resolve against the source file's directory,
// the way an editor or GitHub shows them: "../setup.md#x" in
// content/guide/intro.md points at content/setup.md. Then:
//
//   - a Markdown file becomes its page URL (honoring slug),
//   - a file in contentDir becomes its copied asset path,
//   - a directory becomes its section URL,
//   - a name without extension becomes a page if name.md exists,
//   - anything else becomes the clean path it would have.
//
// Root-absolute destinations are site paths ("/docs/setup/"), except
// that a "/x.md" path is looked up as a source file. Every internal
// destination is finally prefixed with baseUrl's path. External links
// and same-page "#fragment" links are left alone.
//
// Each version has its own resolver: paths are relative to the
// version's content directory, and its URLs carry its prefix. In an
// older version, a root-absolute path to one of its own pages gets the
// prefix; other root-absolute paths (shared static files) stay as they
// are.
type linkResolver struct {
	baseURL string
	prefix  string            // the version's URL prefix, "/" for the current one
	pages   map[string]string // source path relative to the content directory -> URL
	assets  map[string]string // asset source path -> output path
	urls    map[string]bool   // the version's page URLs
	files   map[string]bool   // the version's asset output paths
	// With images set, Markdown images are looked up as files: an asset
	// (srcs) or a file in a static directory, later directories first.
	// With attrs they get width, height and loading="lazy" from it, and
	// with resize a srcset of smaller copies.
	images *images
	attrs  bool
	resize bool
	srcs   map[string]string // asset output path -> file on disk
	static []string
}

func (r *linkResolver) resolve(sourcePath, dest string) (string, bool) {
	if dest == "" || strings.HasPrefix(dest, "#") {
		return dest, false
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" {
		return dest, false // external, or only a query on the same page
	}
	var site string
	if strings.HasPrefix(u.Path, "/") {
		site = u.Path
		if strings.HasSuffix(site, ".md") {
			site = r.page(strings.TrimPrefix(path.Clean(site), "/"))
		} else if v := versionPath(r.prefix, site); r.urls[v] || r.files[strings.TrimPrefix(v, "/")] {
			site = v
		}
	} else {
		rel := path.Join(path.Dir(sourcePath), u.Path)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return dest, false // escapes contentDir; link checking reports it
		}
		site = r.relative(rel, strings.HasSuffix(u.Path, "/"))
	}
	out := render.RelURL(r.baseURL, (&url.URL{Path: site}).EscapedPath())
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		out += "#" + u.EscapedFragment()
	}
	return out, true
}

// relative maps a cleaned path relative to contentDir to a site path.
func (r *linkResolver) relative(rel string, dir bool) string {
	if rel == "." {
		rel = ""
	}
	switch {
	case strings.HasSuffix(rel, ".md"):
		return r.page(rel)
	case r.assets[rel] != "":
		return "/" + r.assets[rel]
	case !dir && path.Ext(rel) == "" && r.pages[rel+".md"] != "":
		return r.pages[rel+".md"]
	}
	for _, index := range []string{"_index.md", "index.md"} {
		if u := r.pages[path.Join(rel, index)]; u != "" {
			return u
		}
	}
	if dir || path.Ext(rel) == "" {
		return versionPath(r.prefix, content.URLFor(path.Join(rel, "_index.md"), ""))
	}
	return versionPath(r.prefix, "/"+content.AssetPath(rel))
}

// page returns the URL of a Markdown source, or the URL it would have.
func (r *linkResolver) page(rel string) string {
	if u, ok := r.pages[rel]; ok {
		return u
	}
	return versionPath(r.prefix, content.URLFor(rel, ""))
}

// collectLinks lists every link, image and directive link attribute in
// doc with its destination as written in the source.
func collectLinks(doc ast.Node) []linkRef {
	var refs []linkRef
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch l := n.(type) {
		case *ast.Link:
			refs = append(refs, linkRef{node: n, dest: string(l.Destination)})
		case *ast.Image:
			refs = append(refs, linkRef{node: n, dest: string(l.Destination)})
		case *markdown.GenericDirective:
			for _, attr := range markdown.LinkAttrs {
				if dest, ok := l.Link(attr); ok {
					refs = append(refs, linkRef{node: n, attr: attr, dest: dest})
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return refs
}

// rewrite sets each link's destination from its original one, so it can
// be applied again to a cached AST.
func (r *linkResolver) rewrite(sourcePath string, refs []linkRef) {
	for _, ref := range refs {
		dest, _ := r.resolve(sourcePath, ref.dest)
		switch l := ref.node.(type) {
		case *ast.Link:
			l.Destination = []byte(dest)
		case *ast.Image:
			l.Destination = []byte(dest)
			if r.images != nil {
				r.image(l, sourcePath, ref.dest)
			}
		case *markdown.GenericDirective:
			l.SetAttributeString(ref.attr, []byte(dest))
		}
	}
}

// image sets the attributes of a Markdown image whose destination, as
// written, is dest.
func (r *linkResolver) image(img *ast.Image, sourcePath, dest string) {
	file, out := r.file(sourcePath, dest)
	if file == "" {
		setImageAttrs(img, image.Point{}, r.attrs, "", "")
		return
	}
	f := r.images.file(file)
	var srcset string
	if r.resize && f.size.X > 0 {
		taken := func(p string) bool { return r.files[p] || r.staticFile(p) != "" }
		if copies := r.images.resized(file, out, f, taken); len(copies) > 0 {
			var b strings.Builder
			for _, c := range copies {
				fmt.Fprintf(&b, "%s %dw, ", r.srcsetURL(c.out), c.width)
			}
			fmt.Fprintf(&b, "%s %dw", r.srcsetURL(out), f.size.X)
			srcset = b.String()
		}
	}
	setImageAttrs(img, f.size, r.attrs, srcset, r.images.cfg.Sizes)
}

// srcsetURL is the URL of an output path in a srcset, where a comma
// would end the candidate.
func (r *linkResolver) srcsetURL(out string) string {
	u := render.RelURL(r.baseURL, (&url.URL{Path: "/" + out}).EscapedPath())
	return strings.ReplaceAll(u, ",", "%2C")
}

// file returns the file on disk that a local destination points at and
// its output path, or "" if it is not a known asset or static file.
func (r *linkResolver) file(sourcePath, dest string) (file, out string) {
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Opaque != "" || u.Path == "" {
		return "", ""
	}
	if !strings.HasPrefix(u.Path, "/") {
		out = r.assets[path.Join(path.Dir(sourcePath), u.Path)]
		return r.srcs[out], out
	}
	out = strings.TrimPrefix(versionPath(r.prefix, u.Path), "/")
	if src := r.srcs[out]; src != "" {
		return src, out
	}
	out = strings.TrimPrefix(path.Clean(u.Path), "/")
	if f := r.staticFile(out); f != "" {
		return f, out
	}
	return "", ""
}

// staticFile returns the file in the static directories served at out,
// or "".
func (r *linkResolver) staticFile(out string) string {
	for _, dir := range slices.Backward(r.static) {
		f := filepath.Join(dir, filepath.FromSlash(out))
		if fi, err := os.Stat(f); err == nil && fi.Mode().IsRegular() {
			return f
		}
	}
	return ""
}
