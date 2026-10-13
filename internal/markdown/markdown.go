// Package markdown configures goldmark: CommonMark plus GFM, heading IDs
// and, in later milestones, tryck's ::: directives.
package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// Options configures the Markdown processor.
type Options struct {
	// UnsafeHTML passes raw HTML in Markdown through. Otherwise it is
	// replaced by an HTML comment (spec §9.4).
	UnsafeHTML bool
}

// Processor parses and renders Markdown. It is safe for concurrent use.
type Processor struct {
	md goldmark.Markdown
}

// New returns a Processor configured from opts.
func New(opts Options) *Processor {
	var rendererOpts []goldmark.Option
	if opts.UnsafeHTML {
		rendererOpts = append(rendererOpts, goldmark.WithRendererOptions(html.WithUnsafe()))
	}
	md := goldmark.New(append([]goldmark.Option{
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // unique per page: dupes get -1, -2
			parser.WithAttribute(),     // {#custom-id} wins over the generated ID
		),
	}, rendererOpts...)...)
	return &Processor{md: md}
}

// Parse parses source into a document AST.
func (p *Processor) Parse(source []byte) ast.Node {
	return p.md.Parser().Parse(text.NewReader(source))
}

// Render renders a document parsed from source to an HTML fragment.
func (p *Processor) Render(doc ast.Node, source []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := p.md.Renderer().Render(&buf, source, doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
