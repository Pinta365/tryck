package plugins

import (
	"bytes"
	"fmt"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"

	"github.com/Pinta365/tryck/pkg/plugin"
)

// Highlight colors fenced code blocks with chroma. It emits CSS classes
// rather than inline styles and writes the theme to /css/chroma.css, so
// themes and dark mode can be switched in CSS: Theme applies in light
// mode and DarkTheme in dark mode (see the built-in theme's colorScheme).
type Highlight struct {
	Theme       string `yaml:"theme"`
	DarkTheme   string `yaml:"darkTheme"` // "" = Theme in both modes
	LineNumbers bool   `yaml:"lineNumbers"`
}

const chromaCSSPath = "css/chroma.css"

func (*Highlight) Name() string          { return "syntaxHighlighting" }
func (*Highlight) ConfigKeys() []string  { return []string{"theme", "darkTheme", "lineNumbers"} }
func (*Highlight) Stylesheets() []string { return []string{"/" + chromaCSSPath} }

func (h *Highlight) Init(cfg map[string]any, _ plugin.SiteInfo) error {
	h.Theme = "github"
	if err := decode(cfg, h); err != nil {
		return err
	}
	for _, t := range []string{h.Theme, h.DarkTheme} {
		if _, ok := styles.Registry[t]; t != "" && !ok {
			return fmt.Errorf("unknown theme %q", t)
		}
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
	f := chromahtml.New(h.formatOptions()...)
	var light bytes.Buffer
	if err := f.WriteCSS(&light, styles.Get(h.Theme)); err != nil {
		return err
	}
	if h.DarkTheme == "" || h.DarkTheme == h.Theme {
		return out.Add(chromaCSSPath, light.Bytes())
	}
	var dark bytes.Buffer
	if err := f.WriteCSS(&dark, styles.Get(h.DarkTheme)); err != nil {
		return err
	}
	// Scope each theme to its own mode, so no rule applies in both.
	var buf bytes.Buffer
	for _, mode := range []struct{ name, theme, css, other string }{
		{"light", h.Theme, light.String(), "dark"},
		{"dark", h.DarkTheme, dark.String(), "light"},
	} {
		fmt.Fprintf(&buf, "/* %s mode: %s */\n", mode.name, mode.theme)
		buf.WriteString(scopeCSS(mode.css, `:root[data-theme="`+mode.name+`"]`, ""))
		fmt.Fprintf(&buf, "@media (prefers-color-scheme: %s) {\n", mode.name)
		buf.WriteString(scopeCSS(mode.css, `:root:not([data-theme="`+mode.other+`"])`, "  "))
		buf.WriteString("}\n")
	}
	return out.Add(chromaCSSPath, buf.Bytes())
}

// scopeCSS prefixes every selector of chroma's one-rule-per-line CSS
// with scope.
func scopeCSS(css, scope, indent string) string {
	var b strings.Builder
	for _, line := range strings.Split(css, "\n") {
		open := strings.Index(line, "{")
		if open < 0 {
			continue
		}
		sel := line[:open]
		if end := strings.Index(sel, "*/"); end >= 0 {
			sel = sel[end+2:]
		}
		parts := strings.Split(sel, ",")
		for i, p := range parts {
			parts[i] = scope + " " + strings.TrimSpace(p)
		}
		b.WriteString(indent + strings.Join(parts, ", ") + " " + line[open:] + "\n")
	}
	return b.String()
}
