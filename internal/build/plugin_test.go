package build_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// probe exercises every phase.
type probe struct {
	anchorOK, anchorMissing bool
	pages                   int
}

func (*probe) Name() string                               { return "probe" }
func (*probe) Init(map[string]any, plugin.SiteInfo) error { return nil }

func (*probe) OnPageParsed(p *plugin.Page) error {
	p.Data["probe"] = "parsed " + p.SourcePath
	if strings.Contains(string(p.Source), "FAIL") {
		return errors.New("refused")
	}
	if strings.Contains(string(p.Source), "WARN") {
		p.Diags.Report(plugin.Diagnostic{Severity: plugin.Warning, Line: p.LineAt(0), Code: "probe.warn", Message: "warned"})
	}
	return nil
}

func (*probe) OnPageRendered(p *plugin.Page, s plugin.SiteView) error {
	p.HTML = append(p.HTML, fmt.Sprintf("<!-- %d pages -->", len(s.Pages()))...)
	return nil
}

func (pr *probe) OnBuildComplete(s plugin.SiteView, out plugin.Outputs) error {
	pr.pages = len(s.Pages())
	pr.anchorOK = s.HasAnchor("/a/", "section-one")
	pr.anchorMissing = s.HasAnchor("/a/", "nope")
	if err := out.Add("probe/list.txt", []byte("ok")); err != nil {
		return err
	}
	for _, bad := range []string{"../escape.txt", "", "a/../b", "a/", "index.html", "a//b"} {
		if err := out.Add(bad, nil); err == nil {
			return fmt.Errorf("Add(%q) accepted", bad)
		}
	}
	return nil
}

func (*probe) DirectiveNames() []string { return []string{"shout"} }

func (*probe) RenderDirective(w io.Writer, d plugin.Directive, entering bool) error {
	if entering {
		_, err := fmt.Fprintf(w, "<div class=\"shout\" data-k=%q>%s\n", d.Attrs["k"], strings.ToUpper(d.Title))
		return err
	}
	_, err := io.WriteString(w, "</div>\n")
	return err
}

func TestPluginPhases(t *testing.T) {
	cfg := writeSite(t, map[string]string{
		"layouts/doc.html": `{{ define "main" }}[{{ .Page.Data.probe }}]{{ .Content }}{{ end }}`,
		"content/index.md": "# Home\n",
		"content/a.md":     "---\ntitle: A\n---\nWARN\n\n## Section one\n\n:::shout[hi]{k=v}\nbody\n:::\n",
		"content/b.md":     "FAIL\n",
	})
	pr := &probe{}
	site, err := build.Build(context.Background(), cfg, build.Options{Plugins: []plugin.Plugin{pr}})
	if err != nil {
		t.Fatal(err)
	}

	a := string(site.Outputs["a/index.html"])
	for _, want := range []string{"[parsed a.md]", "<!-- 3 pages -->", "<div class=\"shout\" data-k=\"v\">HI\n<p>body</p>\n</div>"} {
		if !strings.Contains(a, want) {
			t.Errorf("a/index.html missing %q:\n%s", want, a)
		}
	}
	if string(site.Outputs["probe/list.txt"]) != "ok" {
		t.Error("finalizer output missing")
	}
	if pr.pages != 3 || !pr.anchorOK || pr.anchorMissing {
		t.Errorf("site view: pages=%d anchorOK=%v anchorMissing=%v", pr.pages, pr.anchorOK, pr.anchorMissing)
	}

	var got []string
	for _, d := range site.Diags {
		got = append(got, diag.Format(d))
	}
	want := []string{
		"content/a.md:4: warning[probe.warn]: warned",
		"content/b.md: error[plugin.error]: plugin probe: refused",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if _, ok := site.Outputs["b/index.html"]; !ok {
		t.Error("page whose plugin failed should still render")
	}
}

func TestTOCAndReadingTimeInTemplates(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml":  "site:\n  baseUrl: https://example.com/\ndefaults:\n  tocDepth: [2, 2]\n",
		"layouts/post.html":  `{{ define "main" }}{{ .Page.Data.readingTime }}min|{{ range .TOC }}{{ .ID }};{{ end }}{{ end }}`,
		"content/p.md":       "---\nlayout: post\n---\n## One\n\n### Skipped\n\n## Two\n",
		"content/no-toc.md":  "---\ntoc: false\n---\n## One\n",
		"content/default.md": "## One\n",
	}))
	if p := string(site.Outputs["p/index.html"]); !strings.Contains(p, "1min|one;two;") {
		t.Errorf("post output: %s", p)
	}
	if strings.Contains(string(site.Outputs["no-toc/index.html"]), `class="toc"`) {
		t.Error("toc: false still renders a toc")
	}
	if !strings.Contains(string(site.Outputs["default/index.html"]), `<a href="#one">One</a>`) {
		t.Error("default doc layout has no toc")
	}
}

func TestDisabledHighlighting(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\nplugins:\n  syntaxHighlighting:\n    enabled: false\n",
		"content/a.md":      "```go\nx := 1\n```\n",
	}))
	a := string(site.Outputs["a/index.html"])
	if strings.Contains(a, "chroma") || !strings.Contains(a, `<code class="language-go">`) {
		t.Errorf("highlighting not disabled:\n%s", a)
	}
	if _, ok := site.Outputs["css/chroma.css"]; ok {
		t.Error("chroma.css written while disabled")
	}
}

func TestLinkCheck(t *testing.T) {
	files := map[string]string{
		"tryck.config.yaml":        "site:\n  baseUrl: https://example.com/docs/\n",
		"static/img/logo.png":      "png",
		"content/index.md":         "# Home\n",
		"content/guide/install.md": "# Install\n\n## Step two\n",
		"content/guide/draft.md":   "---\ndraft: true\n---\n# Draft\n",
		"content/guide/shot.png":   "png",
		"content/guide/a.md": `# A

[ok](install.md) [ok anchor](install.md#step-two) [own anchor](#a) ![static](/img/logo.png) ![asset](shot.png)
[ok abs](https://example.com/docs/guide/install/) [external](https://other.example/x) [sitemap](/sitemap.xml)

[typo](/guide/instal/)
[missing md](../nope.md)
[bad anchor](install.md#step-three)
[bad own anchor](#nowhere)
[draft](draft.md)
![missing image](missing.png)
[abs broken](https://example.com/docs/gone/)
`,
	}
	cfg := writeSite(t, files)
	site := mustBuild(t, cfg)
	var got []string
	for _, d := range site.Diags {
		got = append(got, diag.Format(d))
	}
	want := []string{
		"content/guide/a.md:6: error[link.broken]: /guide/instal/ does not exist (did you mean /guide/install/?)",
		"content/guide/a.md:7: error[link.broken]: /nope/ does not exist",
		"content/guide/a.md:8: warning[link.anchor]: #step-three not found on /guide/install/",
		"content/guide/a.md:9: warning[link.anchor]: #nowhere not found on /guide/a/",
		"content/guide/a.md:10: error[link.broken]: /guide/draft/ does not exist",
		"content/guide/a.md:11: error[link.broken]: /guide/missing.png does not exist",
		"content/guide/a.md:12: error[link.broken]: /gone/ does not exist",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	cfg.Build.Drafts = true
	site = mustBuild(t, cfg)
	for _, d := range site.Diags {
		if strings.Contains(d.Message, "/guide/draft/") {
			t.Errorf("draft link reported although drafts are built: %s", diag.Format(d))
		}
	}

	dev, err := build.Build(context.Background(), cfg, build.Options{Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	if dev.HasErrors() {
		t.Errorf("dev build: broken links must be warnings, got %v", dev.Diags)
	}

	files["tryck.config.yaml"] += "plugins:\n  linkCheck:\n    failOnBroken: false\n"
	if site := mustBuild(t, writeSite(t, files)); site.HasErrors() {
		t.Errorf("failOnBroken: false must not fail the build: %v", site.Diags)
	}
}

func TestSiteOutputs(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/sub/\nplugins:\n  search:\n    maxTextLength: 12\n",
		"content/index.md":  "---\ndate: 2026-10-13\n---\n# Home\n\nÅäö åäö åäö åäö\n",
		"content/old.md":    "---\naliases: [/legacy/, /older.html, relative/]\n---\n# Old\n",
		"content/draft.md":  "---\ndraft: true\n---\n# Draft\n",
		"content/clash.md":  "---\naliases: [/old/]\n---\n# Clash\n",
	}))
	sitemap := string(site.Outputs["sitemap.xml"])
	for _, want := range []string{"<loc>https://example.com/sub/</loc>\n    <lastmod>2026-10-13</lastmod>", "<loc>https://example.com/sub/old/</loc>"} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap missing %q:\n%s", want, sitemap)
		}
	}
	if strings.Contains(sitemap, "draft") {
		t.Error("sitemap lists a draft")
	}
	if idx := string(site.Outputs["search-index.json"]); !strings.Contains(idx, `"url":"/sub/","title":"Home","headings":[],"text":"Home Åäö"}`) {
		t.Errorf("search index: %s", idx)
	}
	for _, p := range []string{"legacy/index.html", "older.html"} {
		if !strings.Contains(string(site.Outputs[p]), `url=/sub/old/`) {
			t.Errorf("redirect %s: %s", p, site.Outputs[p])
		}
	}
	var got []string
	for _, d := range site.Diags {
		got = append(got, diag.Format(d))
	}
	want := []string{
		`error[redirect.alias]: alias /old/ of /clash/: output path "old/index.html" is already produced`,
		`error[redirect.alias]: alias "relative/" of /old/ must start with /`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSitemapNeedsAbsoluteBaseURL(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: /\n",
		"content/a.md":      "# A\n",
	}))
	if _, ok := site.Outputs["sitemap.xml"]; ok {
		t.Error("sitemap written without absolute baseUrl")
	}
	if d, ok := findDiag(site.Diags, "sitemap.baseUrl"); !ok || d.Severity != diag.Warning {
		t.Errorf("want sitemap.baseUrl warning, got %v", site.Diags)
	}
}
