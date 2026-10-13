package plugins

import (
	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// TOC stores each page's table of contents as Data["toc"]
// ([]plugin.TOCEntry), from headings between MinLevel and MaxLevel.
// Front matter toc: true/false overrides Default.
type TOC struct {
	Default            bool
	MinLevel, MaxLevel int
}

func (*TOC) Name() string                               { return "toc" }
func (*TOC) Priority() int                              { return 10 }
func (*TOC) Init(map[string]any, plugin.SiteInfo) error { return nil }
func (*TOC) configKeys() []string                       { return nil }

func (t *TOC) OnPageParsed(p *plugin.Page) error {
	show := t.Default
	if p.Front.TOC != nil {
		show = *p.Front.TOC
	}
	if !show || p.AST == nil {
		return nil
	}
	if entries := t.entries(p.AST, p.Source); len(entries) > 0 {
		p.Data["toc"] = entries
	}
	return nil
}

type tocNode struct {
	entry plugin.TOCEntry
	kids  []*tocNode
}

// entries nests headings by level. A heading deeper than its predecessor
// becomes its child, even when levels are skipped.
func (t *TOC) entries(doc ast.Node, source []byte) []plugin.TOCEntry {
	root := &tocNode{entry: plugin.TOCEntry{Level: 0}}
	stack := []*tocNode{root}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		if h.Level < t.MinLevel || h.Level > t.MaxLevel {
			return ast.WalkSkipChildren, nil
		}
		id := ""
		if v, ok := h.AttributeString("id"); ok {
			if b, ok := v.([]byte); ok {
				id = string(b)
			}
		}
		for len(stack) > 1 && stack[len(stack)-1].entry.Level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		node := &tocNode{entry: plugin.TOCEntry{Level: h.Level, ID: id, Text: content.PlainText(h, source)}}
		parent := stack[len(stack)-1]
		parent.kids = append(parent.kids, node)
		stack = append(stack, node)
		return ast.WalkSkipChildren, nil
	})
	return flatten(root.kids)
}

func flatten(nodes []*tocNode) []plugin.TOCEntry {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]plugin.TOCEntry, len(nodes))
	for i, n := range nodes {
		out[i] = n.entry
		out[i].Children = flatten(n.kids)
	}
	return out
}
