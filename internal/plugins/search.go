package plugins

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Search writes /search-index.json: [{url, title, headings, text}] for
// every page that is not a draft, with text cut to MaxTextLength bytes.
// headings are the page's level 2 to 6 headings as {id, text}. Code
// blocks are left out of the text. The built-in theme's search box reads
// this index.
type Search struct {
	MaxTextLength int `yaml:"maxTextLength"`

	site plugin.SiteInfo
	mu   sync.Mutex
	docs map[string]searchDoc // by page URL
}

type searchDoc struct {
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Headings []searchHeading `json:"headings"`
	Text     string          `json:"text"`
}

type searchHeading struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func (*Search) Name() string         { return "search" }
func (*Search) ConfigKeys() []string { return []string{"maxTextLength"} }

func (s *Search) Init(cfg map[string]any, site plugin.SiteInfo) error {
	s.MaxTextLength = 5000
	s.site = site
	s.docs = map[string]searchDoc{}
	return decode(cfg, s)
}

func (s *Search) OnPageParsed(p *plugin.Page) error {
	var text strings.Builder
	headings := []searchHeading{}
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.Heading:
			h := strings.TrimSpace(content.PlainText(t, p.Source))
			if t.Level > 1 {
				id, _ := t.AttributeString("id")
				idb, _ := id.([]byte)
				headings = append(headings, searchHeading{ID: string(idb), Text: h})
			}
			appendText(&text, h)
			return ast.WalkSkipChildren, nil
		}
		// Any block holding inline text: paragraphs, list items, table cells.
		if n.Type() == ast.TypeBlock && n.FirstChild() != nil && n.FirstChild().Type() == ast.TypeInline {
			appendText(&text, content.PlainText(n, p.Source))
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	s.mu.Lock()
	s.docs[p.URL] = searchDoc{Headings: headings, Text: truncate(text.String(), s.MaxTextLength)}
	s.mu.Unlock()
	return nil
}

func appendText(b *strings.Builder, s string) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte(' ')
	}
	b.WriteString(s)
}

// truncate cuts s to at most n bytes without splitting a character.
func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return strings.TrimSpace(s[:n])
}

func (s *Search) OnBuildComplete(v plugin.SiteView, out plugin.Outputs) error {
	docs := []searchDoc{}
	for _, p := range v.Pages() {
		d, ok := s.docs[p.URL]
		if !ok || p.Draft {
			continue
		}
		d.URL = render.RelURL(s.site.BaseURL, p.URL)
		d.Title = p.Title
		docs = append(docs, d)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(docs); err != nil {
		return err
	}
	return out.Add("search-index.json", buf.Bytes())
}
