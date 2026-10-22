package markdown

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// headingAnchors renders headings like goldmark does, and adds a "#"
// link to the heading's own ID after the text of every heading below h1.
type headingAnchors struct{}

func (headingAnchors) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, renderHeading)
}

func renderHeading(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Heading)
	if entering {
		fmt.Fprintf(w, "<h%d", n.Level)
		if n.Attributes() != nil {
			html.RenderAttributes(w, n, html.HeadingAttributeFilter)
		}
		w.WriteByte('>')
		return ast.WalkContinue, nil
	}
	if id, ok := n.AttributeString("id"); ok && n.Level > 1 {
		fmt.Fprintf(w, `<a class="heading-anchor" href="#%s" aria-label="Link to this section">#</a>`, util.EscapeHTML(attrBytes(id)))
	}
	fmt.Fprintf(w, "</h%d>\n", n.Level)
	return ast.WalkContinue, nil
}
