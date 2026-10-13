package plugins

import (
	"bytes"
	"fmt"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"

	"github.com/Pinta365/tryck/pkg/plugin"
)

// Highlight colors fenced code blocks with chroma. It emits CSS classes
// rather than inline styles and writes the theme to /css/chroma.css, so
// themes and dark mode can be switched in CSS.
type Highlight struct {
	Theme       string `yaml:"theme"`
	LineNumbers bool   `yaml:"lineNumbers"`
}

const chromaCSSPath = "css/chroma.css"

func (*Highlight) Name() string          { return "syntaxHighlighting" }
func (*Highlight) configKeys() []string  { return []string{"theme", "lineNumbers"} }
func (*Highlight) Stylesheets() []string { return []string{"/" + chromaCSSPath} }

func (h *Highlight) Init(cfg map[string]any, _ plugin.SiteInfo) error {
	h.Theme = "nord"
	if err := decode(cfg, h); err != nil {
		return err
	}
	if _, ok := styles.Registry[h.Theme]; !ok {
		return fmt.Errorf("unknown theme %q", h.Theme)
	}
	return nil
}

func (h *Highlight) formatOptions() []chromahtml.Option {
	return []chromahtml.Option{chromahtml.WithClasses(true), chromahtml.WithLineNumbers(h.LineNumbers)}
}

func (h *Highlight) Extend(m goldmark.Markdown) {
	highlighting.NewHighlighting(
		highlighting.WithStyle(h.Theme),
		highlighting.WithFormatOptions(h.formatOptions()...),
	).Extend(m)
}

func (h *Highlight) OnBuildComplete(_ plugin.SiteView, out plugin.Outputs) error {
	var buf bytes.Buffer
	if err := chromahtml.New(h.formatOptions()...).WriteCSS(&buf, styles.Get(h.Theme)); err != nil {
		return err
	}
	return out.Add(chromaCSSPath, buf.Bytes())
}
