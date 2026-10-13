package build_test

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/output"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/sites/*/expected")

// writeSite creates a project in a temp dir from path -> content.
func writeSite(t *testing.T, files map[string]string) *config.Config {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, ds := config.Load(filepath.Join(root, config.DefaultPath), false)
	if diag.HasErrors(ds) {
		t.Fatalf("config: %v", ds)
	}
	cfg.Root = root
	return cfg
}

func mustBuild(t *testing.T, cfg *config.Config) *build.Site {
	t.Helper()
	site, err := build.Build(context.Background(), cfg, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return site
}

func findDiag(ds []diag.Diagnostic, code string) (diag.Diagnostic, bool) {
	for _, d := range ds {
		if d.Code == code {
			return d, true
		}
	}
	return diag.Diagnostic{}, false
}

func TestURLConflict(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/docs/setup.md":    "# A",
		"content/docs/01-setup.md": "# B",
	}))
	d, ok := findDiag(site.Diags, "url.conflict")
	if !ok || d.Severity != diag.Error {
		t.Fatalf("want url.conflict error, got %v", site.Diags)
	}
	if !strings.Contains(d.Message, "content/docs/setup.md") || !strings.Contains(d.Message, "content/docs/01-setup.md") {
		t.Errorf("message must name both files: %s", d.Message)
	}
}

func TestBadPageDoesNotStopOthers(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/a.md": "---\ntitle: [\n---\n# A\n",
		"content/b.md": "---\ntitle: B\n",
		"content/c.md": "# C\n",
	}))
	d, ok := findDiag(site.Diags, "frontmatter.yaml")
	if !ok || d.File != "content/a.md" || d.Line != 2 {
		t.Errorf("want frontmatter.yaml at content/a.md:2, got %v", site.Diags)
	}
	n := 0
	for _, d := range site.Diags {
		if d.Code == "frontmatter.yaml" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("want 2 frontmatter errors (invalid and unclosed), got %v", site.Diags)
	}
	if _, ok := site.Outputs["c/index.html"]; !ok {
		t.Error("valid page c.md was not rendered")
	}
	if !site.HasErrors() {
		t.Error("HasErrors() = false")
	}
}

func TestMissingLayout(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/a.md": "---\nlayout: fancy\n---\n# A\n",
	}))
	d, ok := findDiag(site.Diags, "layout.missing")
	if !ok || d.File != "content/a.md" || !strings.Contains(d.Message, `"fancy"`) || !strings.Contains(d.Message, "layouts/fancy.html") {
		t.Errorf("want layout.missing naming page and layout, got %v", site.Diags)
	}
}

func TestTemplateExecError(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"layouts/doc.html": `{{ define "main" }}{{ index .Page.Params "missing" 3 }}{{ end }}`,
		"content/a.md":     "# A\n",
	}))
	d, ok := findDiag(site.Diags, "template.exec")
	if !ok || d.File != "content/a.md" || !strings.Contains(d.Message, "doc.html") {
		t.Errorf("want template.exec naming the template, got %v", site.Diags)
	}
}

func TestDrafts(t *testing.T) {
	files := map[string]string{
		"content/a.md":     "# A\n",
		"content/draft.md": "---\ndraft: true\n---\n# D\n",
	}
	cfg := writeSite(t, files)
	if site := mustBuild(t, cfg); site.ByURL["/draft/"] != nil || len(site.Pages) != 1 {
		t.Errorf("draft included by default: %d pages", len(site.Pages))
	}
	cfg.Build.Drafts = true
	if site := mustBuild(t, cfg); site.ByURL["/draft/"] == nil {
		t.Error("draft excluded with build.drafts")
	}
}

func TestTitleFallbacks(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml":           "site:\n  title: My Site\n",
		"content/index.md":            "No heading here.\n",
		"content/fm.md":               "---\ntitle: From FM\n---\n# From H1\n",
		"content/h1.md":               "Intro\n\n# From *H1*\n",
		"content/02-from-filename.md": "## Only h2\n",
	}))
	for url, want := range map[string]string{"/": "My Site", "/fm/": "From FM", "/h1/": "From H1", "/from-filename/": "From filename"} {
		if p := site.ByURL[url]; p == nil || p.Title != want {
			t.Errorf("%s title = %v, want %q", url, p, want)
		}
	}
}

func TestEmptySiteEmits404(t *testing.T) {
	site := mustBuild(t, writeSite(t, nil))
	if len(site.Diags) != 0 {
		t.Errorf("diagnostics %v", site.Diags)
	}
	if !strings.Contains(string(site.Outputs[build.NotFoundPath]), "Page not found") {
		t.Errorf("404.html missing: %v", site.Outputs)
	}
}

func TestAssetConflict(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/docs/_index.md":  "# Docs\n",
		"content/docs/index.html": "<p>hand written</p>",
	}))
	if _, ok := findDiag(site.Diags, "url.conflict"); !ok {
		t.Errorf("want url.conflict for asset over page, got %v", site.Diags)
	}
}

func TestCancelled(t *testing.T) {
	cfg := writeSite(t, map[string]string{"content/a.md": "# A\n"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := build.Build(ctx, cfg, build.Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// TestGolden builds each testdata/sites/*/input and compares the output
// tree with expected/. Run with -update (or TRYCK_UPDATE=1) to rewrite.
func TestGolden(t *testing.T) {
	sites, err := filepath.Glob("../../testdata/sites/*/input")
	if err != nil || len(sites) == 0 {
		t.Fatalf("no golden sites: %v", err)
	}
	for _, input := range sites {
		name := filepath.Base(filepath.Dir(input))
		t.Run(name, func(t *testing.T) {
			cfg, ds := config.Load(filepath.Join(input, config.DefaultPath), true)
			if diag.HasErrors(ds) {
				t.Fatalf("config: %v", ds)
			}
			out := t.TempDir()
			cfg.Apply(config.Overrides{OutputDir: &out})
			site := mustBuild(t, cfg)
			if site.HasErrors() {
				t.Fatalf("build errors: %v", site.Diags)
			}
			if _, err := output.Write(site); err != nil {
				t.Fatal(err)
			}

			expected := filepath.Join(filepath.Dir(input), "expected")
			if *update || os.Getenv("TRYCK_UPDATE") == "1" {
				if err := os.RemoveAll(expected); err != nil {
					t.Fatal(err)
				}
				if err := os.CopyFS(expected, os.DirFS(out)); err != nil {
					t.Fatal(err)
				}
				return
			}
			compareTrees(t, expected, out)
		})
	}
}

func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("read %s: %v (run with -update to create goldens)", dir, err)
	}
	return files
}

func compareTrees(t *testing.T, wantDir, gotDir string) {
	t.Helper()
	want, got := readTree(t, wantDir), readTree(t, gotDir)
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("missing output %s", name)
			continue
		}
		if g != w {
			t.Errorf("%s differs from golden (run with -update to accept):\n--- got\n%s\n--- want\n%s", name, g, w)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected output %s", name)
		}
	}
}
