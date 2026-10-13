package build

import (
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/render"
)

// linkResolver rewrites Markdown link and image destinations to final
// site URLs (spec §5.2, §11).
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
type linkResolver struct {
	baseURL string
	pages   map[string]string // source path relative to contentDir -> URL
	assets  map[string]string // asset source path -> output path
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
		return content.URLFor(path.Join(rel, "_index.md"), "")
	}
	return "/" + content.AssetPath(rel)
}

// page returns the URL of a Markdown source, or the URL it would have.
func (r *linkResolver) page(rel string) string {
	if u, ok := r.pages[rel]; ok {
		return u
	}
	return content.URLFor(rel, "")
}

// collectLinks lists every link and image in doc with its destination
// as written in the source.
func collectLinks(doc ast.Node) []linkRef {
	var refs []linkRef
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch l := n.(type) {
		case *ast.Link:
			refs = append(refs, linkRef{n, string(l.Destination)})
		case *ast.Image:
			refs = append(refs, linkRef{n, string(l.Destination)})
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
		}
	}
}
