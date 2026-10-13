package render

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/diag"
)

func TestRelAbsURL(t *testing.T) {
	tests := []struct{ base, p, rel, abs string }{
		{"/", "/docs/", "/docs/", "/docs/"},
		{"/", "docs/", "/docs/", "/docs/"},
		{"https://offgrid.dev/", "/docs/", "/docs/", "https://offgrid.dev/docs/"},
		{"https://user.github.io/repo/", "/docs/", "/repo/docs/", "https://user.github.io/repo/docs/"},
		{"https://user.github.io/repo/", "/", "/repo/", "https://user.github.io/repo/"},
		{"https://user.github.io/repo/", "css/a.css", "/repo/css/a.css", "https://user.github.io/repo/css/a.css"},
		{"https://offgrid.dev/", "https://example.com/x", "https://example.com/x", "https://example.com/x"},
		{"http://127.0.0.1:8080/", "/a/", "/a/", "http://127.0.0.1:8080/a/"},
	}
	for _, tt := range tests {
		if got := RelURL(tt.base, tt.p); got != tt.rel {
			t.Errorf("RelURL(%q, %q) = %q, want %q", tt.base, tt.p, got, tt.rel)
		}
		if got := AbsURL(tt.base, tt.p); got != tt.abs {
			t.Errorf("AbsURL(%q, %q) = %q, want %q", tt.base, tt.p, got, tt.abs)
		}
	}
}

func writeLayouts(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func execute(t *testing.T, tm *Templates, layout string) string {
	t.Helper()
	var buf bytes.Buffer
	data := &TemplateData{
		Site:    SiteInfo{Title: "Site", BaseURL: "https://u.github.io/repo/", Language: "en"},
		Page:    &PageInfo{Title: "Page <1>", URL: "/p/"},
		Content: "<p>body</p>",
	}
	if err := tm.Execute(&buf, layout, data); err != nil {
		t.Fatalf("Execute(%q): %v", layout, err)
	}
	return buf.String()
}

func TestEmbeddedDefaults(t *testing.T) {
	tm, ds := Load(filepath.Join(t.TempDir(), "missing"), "layouts", Funcs("https://u.github.io/repo/"))
	if len(ds) != 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	out := execute(t, tm, "doc")
	for _, want := range []string{
		"<title>Page &lt;1&gt; · Site</title>", // front matter strings are escaped
		`<article class="doc">`,
		"<p>body</p>", // content is trusted
		`href="/repo/"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("doc output missing %q:\n%s", want, out)
		}
	}
	if out := execute(t, tm, "404"); !strings.Contains(out, "<title>Page not found · Site</title>") {
		t.Errorf("404 output:\n%s", out)
	}
}

func TestUserLayoutsOverride(t *testing.T) {
	dir := writeLayouts(t, map[string]string{
		"doc.html":             `{{ define "main" }}<div class="mine">{{ .Content }}</div>{{ partial }}{{ end }}`,
		"landing.html":         `<!doctype html><h1>{{ .Page.Title }}</h1>`,
		"partials/footer.html": `<footer>f</footer>`,
	})
	// A stand-in for M3's partial function.
	funcs := Funcs("/")
	funcs["partial"] = func() string { return "[partial]" }
	tm, ds := Load(dir, "layouts", funcs)
	if len(ds) != 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	if out := execute(t, tm, "doc"); !strings.Contains(out, `<div class="mine"><p>body</p></div>[partial]`) || !strings.Contains(out, "<!DOCTYPE html>") {
		t.Errorf("doc should fill the embedded baseof:\n%s", out)
	}
	if out := execute(t, tm, "landing"); out != "<!doctype html><h1>Page &lt;1&gt;</h1>" {
		t.Errorf("standalone layout = %q", out)
	}
	if !tm.Has("404") {
		t.Error("embedded 404 layout lost when user layouts exist")
	}
	if tm.Has("partials/footer") || tm.Has("baseof") {
		t.Error("partials and baseof must not be page layouts")
	}
}

func TestMissingLayout(t *testing.T) {
	tm, _ := Load(t.TempDir(), "layouts", Funcs("/"))
	err := tm.Execute(&bytes.Buffer{}, "nope", &TemplateData{})
	if !errors.Is(err, ErrMissingLayout) {
		t.Errorf("err = %v, want ErrMissingLayout", err)
	}
}

func TestParseErrorDiagnostic(t *testing.T) {
	dir := writeLayouts(t, map[string]string{"doc.html": "line 1\n{{ define \"main\" }}\n{{ if }}x{{ end }}\n{{ end }}"})
	tm, ds := Load(dir, "site/layouts", Funcs("/"))
	if len(ds) != 1 {
		t.Fatalf("diagnostics = %v", ds)
	}
	d := ds[0]
	if d.Severity != diag.Error || d.Code != "template.parse" || d.File != "site/layouts/doc.html" || d.Line != 3 {
		t.Errorf("got %s", diag.Format(d))
	}
	if tm.Has("doc") {
		t.Error("broken layout should not be usable")
	}
}
