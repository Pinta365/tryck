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
	if css := string(out2.files[chromaCSSPath]); !strings.Contains(css, ".chroma") {
		t.Errorf("chroma.css = %q", css)
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
	if got := strings.Join(names, ","); got != "aa,zz,toc,redirects,search,sitemap,syntaxHighlighting,late,linkCheck" {
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
	_ = set
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
