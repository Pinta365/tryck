// Package markdown configures goldmark: CommonMark plus GFM, unique
// heading IDs and tryck's ::: directives.
package markdown

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Options configures the Markdown processor.
type Options struct {
	// UnsafeHTML passes raw HTML in Markdown through. Otherwise it is
	// replaced by an HTML comment.
	UnsafeHTML bool
	// HeadingAnchors adds a "#" link to every heading below h1.
	HeadingAnchors bool
	// Extenders add goldmark extensions, such as syntax highlighting.
	Extenders []plugin.MarkdownExtender
	// Directives render ::: names tryck does not know, by name.
	Directives map[string]plugin.DirectiveRenderer
	// Templates render ::: names tryck does not know from templates, by
	// name.
	Templates map[string]DirectiveTemplate
	// SitePath maps root-absolute links in RenderInline to final URLs,
	// for example by adding the base path.
	SitePath func(string) string
}

// Processor parses and renders Markdown. It is safe for concurrent use.
type Processor struct {
	md        goldmark.Markdown
	custom    map[string]plugin.DirectiveRenderer
	templates map[string]DirectiveTemplate
	sitePath  func(string) string
}

// New returns a Processor configured from opts.
func New(opts Options) *Processor {
	var rendererOpts []goldmark.Option
	if opts.UnsafeHTML {
		rendererOpts = append(rendererOpts, goldmark.WithRendererOptions(html.WithUnsafe()))
	}
	md := goldmark.New(append([]goldmark.Option{
		goldmark.WithExtensions(extension.GFM, directives{custom: opts.Directives, templates: opts.Templates}),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // via ids: Slugify, dupes get -1, -2
			parser.WithAttribute(),     // {#custom-id} wins over the generated ID
		),
	}, rendererOpts...)...)
	if opts.HeadingAnchors {
		md.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(headingAnchors{}, 100)))
	}
	for _, e := range opts.Extenders {
		e.Extend(md)
	}
	return &Processor{md: md, custom: opts.Directives, templates: opts.Templates, sitePath: opts.SitePath}
}

// Issue is a problem found while parsing, located by byte offset in the
// parsed source (-1 if unknown).
type Issue struct {
	Severity diag.Severity
	Code     string
	Offset   int
	Message  string
}

// Parse parses source into a document AST and reports directive problems
// and heading IDs that needed a -N suffix.
func (p *Processor) Parse(source []byte) (ast.Node, []Issue) {
	ids := newIDs()
	ctx := parser.NewContext(parser.WithIDs(ids))
	doc := p.md.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))
	return doc, p.inspect(doc, ids.suffixed)
}

// inspect walks a parsed document for problems the parser cannot report.
func (p *Processor) inspect(doc ast.Node, suffixed []string) []Issue {
	var issues []Issue
	dup := map[string]bool{}
	for _, id := range suffixed {
		dup[id] = true
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if f, ok := n.(fenced); ok {
			d := f.directive()
			if !d.Closed {
				issues = append(issues, Issue{diag.Error, "directive.unclosed", d.Offset,
					fmt.Sprintf("%s%s directive is never closed; end it with a line of %d or more colons", strings.Repeat(":", d.Length), d.Name, d.Length)})
			}
			if d.BadAttrs != "" {
				issues = append(issues, Issue{diag.Warning, "directive.attrs", d.Offset,
					fmt.Sprintf("cannot parse attributes %s of %s%s; they are ignored", d.BadAttrs, strings.Repeat(":", d.Length), d.Name)})
			}
		}
		if t, ok := n.(*Tabs); ok {
			tabs := 0
			for c := t.FirstChild(); c != nil; c = c.NextSibling() {
				if _, ok := c.(*Tab); ok {
					tabs++
				} else if tabs == 0 {
					issues = append(issues, Issue{diag.Warning, "directive.tabContent", nodeOffset(c),
						"content in a tabs directive before the first @tab line is shown above the tabs"})
				}
			}
			if tabs == 0 {
				issues = append(issues, Issue{diag.Warning, "directive.tabContent", t.Offset,
					"tabs directive has no @tab sections"})
			}
		}
		if h, ok := n.(*ast.Heading); ok {
			if v, ok := h.AttributeString("id"); ok {
				if id, _ := v.([]byte); dup[string(id)] {
					issues = append(issues, Issue{diag.Info, "heading.duplicateId", nodeOffset(h),
						fmt.Sprintf("heading ID %q needed a suffix to be unique; add {#id} to choose one", id)})
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return issues
}

// Unknown reports directives that have no renderer. It is separate from
// Parse because the renderers can change while parsed documents are
// reused.
func (p *Processor) Unknown(doc ast.Node) []Issue {
	var issues []Issue
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if g, ok := n.(*GenericDirective); ok && entering && p.custom[g.Name] == nil && p.templates[g.Name] == nil {
			issues = append(issues, Issue{diag.Warning, "directive.unknown", g.Offset,
				fmt.Sprintf("unknown directive %q, rendered as a plain <div>", g.Name)})
		}
		return ast.WalkContinue, nil
	})
	return issues
}

// nodeOffset returns the source offset of a block node's first line, or
// of its first descendant that has lines, or -1.
func nodeOffset(n ast.Node) int {
	switch f := n.(type) {
	case fenced:
		return f.directive().Offset
	case *Tab:
		return f.Offset
	}
	if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
		return n.Lines().At(0).Start
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if o := nodeOffset(c); o >= 0 {
			return o
		}
	}
	return -1
}

func (p *Processor) rootLink(dest []byte) []byte {
	if len(dest) > 0 && dest[0] == '/' && (len(dest) == 1 || dest[1] != '/') {
		return []byte(p.sitePath(string(dest)))
	}
	return dest
}

// Render renders a document parsed from source to an HTML fragment.
func (p *Processor) Render(doc ast.Node, source []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := p.md.Renderer().Render(&buf, source, doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderInline renders a short Markdown string, such as a front matter
// description, and unwraps it if it is a single paragraph. Root-absolute
// link and image destinations ("/guide/") are passed through
// p.SitePath, when set, so they get the site's base path.
func (p *Processor) RenderInline(s string) (string, error) {
	src := []byte(s)
	doc, _ := p.Parse(src)
	if p.sitePath != nil {
		_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			switch l := n.(type) {
			case *ast.Link:
				l.Destination = p.rootLink(l.Destination)
			case *ast.Image:
				l.Destination = p.rootLink(l.Destination)
			}
			return ast.WalkContinue, nil
		})
	}
	out, err := p.Render(doc, src)
	if err != nil {
		return "", err
	}
	html := strings.TrimSpace(string(out))
	if doc.ChildCount() == 1 && doc.FirstChild().Kind() == ast.KindParagraph {
		html = strings.TrimSuffix(strings.TrimPrefix(html, "<p>"), "</p>")
	}
	return html, nil
}
