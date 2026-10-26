package plugins

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Pinta365/tryck/pkg/plugin"
)

// Math renders TeX formulas to MathML at build time with KaTeX, so pages
// need no script, stylesheet or font for them. It understands the syntax
// GitHub does: $...$ inline (with Inline), $$...$$ for a displayed
// formula, alone on its lines or within a paragraph, and ```math blocks.
// A formula KaTeX cannot parse is a math.error warning and is shown as
// code.
type Math struct {
	Inline bool              `yaml:"inline"`
	Macros map[string]string `yaml:"macros"`

	macros string // Macros as JSON, part of the cache key
}

const katexVersion = "0.18.11"

//go:embed katex/katex.min.js
var katexJS string

func (*Math) Name() string         { return "math" }
func (*Math) ConfigKeys() []string { return []string{"inline", "macros"} }

func (m *Math) Init(cfg map[string]any, _ plugin.SiteInfo) error {
	*m = Math{Inline: true}
	if err := decode(cfg, m); err != nil {
		return err
	}
	if m.Macros == nil {
		m.Macros = map[string]string{} // KaTeX needs an object, not null
	}
	b, err := json.Marshal(m.Macros)
	if err != nil {
		return err
	}
	m.macros = string(b)
	return nil
}

// Formula is a TeX formula in a page. HTML is set once it is rendered.
type Formula struct {
	Display bool
	TeX     string
	Offset  int // of the formula in the source, for diagnostics; -1 if unknown
	HTML    string
}

func (f *Formula) formula() *Formula { return f }

type formula interface{ formula() *Formula }

// MathInline is a formula within a paragraph: $...$, or $$...$$
// displayed.
type MathInline struct {
	ast.BaseInline
	Formula
}

// MathBlock is a displayed formula on lines of its own: $$ lines or a
// ```math block.
type MathBlock struct {
	ast.BaseBlock
	Formula
	closed bool
}

var (
	KindMathInline = ast.NewNodeKind("MathInline")
	KindMathBlock  = ast.NewNodeKind("MathBlock")
)

func (n *MathInline) Kind() ast.NodeKind { return KindMathInline }
func (n *MathInline) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"TeX": n.TeX}, nil)
}
func (n *MathBlock) Kind() ast.NodeKind { return KindMathBlock }
func (n *MathBlock) IsRaw() bool        { return true }
func (n *MathBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"TeX": n.TeX}, nil)
}

func (m *Math) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(
		parser.WithInlineParsers(util.Prioritized(&mathInlineParser{single: m.Inline}, 150)),
		parser.WithBlockParsers(util.Prioritized(mathBlockParser{}, 150)),
		parser.WithASTTransformers(util.Prioritized(mathFenceTransformer{}, 100)),
	)
	md.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(mathRenderer{}, 100)))
}

// OnPageParsed renders the page's formulas and reports the ones KaTeX
// cannot parse.
func (m *Math) OnPageParsed(p *plugin.Page) error {
	var err error
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		f, ok := n.(formula)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		mn := f.formula()
		html, msg, rerr := m.render(mn.TeX, mn.Display)
		if rerr != nil {
			err = rerr
			return ast.WalkStop, nil
		}
		if msg != "" {
			mn.HTML = mathError(mn.TeX, msg, mn.Display)
			line := 0
			if mn.Offset >= 0 && mn.Offset <= len(p.Source) {
				line = p.BodyLine + bytes.Count(p.Source[:mn.Offset], []byte("\n"))
			}
			p.Diags.Report(plugin.Diagnostic{Severity: plugin.Warning, Line: line, Code: "math.error", Message: msg})
		} else {
			mn.HTML = html
		}
		return ast.WalkSkipChildren, nil
	})
	return err
}

// mathError shows a formula that could not be rendered as its source,
// with the reason as a tooltip.
func mathError(tex, msg string, display bool) string {
	if display {
		return fmt.Sprintf(`<pre class="math-error" title="%s"><code>%s</code></pre>`, html.EscapeString(msg), html.EscapeString(tex))
	}
	return fmt.Sprintf(`<code class="math-error" title="%s">%s</code>`, html.EscapeString(msg), html.EscapeString(tex))
}

// Rendered formulas are kept for the life of the process, so tryck dev
// renders each formula once.
var mathCache sync.Map // key -> mathResult

type mathResult struct{ html, msg string }

// render returns a formula's HTML, or KaTeX's message if it cannot
// parse it. err is for KaTeX itself failing.
func (m *Math) render(tex string, display bool) (html, msg string, err error) {
	key := fmt.Sprintf("%t\x00%s\x00%s", display, m.macros, tex)
	if r, ok := mathCache.Load(key); ok {
		return r.(mathResult).html, r.(mathResult).msg, nil
	}
	e, err := getKatex()
	if err != nil {
		return "", "", err
	}
	defer katexEngines.Put(e)
	html, msg, err = e.render(tex, display, m.macros)
	if err == nil {
		mathCache.Store(key, mathResult{html, msg})
	}
	return html, msg, err
}

// katexEngine is KaTeX loaded in a goja runtime. A runtime is not safe
// for concurrent use, so engines are pooled.
type katexEngine struct {
	rt     *goja.Runtime
	render func(tex string, display bool, macros string) (string, string, error)
}

var katexEngines sync.Pool

func getKatex() (*katexEngine, error) {
	if e, ok := katexEngines.Get().(*katexEngine); ok {
		return e, nil
	}
	rt := goja.New()
	if _, err := rt.RunString("var module = {exports: {}}, exports = module.exports;"); err != nil {
		return nil, err
	}
	if _, err := rt.RunScript("katex.min.js", katexJS); err != nil {
		return nil, fmt.Errorf("load KaTeX %s: %w", katexVersion, err)
	}
	if _, err := rt.RunString(`function tryckRender(tex, display, macros) {
	try {
		return [module.exports.renderToString(tex, {displayMode: display, output: "mathml", throwOnError: true, macros: JSON.parse(macros)}), ""];
	} catch (e) {
		if (e instanceof module.exports.ParseError) return ["", e.message];
		throw e;
	}
}`); err != nil {
		return nil, err
	}
	var fn func(string, bool, string) []string
	if err := rt.ExportTo(rt.Get("tryckRender"), &fn); err != nil {
		return nil, err
	}
	e := &katexEngine{rt: rt}
	e.render = func(tex string, display bool, macros string) (out, msg string, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("KaTeX: %v", r)
			}
		}()
		r := fn(tex, display, macros)
		return r[0], strings.TrimPrefix(r[1], "KaTeX parse error: "), nil
	}
	return e, nil
}

type mathRenderer struct{}

func (mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMathInline, renderMath)
	reg.Register(KindMathBlock, renderMath)
}

func renderMath(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	mn := n.(formula).formula()
	out := mn.HTML
	if out == "" { // not rendered: the math plugin's page hook did not run
		out = mathError(mn.TeX, "not rendered", mn.Display)
	}
	if _, ok := n.(*MathBlock); ok {
		_, _ = w.WriteString(`<div class="math">` + out + "</div>\n")
	} else {
		_, _ = w.WriteString(out)
	}
	return ast.WalkSkipChildren, nil
}

// mathInlineParser parses $...$ (when single is set) and $$...$$ within
// a paragraph. Single dollars follow Pandoc: the opening $ has no space
// after it, the closing $ no space before it and no digit after it, so
// "$5 and $10" stays text. A formula may span lines.
type mathInlineParser struct{ single bool }

func (p *mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (p *mathInlineParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, start := block.PeekLine()
	delim := 1
	if len(line) > 1 && line[1] == '$' {
		delim = 2
	}
	if delim == 1 && !p.single {
		return nil
	}
	if len(line) <= delim || line[delim] == ' ' || line[delim] == '\t' || line[delim] == '\n' {
		return nil
	}
	l, pos := block.Position()
	block.Advance(delim)
	var tex strings.Builder
	for {
		line, _ := block.PeekLine()
		if line == nil {
			block.SetPosition(l, pos)
			return nil
		}
		for i := 0; i < len(line); i++ {
			switch {
			case line[i] == '\\' && i+1 < len(line):
				i++
			case line[i] == '`':
				// A code span comes first: "$5 and `$x`" is not math.
				block.SetPosition(l, pos)
				return nil
			case line[i] == '$':
				if delim == 2 && (i+1 >= len(line) || line[i+1] != '$') {
					continue
				}
				prev := byte(' ')
				if i > 0 {
					prev = line[i-1]
				} else if tex.Len() > 0 {
					prev = '\n'
				}
				after := byte(' ')
				if i+delim < len(line) {
					after = line[i+delim]
				}
				if delim == 1 && (prev == ' ' || prev == '\t' || prev == '\n' || (after >= '0' && after <= '9')) {
					continue
				}
				tex.Write(line[:i])
				block.Advance(i + delim)
				return &MathInline{Formula: Formula{Display: delim == 2, TeX: strings.TrimSpace(tex.String()), Offset: start.Start}}
			}
		}
		tex.Write(line)
		block.AdvanceLine()
	}
}

// mathBlockParser parses $$ on lines of its own:
//
//	$$
//	\int_0^1 x\,dx
//	$$
//
// or "$$ ... $$" on one line, or opened and closed on content lines.
type mathBlockParser struct{}

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (mathBlockParser) Open(_ ast.Node, reader text.Reader, _ parser.Context) (ast.Node, parser.State) {
	line, seg := reader.PeekLine()
	pos := reader.LineOffset()
	trimmed := bytes.TrimSpace(line)
	if pos > 3 || !bytes.HasPrefix(trimmed, []byte("$$")) {
		return nil, parser.NoChildren
	}
	rest := bytes.TrimSpace(trimmed[2:])
	n := &MathBlock{}
	n.Display, n.Offset = true, seg.Start
	if bytes.HasSuffix(rest, []byte("$$")) {
		// $$ ... $$ on one line
		n.TeX = string(bytes.TrimSpace(rest[:len(rest)-2]))
		n.SetBlankPreviousLines(false)
		reader.AdvanceToEOL()
		n.Lines().Append(seg)
		n.closed = true
		return n, parser.NoChildren
	}
	if len(rest) > 0 && !bytes.ContainsAny(rest, "$") {
		n.TeX = string(rest) + "\n"
	} else if len(rest) > 0 {
		return nil, parser.NoChildren // "$$x$" and the like: leave it to the paragraph
	}
	reader.AdvanceToEOL()
	n.Lines().Append(seg)
	return n, parser.NoChildren
}

func (mathBlockParser) Continue(node ast.Node, reader text.Reader, _ parser.Context) parser.State {
	n := node.(*MathBlock)
	if n.closed {
		return parser.Close
	}
	line, seg := reader.PeekLine()
	if line == nil {
		return parser.Close
	}
	n.Lines().Append(seg)
	trimmed := bytes.TrimSpace(line)
	reader.AdvanceToEOL()
	if before, ok := bytes.CutSuffix(trimmed, []byte("$$")); ok {
		n.TeX += string(before)
		n.TeX = strings.TrimSpace(n.TeX)
		n.closed = true
		return parser.Close
	}
	n.TeX += string(line)
	return parser.Continue | parser.NoChildren
}

func (mathBlockParser) Close(node ast.Node, _ text.Reader, _ parser.Context) {
	n := node.(*MathBlock)
	n.TeX = strings.TrimSpace(n.TeX)
}

func (mathBlockParser) CanInterruptParagraph() bool { return true }
func (mathBlockParser) CanAcceptIndentedLine() bool { return false }

type mathFenceTransformer struct{}

// Transform replaces ```math fenced code blocks with MathBlocks.
func (mathFenceTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var blocks []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if fc, ok := n.(*ast.FencedCodeBlock); ok && entering && string(fc.Language(source)) == "math" {
			blocks = append(blocks, fc)
		}
		return ast.WalkContinue, nil
	})
	for _, fc := range blocks {
		mb := &MathBlock{}
		mb.Display, mb.Offset = true, -1
		if fc.Lines().Len() > 0 {
			mb.Offset = fc.Lines().At(0).Start
		}
		var tex strings.Builder
		for i := 0; i < fc.Lines().Len(); i++ {
			seg := fc.Lines().At(i)
			tex.Write(seg.Value(source))
		}
		mb.TeX = strings.TrimSpace(tex.String())
		mb.SetLines(fc.Lines())
		fc.Parent().ReplaceChild(fc.Parent(), fc, mb)
	}
}
