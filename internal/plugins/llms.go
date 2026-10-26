package plugins

import (
	"bytes"
	"cmp"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/markdown"
	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// LLMsTxt writes /llms.txt (https://llmstxt.org): the site's title and
// description, then a list of its pages with their descriptions, one
// section per top-level folder of the navigation. With Pages, every page
// is also written as Markdown next to its URL ("/guide/setup/" ->
// "/guide/setup.md"), and llms.txt links those. With Full, llms-full.txt
// holds every page in one file.
//
// The Markdown is the page source as written, with front matter removed,
// the page title as its first line and every link and image made
// absolute. Drafts, older versions and pages under Exclude are left out;
// pages under Optional are listed in an "Optional" section, which tools
// may skip. Raw HTML is removed unless UnsafeHTML is set.
type LLMsTxt struct {
	Pages    bool     `yaml:"pages"`
	Full     bool     `yaml:"full"`
	Details  string   `yaml:"details"`
	Optional []string `yaml:"optional"`
	Exclude  []string `yaml:"exclude"`
	// UnsafeHTML keeps raw HTML, as build.unsafeHTML does for pages;
	// otherwise it is removed, like HTML comments and scripts are from
	// pages.
	UnsafeHTML bool `yaml:"-"`

	site    plugin.SiteInfo
	mu      sync.Mutex
	sources map[string]llmsSource // page URL -> source with link edits
}

// llmsSource is a page body and the changes that make it standalone.
type llmsSource struct {
	body  []byte
	edits []llmsEdit
}

// llmsEdit replaces body[start:end]. With link set, the replacement is
// pre, the final link destination of dest and post, made once all pages
// are known; otherwise the span is removed.
type llmsEdit struct {
	start, end int
	link       bool
	dest       string
	pre, post  string
}

func (*LLMsTxt) Name() string { return "llmsTxt" }
func (*LLMsTxt) ConfigKeys() []string {
	return []string{"pages", "full", "details", "optional", "exclude"}
}

func (l *LLMsTxt) Init(cfg map[string]any, site plugin.SiteInfo) error {
	*l = LLMsTxt{Pages: true, UnsafeHTML: l.UnsafeHTML, site: site, sources: map[string]llmsSource{}}
	if err := decode(cfg, l); err != nil {
		return err
	}
	for _, list := range [][]string{l.Optional, l.Exclude} {
		for i, s := range list {
			list[i] = strings.Replace("/"+strings.Trim(s, "/")+"/", "//", "/", 1)
		}
	}
	return nil
}

// OnPageParsed records where the page's links are and what they point
// to. Links have their final URLs by now.
func (l *LLMsTxt) OnPageParsed(p *plugin.Page) error {
	if p.Front.Draft || p.Version != l.site.Version || l.excluded(p.URL) {
		return nil
	}
	src := llmsSource{body: p.Source}
	if h := p.AST.FirstChild(); h != nil {
		if h, ok := h.(*ast.Heading); ok && h.Level == 1 && h.Pos() >= 0 {
			// The title is written from the page's title instead.
			src.edits = append(src.edits, llmsEdit{start: h.Pos(), end: headingEnd(p.Source, h)})
		}
	}
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.Link:
			if e, ok := linkEdit(p.Source, n, string(n.Destination), n.Title); ok {
				src.edits = append(src.edits, e)
			}
		case *ast.Image:
			if e, ok := linkEdit(p.Source, n, string(n.Destination), n.Title); ok {
				src.edits = append(src.edits, e)
			}
		case *markdown.GenericDirective:
			src.edits = append(src.edits, directiveEdits(p.Source, n)...)
		case *ast.HTMLBlock:
			if !l.UnsafeHTML && n.Lines().Len() > 0 {
				end := n.Lines().At(n.Lines().Len() - 1).Stop
				if n.HasClosure() {
					end = max(end, n.ClosureLine.Stop)
				}
				src.edits = append(src.edits, llmsEdit{start: n.Lines().At(0).Start, end: end})
			}
		case *ast.RawHTML:
			if !l.UnsafeHTML && n.Segments.Len() > 0 {
				src.edits = append(src.edits, llmsEdit{start: n.Segments.At(0).Start, end: n.Segments.At(n.Segments.Len() - 1).Stop})
			}
		}
		return ast.WalkContinue, nil
	})
	slices.SortFunc(src.edits, func(a, b llmsEdit) int { return a.start - b.start })
	l.mu.Lock()
	l.sources[p.URL] = src
	l.mu.Unlock()
	return nil
}

// has reports whether a page is included: recorded while parsing.
func (l *LLMsTxt) has(pageURL string) bool {
	_, ok := l.sources[pageURL]
	return ok
}

func (l *LLMsTxt) excluded(u string) bool   { return hasPrefix(u, l.Exclude) }
func (l *LLMsTxt) isOptional(u string) bool { return hasPrefix(u, l.Optional) }

func hasPrefix(u string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(u, p) })
}

func (l *LLMsTxt) OnBuildComplete(v plugin.SiteView, out plugin.Outputs) error {
	// Pages in navigation order, grouped by top-level section.
	type group struct {
		title string
		pages []*plugin.PageInfo
	}
	var top group // the home page and pages outside any section
	var groups []group
	var optional []*plugin.PageInfo
	seen := map[string]bool{}
	var add func(g *group, n *plugin.NavNode)
	add = func(g *group, n *plugin.NavNode) {
		if p, ok := v.PageByURL(n.URL); ok && n.URL != "" && !seen[n.URL] && l.has(n.URL) {
			seen[n.URL] = true
			if l.isOptional(n.URL) {
				optional = append(optional, p)
			} else {
				g.pages = append(g.pages, p)
			}
		}
		for _, c := range n.Children {
			add(g, c)
		}
	}
	nav := v.Nav()
	add(&top, &plugin.NavNode{URL: nav.URL})
	for _, n := range nav.Children {
		if !n.Section {
			add(&top, n)
			continue
		}
		g := group{title: n.Title}
		add(&g, n)
		if len(g.pages) > 0 {
			groups = append(groups, g)
		}
	}
	if len(top.pages) > 0 {
		groups = append([]group{{title: "Pages", pages: top.pages}}, groups...)
	}
	if len(optional) > 0 {
		groups = append(groups, group{title: "Optional", pages: optional})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", cmp.Or(l.site.Title, "Documentation"))
	if d := oneLine(l.site.Description); d != "" {
		fmt.Fprintf(&b, "\n> %s\n", d)
	}
	if d := strings.TrimSpace(l.Details); d != "" {
		fmt.Fprintf(&b, "\n%s\n", d)
	}
	for _, g := range groups {
		fmt.Fprintf(&b, "\n## %s\n\n", g.title)
		for _, p := range g.pages {
			fmt.Fprintf(&b, "- [%s](%s)", escapeLinkText(p.Title), l.pageLink(p.URL))
			if d := oneLine(p.Description); d != "" {
				fmt.Fprintf(&b, ": %s", d)
			}
			b.WriteByte('\n')
		}
	}
	if err := out.Add("llms.txt", []byte(b.String())); err != nil {
		return err
	}

	var full strings.Builder
	if l.Full {
		fmt.Fprintf(&full, "# %s\n", cmp.Or(l.site.Title, "Documentation"))
		if d := oneLine(l.site.Description); d != "" {
			fmt.Fprintf(&full, "\n> %s\n", d)
		}
	}
	for _, g := range groups {
		for _, p := range g.pages {
			md := l.markdown(v, p)
			if l.Pages {
				if err := out.Add(mdPath(p.URL), md); err != nil {
					return err
				}
			}
			if l.Full {
				fmt.Fprintf(&full, "\n---\n\nSource: %s\n\n", render.AbsURL(l.site.BaseURL, p.URL))
				full.Write(md)
			}
		}
	}
	if l.Full {
		return out.Add("llms-full.txt", []byte(full.String()))
	}
	return nil
}

// markdown returns a page as standalone Markdown.
func (l *LLMsTxt) markdown(v plugin.SiteView, p *plugin.PageInfo) []byte {
	src := l.sources[p.URL]
	var body bytes.Buffer
	last := 0
	for _, e := range src.edits {
		if e.start < last {
			continue // inside an earlier edit
		}
		body.Write(src.body[last:e.start])
		if e.link {
			body.WriteString(e.pre + l.destination(v, p.URL, e.dest) + e.post)
		}
		last = e.end
	}
	body.Write(src.body[last:])
	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s\n\n", p.Title)
	if d := oneLine(p.Description); d != "" {
		fmt.Fprintf(&b, "> %s\n\n", d)
	}
	b.Write(bytes.TrimSpace(body.Bytes()))
	return append(bytes.TrimSpace(b.Bytes()), '\n')
}

// destination makes a final link destination absolute. A link to a page
// that has a Markdown copy points at the copy.
func (l *LLMsTxt) destination(v plugin.SiteView, pageURL, dest string) string {
	if strings.HasPrefix(dest, "#") {
		return render.AbsURL(l.site.BaseURL, pageURL) + dest
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" || u.Host != "" || !strings.HasPrefix(u.Path, "/") {
		return dest
	}
	basePath := "/"
	if b, err := url.Parse(l.site.BaseURL); err == nil && b.Path != "" {
		basePath = b.Path
	}
	sitePath := "/" + strings.TrimPrefix(u.Path, basePath)
	if _, ok := v.PageByURL(sitePath); ok && l.Pages && l.has(sitePath) {
		return l.pageLink(sitePath)
	}
	return render.AbsURL(l.site.BaseURL, strings.TrimPrefix(dest, strings.TrimSuffix(basePath, "/")))
}

// pageLink is the URL llms.txt lists for a page: its Markdown copy, or
// the page itself without Pages.
func (l *LLMsTxt) pageLink(pageURL string) string {
	if l.Pages {
		return render.AbsURL(l.site.BaseURL, "/"+mdPath(pageURL))
	}
	return render.AbsURL(l.site.BaseURL, pageURL)
}

// mdPath is the output path of a page's Markdown copy:
// "/guide/setup/" -> "guide/setup.md", "/" -> "index.md".
func mdPath(pageURL string) string {
	return cmp.Or(strings.Trim(pageURL, "/"), "index") + ".md"
}

// linkEdit finds the destination of an inline, reference or shortcut
// link or image n in src. A reference link becomes an inline one, since
// its definition is not rewritten.
func linkEdit(src []byte, n ast.Node, dest string, title []byte) (llmsEdit, bool) {
	open := n.Pos()
	if open >= 0 && open < len(src) && src[open] == '!' {
		open++
	}
	if open < 0 || open >= len(src) || src[open] != '[' {
		return llmsEdit{}, false
	}
	i := closingBracket(src, max(open+1, textEnd(n)))
	if i < 0 {
		return llmsEdit{}, false
	}
	i++ // past "]"
	// A reference link is replaced by (dest "title").
	ref := llmsEdit{start: i, end: i, link: true, dest: dest, pre: "(", post: ")"}
	if len(title) > 0 {
		ref.post = ` "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(string(title)) + `")`
	}
	switch {
	case i < len(src) && src[i] == '(':
		s, e, ok := inlineDestination(src, i+1)
		if !ok {
			return llmsEdit{}, false
		}
		return llmsEdit{start: s, end: e, link: true, dest: dest}, true
	case i < len(src) && src[i] == '[': // [text][ref] or [text][]
		c := closingBracket(src, i+1)
		if c < 0 {
			return llmsEdit{}, false
		}
		ref.end = c + 1
		return ref, true
	default: // [text]
		return ref, true
	}
}

// textEnd returns where the last text inside n ends in the source, or -1.
func textEnd(n ast.Node) int {
	end := -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			end = max(end, t.Segment.Stop)
		}
		return ast.WalkContinue, nil
	})
	return end
}

// closingBracket returns the index of the first unescaped "]" at or
// after i, or -1.
func closingBracket(src []byte, i int) int {
	for ; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case ']':
			return i
		}
	}
	return -1
}

// inlineDestination returns the span of the destination of an inline
// link whose "(" is just before i.
func inlineDestination(src []byte, i int) (start, end int, ok bool) {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n') {
		i++
	}
	start = i
	if i < len(src) && src[i] == '<' {
		j := bytes.IndexByte(src[i:], '>')
		if j < 0 {
			return 0, 0, false
		}
		return start, i + j + 1, true
	}
	depth := 0
	for ; i < len(src); i++ {
		switch c := src[i]; {
		case c == '\\':
			i++
		case c == '(':
			depth++
		case c == ')' && depth == 0, c == ' ', c == '\t', c == '\n':
			return start, i, true
		case c == ')':
			depth--
		}
	}
	return 0, 0, false
}

var directiveLinkRE = regexp.MustCompile(`([\s{])(href|src)=("[^"]*"|'[^']*'|[^\s}]+)`)

// directiveEdits rewrites the link attributes on a directive's opening
// line.
func directiveEdits(src []byte, n *markdown.GenericDirective) []llmsEdit {
	off := n.Offset
	if off < 0 || off >= len(src) {
		return nil
	}
	end := bytes.IndexByte(src[off:], '\n')
	if end < 0 {
		end = len(src) - off
	}
	var edits []llmsEdit
	for _, m := range directiveLinkRE.FindAllSubmatchIndex(src[off:off+end], -1) {
		dest, ok := n.AttributeString(string(src[off+m[4] : off+m[5]]))
		if d, isBytes := dest.([]byte); ok && isBytes {
			edits = append(edits, llmsEdit{start: off + m[6], end: off + m[7], link: true, dest: string(d), pre: `"`, post: `"`})
		}
	}
	return edits
}

// headingEnd returns the end of a heading's last source line, including
// a setext underline.
func headingEnd(src []byte, h *ast.Heading) int {
	end := h.Pos()
	if h.Lines().Len() > 0 {
		end = h.Lines().At(h.Lines().Len() - 1).Stop
	}
	lineEnd := func(i int) int {
		if j := bytes.IndexByte(src[i:], '\n'); j >= 0 {
			return i + j
		}
		return len(src)
	}
	end = lineEnd(end)
	if next := end + 1; next < len(src) {
		if u := bytes.TrimSpace(src[next:lineEnd(next)]); len(u) > 0 && strings.Trim(string(u), "=") == "" {
			end = lineEnd(next)
		}
	}
	return end
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func escapeLinkText(s string) string {
	return strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(s)
}
