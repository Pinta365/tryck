package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/Pinta365/tryck/pkg/plugin"
)

// Mermaid renders ```mermaid code blocks as diagrams. Blocks become
// <pre class="mermaid"> (readable source without JavaScript), and pages
// that have one get a module script that loads mermaid from URL and
// draws them, in the dark theme when the page is dark.
type Mermaid struct {
	URL string `yaml:"url"`
}

// DefaultMermaidURL loads mermaid 11 from jsDelivr. Point url at a copy
// in static/ for sites that must work offline.
const DefaultMermaidURL = "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs"

func (*Mermaid) Name() string         { return "mermaid" }
func (*Mermaid) ConfigKeys() []string { return []string{"url"} }

func (m *Mermaid) Init(cfg map[string]any, _ plugin.SiteInfo) error {
	m.URL = DefaultMermaidURL
	if err := decode(cfg, m); err != nil {
		return err
	}
	if m.URL == "" {
		return fmt.Errorf("url must not be empty")
	}
	return nil
}

// MermaidBlock is a ```mermaid code block.
type MermaidBlock struct {
	ast.BaseBlock
}

// KindMermaidBlock is the node kind of MermaidBlock.
var KindMermaidBlock = ast.NewNodeKind("MermaidBlock")

func (n *MermaidBlock) Kind() ast.NodeKind { return KindMermaidBlock }
func (n *MermaidBlock) IsRaw() bool        { return true }
func (n *MermaidBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

func (m *Mermaid) Extend(md goldmark.Markdown) {
	md.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(mermaidTransformer{}, 100)))
	md.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(mermaidRenderer{}, 100)))
}

type mermaidTransformer struct{}

// Transform replaces ```mermaid fenced code blocks with MermaidBlocks.
func (mermaidTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var blocks []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if fc, ok := n.(*ast.FencedCodeBlock); ok && entering && string(fc.Language(source)) == "mermaid" {
			blocks = append(blocks, fc)
		}
		return ast.WalkContinue, nil
	})
	for _, fc := range blocks {
		mb := &MermaidBlock{}
		mb.SetLines(fc.Lines())
		fc.Parent().ReplaceChild(fc.Parent(), fc, mb)
	}
}

type mermaidRenderer struct{}

func (mermaidRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMermaidBlock, func(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		w.WriteString(`<pre class="mermaid">`)
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			w.Write(util.EscapeHTML(seg.Value(source)))
		}
		w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	})
}

func (*Mermaid) OnPageParsed(p *plugin.Page) error {
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if n.Kind() == KindMermaidBlock {
			p.Data["mermaid"] = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return nil
}

// mermaidScript draws every pre.mermaid and redraws them when the page
// switches between light and dark.
const mermaidScript = `<script type="module">
import mermaid from %s;
const blocks = [...document.querySelectorAll("pre.mermaid")];
blocks.forEach((b) => (b.dataset.source = b.textContent));
const dark = () => {
  const t = document.documentElement.dataset.theme;
  return t ? t === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
};
async function draw() {
  blocks.forEach((b) => { b.removeAttribute("data-processed"); b.textContent = b.dataset.source; });
  // A darker edge label background than mermaid's dark theme, so labels
  // meet the 4.5:1 contrast minimum.
  const themeVariables = dark() ? { edgeLabelBackground: "#333333" } : {};
  mermaid.initialize({ startOnLoad: false, theme: dark() ? "dark" : "default", themeVariables });
  await mermaid.run({ nodes: blocks });
}
draw();
new MutationObserver(draw).observe(document.documentElement, { attributeFilter: ["data-theme"] });
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", draw);
</script>
`

func (m *Mermaid) OnPageRendered(p *plugin.Page, _ plugin.SiteView) error {
	if p.Data["mermaid"] != true {
		return nil
	}
	url, err := json.Marshal(m.URL) // a JS string literal; < is escaped, so no </script>
	if err != nil {
		return err
	}
	script := fmt.Sprintf(mermaidScript, url)
	if i := bytes.LastIndex(p.HTML, []byte("</body>")); i >= 0 {
		p.HTML = append(p.HTML[:i:i], append([]byte(script), p.HTML[i:]...)...)
	} else {
		p.HTML = append(p.HTML, script...)
	}
	return nil
}
