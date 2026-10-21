package build_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
)

func TestDirectiveTemplates(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  title: Docs\n  baseUrl: https://example.com/\n",
		"layouts/directives/card.html": `<div class="card {{ .Attrs.class }}"{{ with .Attrs.id }} id="{{ . }}"{{ end }}>` +
			`{{ with .Title }}<h3>{{ . }}</h3>{{ end }}{{ .Content }}{{ partial "card-foot" . }}</div>`,
		"layouts/partials/card-foot.html": `<small>{{ .Name }} on {{ .Site.Title }}</small>`,
		"layouts/directives/youtube.html": `<iframe src="https://www.youtube.com/embed/{{ .Attrs.id }}"></iframe>`,
		"content/a.md": "# A\n\n:::card[Hello <b>]{#c1 .wide}\nBody with *emphasis*.\n\n:::card\nInner\n:::\n:::\n\n" +
			":::youtube{id=abc}\nnot shown\n:::\n",
	}))
	for _, d := range site.Diags {
		if d.Severity > 0 {
			t.Errorf("unexpected diagnostic: %v", d)
		}
	}
	html := string(site.Outputs["a/index.html"])
	for _, want := range []string{
		`<div class="card wide" id="c1"><h3>Hello &lt;b&gt;</h3>`,
		"<p>Body with <em>emphasis</em>.</p>",
		`<div class="card "><p>Inner</p>` + "\n" + `<small>card on Docs</small></div>`,
		`<iframe src="https://www.youtube.com/embed/abc"></iframe>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
	if strings.Contains(html, "not shown") {
		t.Error("a template without .Content must drop the body")
	}
}

func TestDirectiveTemplateErrors(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"layouts/directives/note.html":   `<div>{{ .Content }}</div>`,
		"layouts/directives/9bad.html":   `<div>{{ .Content }}</div>`,
		"layouts/directives/broken.html": `{{ index .Attrs "x" 1 }}`,
		"content/a.md":                   ":::broken\nx\n:::\n",
	}))
	for _, want := range []string{
		`layouts/directives/note.html: error[template.parse]: "note" is a built-in directive`,
		`layouts/directives/9bad.html: error[template.parse]: "9bad" is not a directive name`,
		"content/a.md: error[markdown.render]: :::broken: template: directives/broken.html:1:3",
	} {
		found := false
		for _, d := range site.Diags {
			found = found || strings.HasPrefix(d.String(), want)
		}
		if !found {
			t.Errorf("missing %q in %v", want, site.Diags)
		}
	}
}

// A directive template added while tryck dev runs clears the
// directive.unknown warning of a cached page.
func TestDirectiveTemplateWithCache(t *testing.T) {
	cfg := writeSite(t, map[string]string{"content/a.md": ":::card\nx\n:::\n"})
	opts := build.Options{Cache: build.NewCache()}
	site, err := build.Build(context.Background(), cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findDiag(site.Diags, "directive.unknown"); !ok {
		t.Fatalf("want directive.unknown, got %v", site.Diags)
	}
	p := filepath.Join(cfg.Root, "layouts", "directives", "card.html")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`<div class="card">{{ .Content }}</div>`), 0o644); err != nil {
		t.Fatal(err)
	}
	site, err = build.Build(context.Background(), cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findDiag(site.Diags, "directive.unknown"); ok {
		t.Errorf("directive.unknown after adding the template: %v", site.Diags)
	}
	if !strings.Contains(string(site.Outputs["a/index.html"]), `<div class="card"><p>x</p>`) {
		t.Errorf("template not used:\n%s", site.Outputs["a/index.html"])
	}
}

// href and src on a custom directive are rewritten and checked like
// Markdown links.
func TestDirectiveLinkAttributes(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml":            "site:\n  baseUrl: https://example.com/docs/\n",
		"layouts/directives/card.html": `<a class="card" href="{{ .Attrs.href }}"><img src="{{ .Attrs.src }}">{{ .Content }}</a>`,
		"content/guide/a.md":           "# A\n\n:::card{href=\"../b.md#more\" src=\"pic.png\"}\nok\n:::\n\ntext\n\n:::card{href=\"../missing.md\"}\nbroken\n:::\n",
		"content/guide/pic.png":        "png",
		"content/b.md":                 "# B\n\n## More\n",
	}))
	a := string(site.Outputs["guide/a/index.html"])
	if !strings.Contains(a, `<a class="card" href="/docs/b/#more"><img src="/docs/guide/pic.png">`) {
		t.Errorf("link attributes not rewritten:\n%s", a)
	}
	d, ok := findDiag(site.Diags, "link.broken")
	if !ok || d.File != "content/guide/a.md" || d.Line != 9 || !strings.Contains(d.Message, "/missing/") {
		t.Errorf("want link.broken for the second card on line 9, got %v", site.Diags)
	}
	if n := len(site.Diags); n != 1 {
		t.Errorf("want only the broken link, got %v", site.Diags)
	}
}
