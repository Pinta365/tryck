package build_test

import (
	"context"
	"errors"
	"flag"
	"maps"
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
	if _, ok := files[config.DefaultPath]; !ok {
		files = maps.Clone(files)
		if files == nil {
			files = map[string]string{}
		}
		files[config.DefaultPath] = "site:\n  baseUrl: https://example.com/\n"
	}
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
		"tryck.config.yaml":           "site:\n  title: My Site\n  baseUrl: https://example.com/\n",
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

func TestNavAndPrevNext(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/index.md":           "# Home\n",
		"content/02-b.md":            "# B\n",
		"content/01-guide/_index.md": "---\ntitle: Guide\n---\n",
		"content/01-guide/01-a.md":   "---\nnavTitle: Short A\n---\n# A long title\n",
	}))
	if len(site.Diags) != 0 {
		t.Fatalf("diagnostics %v", site.Diags)
	}
	a := string(site.Outputs["guide/a/index.html"])
	for _, want := range []string{
		`<a class="pager-prev" rel="prev" href="/guide/"><span>Previous</span>Guide</a>`,
		`<a class="pager-next" rel="next" href="/b/"><span>Next</span>B</a>`,
		`<a href="/guide/a/" aria-current="page">Short A</a>`,
		`<li class="in-path"><a href="/guide/">Guide</a>`,
		`<title>A long title</title>`,
	} {
		if !strings.Contains(a, want) {
			t.Errorf("guide/a missing %q:\n%s", want, a)
		}
	}
	home := string(site.Outputs["index.html"])
	if strings.Contains(home, `rel="prev"`) || !strings.Contains(home, `rel="next" href="/guide/"`) {
		t.Errorf("home pager wrong:\n%s", home)
	}
	if b := string(site.Outputs["b/index.html"]); strings.Contains(b, `rel="next"`) {
		t.Error("last page has a next link")
	}
}

func TestDraftBanner(t *testing.T) {
	cfg := writeSite(t, map[string]string{"content/d.md": "---\ndraft: true\n---\n# D\n"})
	cfg.Build.Drafts = true
	site := mustBuild(t, cfg)
	if !strings.Contains(string(site.Outputs["d/index.html"]), `class="draft-banner"`) {
		t.Error("draft page has no banner")
	}
}

func TestDuplicateHeadingDiag(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/a.md": "---\ntitle: A\n---\n\n## Setup\n\ntext\n\n## Setup\n",
	}))
	d, ok := findDiag(site.Diags, "heading.duplicateId")
	if !ok || d.Severity != diag.Info || d.File != "content/a.md" || d.Line != 9 || !strings.Contains(d.Message, `"setup-1"`) {
		t.Errorf("want info at content/a.md:9 about setup-1, got %v", site.Diags)
	}
}

func TestDirectiveDiagnosticsUseFileLines(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/a.md": "---\ntitle: A\n---\n\nIntro.\n\n:::warning\nnever closed\n",
		"content/b.md": ":::youtube\nx\n:::\n",
	}))
	d, ok := findDiag(site.Diags, "directive.unclosed")
	if !ok || d.File != "content/a.md" || d.Line != 7 || d.Severity != diag.Error {
		t.Errorf("want directive.unclosed error at content/a.md:7, got %v", site.Diags)
	}
	d, ok = findDiag(site.Diags, "directive.unknown")
	if !ok || d.File != "content/b.md" || d.Line != 1 || d.Severity != diag.Warning {
		t.Errorf("want directive.unknown warning at content/b.md:1, got %v", site.Diags)
	}
}

func TestHeadAndBodyEndPartials(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"layouts/partials/head-end.html": `{{ if not .Dev }}<script defer src="/a.js"></script>{{ end }}`,
		"layouts/partials/body-end.html": `<script>end()</script>`,
		"content/a.md":                   "# A\n",
	}))
	for _, path := range []string{"a/index.html", build.NotFoundPath} {
		html := string(site.Outputs[path])
		if !strings.Contains(html, "<script defer src=\"/a.js\"></script>\n</head>") || !strings.Contains(html, "<script>end()</script>\n</body>") {
			t.Errorf("%s: partials not at the end of head and body:\n%s", path, html)
		}
	}
}

func TestSearchBox(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{"content/a.md": "# A\n\n## Install it {#setup}\n"}))
	a := string(site.Outputs["a/index.html"])
	for _, want := range []string{`<script src="/js/search.js" defer></script>`, `<div class="search" data-index="/search-index.json" hidden>`} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %q", want)
		}
	}
	if _, ok := site.Outputs["js/search.js"]; !ok {
		t.Error("js/search.js is not written")
	}
	if idx := string(site.Outputs["search-index.json"]); !strings.Contains(idx, `"headings":[{"id":"setup","text":"Install it"}]`) {
		t.Errorf("headings need ids: %s", idx)
	}

	off := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\nplugins:\n  search:\n    enabled: false\n",
		"content/a.md":      "# A\n",
	}))
	if a := string(off.Outputs["a/index.html"]); strings.Contains(a, "search") {
		t.Errorf("search is off but the page has a search box or script:\n%s", a)
	}
}
