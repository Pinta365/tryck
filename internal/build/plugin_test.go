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
		"tryck.config.yaml":  "defaults:\n  tocDepth: [2, 2]\n",
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
		"tryck.config.yaml": "plugins:\n  syntaxHighlighting:\n    enabled: false\n",
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
