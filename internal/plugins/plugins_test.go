package plugins

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/markdown"
	"github.com/Pinta365/tryck/pkg/plugin"
)

func parsePage(t *testing.T, src string) *plugin.Page {
	t.Helper()
	doc, _ := markdown.New(markdown.Options{}).Parse([]byte(src))
	return &plugin.Page{AST: doc, Source: []byte(src), Data: map[string]any{}}
}

func tocJSON(t *testing.T, p *plugin.Page) string {
	t.Helper()
	b, err := json.Marshal(p.Data["toc"])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTOCNesting(t *testing.T) {
	p := parsePage(t, "# Title\n\n## A\n\n### A1\n\n#### too deep\n\n### A2\n\n## B\n\n### B1 `code`\n")
	if err := (&TOC{Default: true, MinLevel: 2, MaxLevel: 3}).OnPageParsed(p); err != nil {
		t.Fatal(err)
	}
	want := `[{"Level":2,"ID":"a","Text":"A","Children":[{"Level":3,"ID":"a1","Text":"A1","Children":null},{"Level":3,"ID":"a2","Text":"A2","Children":null}]},` +
		`{"Level":2,"ID":"b","Text":"B","Children":[{"Level":3,"ID":"b1-code","Text":"B1 code","Children":null}]}]`
	if got := tocJSON(t, p); got != want {
		t.Errorf("toc:\n%s\nwant:\n%s", got, want)
	}
}

func TestTOCSkippedLevels(t *testing.T) {
	p := parsePage(t, "### Deep first\n\n## Then shallow\n\n#### Skip a level\n")
	if err := (&TOC{Default: true, MinLevel: 2, MaxLevel: 4}).OnPageParsed(p); err != nil {
		t.Fatal(err)
	}
	want := `[{"Level":3,"ID":"deep-first","Text":"Deep first","Children":null},{"Level":2,"ID":"then-shallow","Text":"Then shallow","Children":[{"Level":4,"ID":"skip-a-level","Text":"Skip a level","Children":null}]}]`
	if got := tocJSON(t, p); got != want {
		t.Errorf("toc:\n%s\nwant:\n%s", got, want)
	}
}

func TestTOCFrontMatterOverride(t *testing.T) {
	no, yes := false, true
	tests := []struct {
		def   bool
		front *bool
		want  bool
	}{{true, nil, true}, {false, nil, false}, {true, &no, false}, {false, &yes, true}}
	for _, tt := range tests {
		p := parsePage(t, "## A\n")
		p.Front.TOC = tt.front
		if err := (&TOC{Default: tt.def, MinLevel: 2, MaxLevel: 3}).OnPageParsed(p); err != nil {
			t.Fatal(err)
		}
		if _, got := p.Data["toc"]; got != tt.want {
			t.Errorf("default=%v front=%v: toc present = %v", tt.def, tt.front, got)
		}
	}
	p := parsePage(t, "No headings.\n")
	_ = (&TOC{Default: true, MinLevel: 2, MaxLevel: 3}).OnPageParsed(p)
	if _, ok := p.Data["toc"]; ok {
		t.Error("empty toc should not be stored")
	}
}

func TestReadingTime(t *testing.T) {
	r := &ReadingTime{}
	if err := r.Init(map[string]any{"wordsPerMinute": 4}, plugin.SiteInfo{}); err != nil {
		t.Fatal(err)
	}
	src := "# One two\n\nthree **four** five_six\nseven\n\n```go\nnot counted at all\n```\n\n    indented code not counted\n\n<div>html not counted</div>\n\n- eight `nine`\n"
	p := parsePage(t, src)
	if err := r.OnPageParsed(p); err != nil {
		t.Fatal(err)
	}
	if p.Data["wordCount"] != 8 || p.Data["readingTime"] != 2 {
		t.Errorf("wordCount=%v readingTime=%v, want 8 and 2", p.Data["wordCount"], p.Data["readingTime"])
	}
	p = parsePage(t, "")
	_ = r.OnPageParsed(p)
	if p.Data["readingTime"] != 1 {
		t.Errorf("empty page readingTime = %v, want 1", p.Data["readingTime"])
	}
	if err := (&ReadingTime{}).Init(map[string]any{"wordsPerMinute": 0}, plugin.SiteInfo{}); err == nil {
		t.Error("wordsPerMinute 0 accepted")
	}
}

func TestHighlight(t *testing.T) {
	h := &Highlight{}
	if err := h.Init(map[string]any{"theme": "github", "lineNumbers": true}, plugin.SiteInfo{}); err != nil {
		t.Fatal(err)
	}
	md := markdown.New(markdown.Options{Extenders: []plugin.MarkdownExtender{h}})
	src := []byte("```go\nx := 1\n```\n")
	doc, _ := md.Parse(src)
	out, err := md.Render(doc, src)
	if err != nil {
		t.Fatal(err)
	}
	if html := string(out); !strings.Contains(html, `<pre class="chroma">`) || strings.Contains(html, "style=") || !strings.Contains(html, `class="ln"`) {
		t.Errorf("want class-based highlighting with line numbers:\n%s", html)
	}
	out2 := &memOutputs{}
	if err := h.OnBuildComplete(nil, out2); err != nil {
		t.Fatal(err)
	}
	if css := string(out2.files[chromaCSSPath]); !strings.Contains(css, ".chroma") || strings.Contains(css, "data-theme") {
		t.Errorf("chroma.css = %q", css)
	}

	dark := &Highlight{}
	if err := dark.Init(map[string]any{"theme": "github", "darkTheme": "nord"}, plugin.SiteInfo{}); err != nil {
		t.Fatal(err)
	}
	out3 := &memOutputs{}
	if err := dark.OnBuildComplete(nil, out3); err != nil {
		t.Fatal(err)
	}
	css := string(out3.files[chromaCSSPath])
	for _, want := range []string{
		`:root[data-theme="dark"] .chroma .k {`,
		"@media (prefers-color-scheme: dark) {\n  :root:not([data-theme=\"light\"]) .bg {",
		`:root[data-theme="light"] .chroma .nx {`,
		"@media (prefers-color-scheme: light) {\n  :root:not([data-theme=\"dark\"]) .bg {",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("dark chroma.css missing %q", want)
		}
	}
	// No rule may apply in both modes.
	for _, line := range strings.Split(css, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), ".") {
			t.Errorf("unscoped rule: %s", line)
		}
	}
	if err := (&Highlight{}).Init(map[string]any{"darkTheme": "nope"}, plugin.SiteInfo{}); err == nil {
		t.Error("unknown dark theme accepted")
	}
	if err := (&Highlight{}).Init(map[string]any{"theme": "no-such-theme"}, plugin.SiteInfo{}); err == nil {
		t.Error("unknown theme accepted")
	}
}

type memOutputs struct {
	files map[string][]byte
	diags []plugin.Diagnostic
}

func (m *memOutputs) Report(d plugin.Diagnostic) { m.diags = append(m.diags, d) }

func (m *memOutputs) Add(p string, b []byte) error {
	if m.files == nil {
		m.files = map[string][]byte{}
	}
	m.files[p] = b
	return nil
}

type fake struct {
	name     string
	priority int
	initErr  error
	dirs     []string
}

func (f *fake) Name() string                               { return f.name }
func (f *fake) Init(map[string]any, plugin.SiteInfo) error { return f.initErr }
func (f *fake) Priority() int                              { return f.priority }

type fakeDirective struct{ fake }

func (f *fakeDirective) DirectiveNames() []string { return f.dirs }
func (f *fakeDirective) RenderDirective(w io.Writer, d plugin.Directive, entering bool) error {
	return nil
}

func TestLoadOrderingAndConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.Plugins["readingTime"]["enabled"] = false
	cfg.Plugins["syntaxHighlighting"]["thme"] = "x"
	cfg.Plugins["nope"] = map[string]any{}
	set, ds := Load(cfg, plugin.SiteInfo{}, &fake{name: "zz", priority: 5}, &fake{name: "aa", priority: 5}, &fake{name: "late", priority: 200})

	var names []string
	for _, p := range set.Plugins() {
		names = append(names, p.Name())
	}
	if got := strings.Join(names, ","); got != "aa,zz,toc,feed,llmsTxt,math,mermaid,redirects,search,sitemap,social,syntaxHighlighting,late,linkCheck" {
		t.Errorf("order = %s", got)
	}
	var msgs []string
	for _, d := range ds {
		msgs = append(msgs, d.Severity.String()+":"+d.Message)
	}
	want := `warning:unknown config key "plugins.syntaxHighlighting.thme"|warning:unknown plugin "nope" in plugins`
	if got := strings.Join(msgs, "|"); got != want {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", got, want)
	}
}

func TestLoadErrors(t *testing.T) {
	cfg := config.Defaults()
	cfg.Plugins["sitemap"]["enabled"] = "yes"
	cfg.Plugins["syntaxHighlighting"]["theme"] = "nope"
	set, ds := Load(cfg, plugin.SiteInfo{})
	if !diag.HasErrors(ds) || len(ds) != 2 || ds[0].Code != "plugin.config" || ds[1].Code != "config.invalid" {
		t.Errorf("want plugin.config for the theme and config.invalid for enabled, got %v", ds)
	}
	for _, p := range set.Plugins() {
		if p.Name() == "syntaxHighlighting" {
			t.Error("plugin that failed Init is enabled")
		}
	}
}

func TestLoadDuplicateName(t *testing.T) {
	cfg := config.Defaults()
	set, ds := Load(cfg, plugin.SiteInfo{}, &fake{name: "dup", priority: 1}, &fake{name: "dup", priority: 2})
	if !diag.HasErrors(ds) || len(ds) != 1 || ds[0].Code != "plugin.config" || !strings.Contains(ds[0].Message, "dup: duplicate plugin name") {
		t.Errorf("diagnostics = %v", ds)
	}
	n := 0
	for _, p := range set.Plugins() {
		if f, ok := p.(*fake); ok && f.name == "dup" {
			n++
			if f.priority != 1 {
				t.Error("the later duplicate plugin was kept")
			}
		}
	}
	if n != 1 {
		t.Errorf("enabled dup plugins = %d, want 1", n)
	}
}

type fakeKeys struct {
	fake
	keys []string
}

func (f *fakeKeys) ConfigKeys() []string { return f.keys }

func TestLoadThirdPartyConfigKeys(t *testing.T) {
	cfg := config.Defaults()
	cfg.Plugins["hello"] = map[string]any{"greeting": "Hej", "greetng": "x"}
	_, ds := Load(cfg, plugin.SiteInfo{}, &fakeKeys{fake: fake{name: "hello"}, keys: []string{"greeting"}})
	if len(ds) != 1 || ds[0].Severity != diag.Warning || ds[0].Code != "config.unknownKey" ||
		!strings.Contains(ds[0].Message, `"plugins.hello.greetng"`) {
		t.Errorf("diagnostics = %v", ds)
	}
}

func TestDirectiveClaims(t *testing.T) {
	cfg := config.Defaults()
	set, _ := Load(cfg, plugin.SiteInfo{},
		&fakeDirective{fake{name: "a", dirs: []string{"youtube", "note"}}},
		&fakeDirective{fake{name: "b", dirs: []string{"youtube", "mermaid"}}},
	)
	m, ds := set.Directives()
	if len(m) != 2 || m["youtube"] == nil || m["mermaid"] == nil {
		t.Errorf("directives = %v", m)
	}
	if len(ds) != 2 || !strings.Contains(ds[0].Message, `"note" is built in`) || !strings.Contains(ds[1].Message, `already rendered by a`) {
		t.Errorf("diagnostics = %v", ds)
	}
}

func TestMermaid(t *testing.T) {
	m := &Mermaid{}
	if err := m.Init(nil, plugin.SiteInfo{}); err != nil {
		t.Fatal(err)
	}
	h := &Highlight{}
	if err := h.Init(nil, plugin.SiteInfo{}); err != nil {
		t.Fatal(err)
	}
	md := markdown.New(markdown.Options{Extenders: []plugin.MarkdownExtender{h, m}})
	src := []byte("```mermaid\ngraph LR\n  a[\"<b>\"] --> b\n```\n\n```go\nx := 1\n```\n")
	doc, _ := md.Parse(src)
	out, err := md.Render(doc, src)
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if !strings.Contains(html, "<pre class=\"mermaid\">graph LR\n  a[&quot;&lt;b&gt;&quot;] --&gt; b\n</pre>") || !strings.Contains(html, `<pre class="chroma">`) {
		t.Errorf("render:\n%s", html)
	}

	p := &plugin.Page{AST: doc, Source: src, Data: map[string]any{}, HTML: []byte("<html><body>x</body></html>")}
	if err := m.OnPageParsed(p); err != nil || p.Data["mermaid"] != true {
		t.Fatalf("mermaid not detected: %v", err)
	}
	if err := m.OnPageRendered(p, nil); err != nil {
		t.Fatal(err)
	}
	if got := string(p.HTML); !strings.Contains(got, `import mermaid from "`+DefaultMermaidURL+`";`) || !strings.HasSuffix(got, "</script>\n</body></html>") {
		t.Errorf("script not injected before </body>:\n%s", got)
	}

	plain := &plugin.Page{AST: parsePage(t, "no diagrams").AST, Data: map[string]any{}, HTML: []byte("<body></body>")}
	_ = m.OnPageParsed(plain)
	_ = m.OnPageRendered(plain, nil)
	if string(plain.HTML) != "<body></body>" {
		t.Error("script added to a page without diagrams")
	}

	evil := &Mermaid{}
	_ = evil.Init(map[string]any{"url": "x</script><script>alert(1)"}, plugin.SiteInfo{})
	ep := &plugin.Page{Data: map[string]any{"mermaid": true}, HTML: []byte("</body>")}
	_ = evil.OnPageRendered(ep, nil)
	if strings.Contains(string(ep.HTML), "</script><script>alert") {
		t.Error("url can break out of the script")
	}
}
