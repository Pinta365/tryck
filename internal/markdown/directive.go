package markdown

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Pinta365/tryck/pkg/plugin"
)

// Directive blocks (spec §7.2):
//
//	:::name[title]{attrs}
//	Markdown content
//	:::
//
// A closing fence needs at least as many colons as its opening fence. It
// closes the innermost open directive it satisfies, unless that directive
// used a shorter fence than its parent: then the longer fence is the
// parent's. So nesting works with equal fences, and longer outer fences
// make it explicit.

// Admonition kinds rendered as <aside class="admonition admonition-{kind}">.
var admonitionKinds = map[string]bool{"note": true, "tip": true, "info": true, "warning": true, "danger": true}

// fence holds what every directive node records about its source.
type fence struct {
	Name   string
	Title  string
	Length int  // number of colons in the opening fence
	Offset int  // byte offset of the opening line in the source
	Closed bool // a closing fence was found
}

func (f *fence) directive() *fence { return f }

// fenced is implemented by every directive node.
type fenced interface {
	ast.Node
	directive() *fence
}

// Admonition is :::note, :::tip, :::info, :::warning or :::danger.
// The spec's Kind field is called Variant here, since goldmark nodes
// already have Kind and Type methods.
type Admonition struct {
	ast.BaseBlock
	fence
	Variant string // note, tip, info, warning or danger
}

// Details is :::details[summary].
type Details struct {
	ast.BaseBlock
	fence
}

// Tabs is ::::tabs, whose children are Tab nodes.
type Tabs struct {
	ast.BaseBlock
	fence
	Index int // 1-based position among the page's tab sets, for element IDs
}

// Tab is one "@tab Label" section inside Tabs.
type Tab struct {
	ast.BaseBlock
	Label  string
	Offset int
}

// GenericDirective is any directive name without a built-in renderer.
type GenericDirective struct {
	ast.BaseBlock
	fence
}

var (
	KindAdmonition       = ast.NewNodeKind("Admonition")
	KindDetails          = ast.NewNodeKind("Details")
	KindTabs             = ast.NewNodeKind("Tabs")
	KindTab              = ast.NewNodeKind("Tab")
	KindGenericDirective = ast.NewNodeKind("GenericDirective")
)

func (n *Admonition) Kind() ast.NodeKind { return KindAdmonition }
func (n *Admonition) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Variant": n.Variant, "Title": n.Title}, nil)
}

func (n *Details) Kind() ast.NodeKind { return KindDetails }
func (n *Details) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Title": n.Title}, nil)
}

func (n *Tabs) Kind() ast.NodeKind { return KindTabs }
func (n *Tabs) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

func (n *Tab) Kind() ast.NodeKind { return KindTab }
func (n *Tab) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Label": n.Label}, nil)
}

func (n *GenericDirective) Kind() ast.NodeKind { return KindGenericDirective }
func (n *GenericDirective) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Name": n.Name, "Title": n.Title}, nil)
}

var openRE = regexp.MustCompile(`^(:{3,})([A-Za-z][A-Za-z0-9_-]*)(?:\[([^\]]*)\])?[ \t]*(\{.*\})?[ \t]*$`)

var tabsIndexKey = parser.NewContextKey()

type directiveParser struct{}

func (directiveParser) Trigger() []byte { return []byte{':'} }

func (directiveParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return nil, parser.NoChildren
	}
	m := openRE.FindSubmatch(bytes.TrimRight(line[pos:], "\r\n"))
	if m == nil {
		return nil, parser.NoChildren
	}
	f := fence{
		Name:   string(m[2]),
		Title:  strings.TrimSpace(string(m[3])),
		Length: len(m[1]),
		Offset: segment.Start,
	}

	var node fenced
	switch {
	case admonitionKinds[f.Name]:
		node = &Admonition{fence: f, Variant: f.Name}
	case f.Name == "details":
		node = &Details{fence: f}
	case f.Name == "tabs":
		n, _ := pc.Get(tabsIndexKey).(int)
		pc.Set(tabsIndexKey, n+1)
		node = &Tabs{fence: f, Index: n + 1}
	default:
		node = &GenericDirective{fence: f}
	}
	if len(m[4]) > 0 {
		if attrs, ok := parser.ParseAttributes(text.NewReader(m[4])); ok {
			for _, a := range attrs {
				node.SetAttribute(a.Name, a.Value)
			}
		}
	}
	reader.AdvanceToEOL()
	return node, parser.HasChildren
}

func (directiveParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	f := node.(fenced).directive()
	pos := pc.BlockOffset()
	if pos >= 0 {
		if n := closingFence(line[pos:]); n >= f.Length && !deferToOpenChild(node, pc, n, f.Length) {
			f.Closed = true
			reader.AdvanceToEOL()
			return parser.Close
		}
	}
	return parser.Continue | parser.HasChildren
}

// closingFence returns the number of colons if line is a bare fence.
func closingFence(line []byte) int {
	n := 0
	for n < len(line) && line[n] == ':' {
		n++
	}
	if n < 3 || !util.IsBlank(line[n:]) {
		return 0
	}
	return n
}

// deferToOpenChild reports whether a fence of length n belongs to a block
// opened inside node (whose own fence has length outer): a code or HTML
// block, whose content it is, or a nested directive it can close. A
// nested directive opened with a shorter fence than its parent does not
// take the parent's longer fence.
func deferToOpenChild(node ast.Node, pc parser.Context, n, outer int) bool {
	inside := false
	for _, b := range pc.OpenedBlocks() {
		if b.Node == node {
			inside = true
			continue
		}
		if !inside {
			continue
		}
		switch c := b.Node.(type) {
		case *ast.FencedCodeBlock, *ast.HTMLBlock:
			return true
		case fenced:
			if l := c.directive().Length; n >= l && l >= outer {
				return true
			}
		}
	}
	return false
}

func (directiveParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}
func (directiveParser) CanInterruptParagraph() bool                                { return true }
func (directiveParser) CanAcceptIndentedLine() bool                                { return false }

// tabParser opens a Tab for "@tab Label" lines directly inside Tabs.
type tabParser struct{}

func (tabParser) Trigger() []byte { return []byte{'@'} }

func (tabParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if _, ok := parent.(*Tabs); !ok {
		return nil, parser.NoChildren
	}
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 {
		return nil, parser.NoChildren
	}
	label, ok := tabLabel(line[pos:])
	if !ok {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return &Tab{Label: label, Offset: segment.Start}, parser.HasChildren
}

func (tabParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	line, _ := reader.PeekLine()
	if pos := pc.BlockOffset(); pos >= 0 {
		if _, ok := tabLabel(line[pos:]); ok && !deferToOpenChild(node, pc, 0, 0) {
			return parser.Close // the next @tab opens a sibling
		}
	}
	return parser.Continue | parser.HasChildren
}

// tabLabel parses "@tab Label".
func tabLabel(line []byte) (string, bool) {
	rest, ok := bytes.CutPrefix(line, []byte("@tab"))
	if !ok || len(rest) == 0 || (rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	label := strings.TrimSpace(string(rest))
	return label, label != ""
}

func (tabParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}
func (tabParser) CanInterruptParagraph() bool                                { return true }
func (tabParser) CanAcceptIndentedLine() bool                                { return false }

// directives is the goldmark extension for ::: blocks.
type directives struct {
	custom map[string]plugin.DirectiveRenderer
}

func (d directives) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(directiveParser{}, 50),
		util.Prioritized(tabParser{}, 50),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(directiveRenderer{custom: d.custom}, 500)))
}
