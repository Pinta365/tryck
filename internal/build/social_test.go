package build_test

import (
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/diag"
)

func TestSocial(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  title: Demo\n  description: A demo.\n  baseUrl: https://example.com/docs/\n" +
			"plugins:\n  social:\n    image: /img/card.png\n    twitter: pinta365\n",
		"static/img/card.png": encodePNG(t, 1200, 630),
		"content/a.md":        "# A & B\n",
		"content/post.md":     "---\ndate: 2026-03-01\ndescription: News.\nimage: /img/missing.png\n---\n# Post\n",
		"content/bad.md":      "---\nimage: card.png\n---\n# Bad\n",
	}))
	a := string(site.Outputs["a/index.html"])
	want := `<meta property="og:type" content="website">
<meta property="og:site_name" content="Demo">
<meta property="og:title" content="A &amp; B">
<meta property="og:description" content="A demo.">
<meta property="og:url" content="https://example.com/docs/a/">
<meta property="og:image" content="https://example.com/docs/img/card.png">
<meta property="og:image:width" content="1200">
<meta property="og:image:height" content="630">
<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:site" content="@pinta365">
`
	if !strings.Contains(a, "</title>\n"+want) {
		t.Errorf("a/index.html tags:\n%s\nwant after the title:\n%s", a, want)
	}

	post := string(site.Outputs["post/index.html"])
	for _, s := range []string{
		`<meta property="og:type" content="article">`,
		`<meta property="og:description" content="News.">`,
		`<meta property="og:image" content="https://example.com/docs/img/missing.png">`,
		`<meta property="article:published_time" content="2026-03-01T00:00:00Z">`,
	} {
		if !strings.Contains(post, s) {
			t.Errorf("post/index.html: missing %s", s)
		}
	}
	if strings.Contains(post, "og:image:width") {
		t.Error("a missing image has a size")
	}
	if !strings.Contains(string(site.Outputs["bad/index.html"]), `content="https://example.com/docs/img/card.png"`) {
		t.Error("a page with an invalid image does not fall back to the site image")
	}

	var got []string
	for _, d := range site.Diags {
		got = append(got, d.Code+" "+d.File+" "+d.Message)
	}
	all := strings.Join(got, "\n")
	if len(got) != 2 || !strings.Contains(all, "social.image content/bad.md front matter image \"card.png\" must be") ||
		!strings.Contains(all, "social.image  card image /img/missing.png does not exist (used by /post/)") {
		t.Errorf("diagnostics:\n%s", strings.Join(got, "\n"))
	}
}

func TestSocialNeedsHost(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: /\nplugins:\n  sitemap:\n    enabled: false\n",
		"content/a.md":      "# A\n",
	}))
	if strings.Contains(string(site.Outputs["a/index.html"]), "og:") {
		t.Error("tags without absolute URLs")
	}
	if d, ok := findDiag(site.Diags, "social.baseUrl"); !ok || d.Severity != diag.Info {
		t.Errorf("want a social.baseUrl info, got %v", site.Diags)
	}
}

func TestSocialConfig(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\nplugins:\n  social:\n    image: img/card.png\n",
		"content/a.md":      "# A\n",
	}))
	if d, ok := findDiag(site.Diags, "plugin.config"); !ok || !strings.Contains(d.Message, `image: "img/card.png" must be a site path`) {
		t.Errorf("want a plugin.config error, got %v", site.Diags)
	}
}
