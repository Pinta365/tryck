package build_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
)

const versionedConfig = "site:\n  title: Docs\n  baseUrl: https://example.com/\nversions:\n  current: v2\n  older:\n    - name: v1\n"

func versionedSite(t *testing.T, extra map[string]string) *build.Site {
	t.Helper()
	files := map[string]string{
		"tryck.config.yaml":       versionedConfig,
		"content/index.md":        "# Home\n",
		"content/guide.md":        "# Guide\n\n[Setup](setup.md) and [home](/).\n",
		"content/setup.md":        "# Setup\n",
		"content/only-new.md":     "# New\n",
		"versions/v1/index.md":    "# Home\n",
		"versions/v1/guide.md":    "---\naliases: [/old-guide/]\n---\n# Old guide\n\n[Setup](setup.md), [root setup](/setup/), [robots](/robots.txt), [source](/setup.md).\n",
		"versions/v1/setup.md":    "# Old setup\n",
		"versions/v1/pic.png":     "png",
		"versions/v1/with-pic.md": "# Pic\n\n![p](pic.png) ![q](/pic.png)\n",
		"static/robots.txt":       "User-agent: *\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	return mustBuild(t, writeSite(t, files))
}

func TestVersionedURLsAndLinks(t *testing.T) {
	site := versionedSite(t, nil)
	for _, d := range site.Diags {
		if d.Severity > 0 {
			t.Errorf("unexpected diagnostic: %v", d)
		}
	}
	for _, path := range []string{"index.html", "guide/index.html", "only-new/index.html", "v1/index.html", "v1/guide/index.html", "v1/old-guide/index.html"} {
		if _, ok := site.Outputs[path]; !ok {
			t.Errorf("missing %s", path)
		}
	}
	if _, ok := site.Outputs["old-guide/index.html"]; ok {
		t.Error("an older version's alias must stay inside its version")
	}
	old := string(site.Outputs["v1/guide/index.html"])
	for _, want := range []string{
		`<a href="/v1/setup/">Setup</a>`,      // relative .md link, within the version
		`<a href="/v1/setup/">root setup</a>`, // root-absolute path to a page of the version
		`<a href="/robots.txt">robots</a>`,    // a shared static file stays at the root
		`<a href="/v1/setup/">source</a>`,     // /x.md is looked up in the version
	} {
		if !strings.Contains(old, want) {
			t.Errorf("missing %s in v1 guide", want)
		}
	}
	pic := string(site.Outputs["v1/with-pic/index.html"])
	if strings.Count(pic, `src="/v1/pic.png"`) != 2 {
		t.Errorf("version assets must resolve within the version:\n%s", pic)
	}
	if cur := string(site.Outputs["guide/index.html"]); !strings.Contains(cur, `<a href="/setup/">Setup</a>`) || !strings.Contains(cur, `<a href="/">home</a>`) {
		t.Errorf("current version links changed:\n%s", cur)
	}
}

func TestVersionedNavAndPicker(t *testing.T) {
	site := versionedSite(t, nil)
	old := string(site.Outputs["v1/guide/index.html"])
	// The sidebar and prev/next stay within the version.
	sidebar := old[strings.Index(old, `<nav class="sidebar"`):]
	sidebar = sidebar[:strings.Index(sidebar, "</nav>")]
	if strings.Contains(sidebar, `href="/guide/"`) || !strings.Contains(sidebar, `href="/v1/setup/"`) || strings.Contains(old, `/only-new/`) {
		t.Errorf("v1 nav mixes versions:\n%s", sidebar)
	}
	if !strings.Contains(old, `rel="next" href="/v1/setup/"`) {
		t.Errorf("v1 guide's next page must be v1's setup")
	}
	// The picker links to the same page in each version, or the version's home.
	for _, want := range []string{
		`<summary title="Documentation version">v1</summary>`,
		`<li><a href="/guide/">v2 <span class="versions-current">current</span></a></li>`,
		`<li><a href="/v1/guide/" aria-current="true">v1</a></li>`,
		`<meta name="robots" content="noindex">`,
		`This is the documentation for v1. <a href="/guide/">See the current version, v2</a>.`,
		`data-index="/v1/search-index.json"`,
	} {
		if !strings.Contains(old, want) {
			t.Errorf("missing %s in v1 guide", want)
		}
	}
	cur := string(site.Outputs["only-new/index.html"])
	if !strings.Contains(cur, `<li><a href="/v1/">v1</a></li>`) {
		t.Errorf("a page missing from v1 must link to v1's home:\n%s", cur)
	}
	if strings.Contains(cur, "noindex") || strings.Contains(cur, "version-banner") {
		t.Error("current pages must not have noindex or the old-version banner")
	}
}

func TestVersionedSitemapAndSearch(t *testing.T) {
	site := versionedSite(t, nil)
	if sm := string(site.Outputs["sitemap.xml"]); strings.Contains(sm, "/v1/") || !strings.Contains(sm, "https://example.com/guide/") {
		t.Errorf("the sitemap must list only the current version:\n%s", sm)
	}
	urls := func(file string) []string {
		var docs []struct{ URL string }
		if err := json.Unmarshal(site.Outputs[file], &docs); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		var out []string
		for _, d := range docs {
			out = append(out, d.URL)
		}
		return out
	}
	if got := strings.Join(urls("search-index.json"), " "); got != "/ /guide/ /only-new/ /setup/" {
		t.Errorf("current index = %s", got)
	}
	if got := strings.Join(urls("v1/search-index.json"), " "); got != "/v1/ /v1/guide/ /v1/setup/ /v1/with-pic/" {
		t.Errorf("v1 index = %s", got)
	}
}

func TestVersionedBrokenLinkAndConflict(t *testing.T) {
	site := versionedSite(t, map[string]string{
		"versions/v1/broken.md": "# B\n\n[gone](only-new.md)\n",
		"content/v1/index.md":   "# Clash\n",
	})
	if d, ok := findDiag(site.Diags, "link.broken"); !ok || d.File != "versions/v1/broken.md" || !strings.Contains(d.Message, "/v1/only-new/") {
		t.Errorf("want link.broken in versions/v1/broken.md, got %v", site.Diags)
	}
	if _, ok := findDiag(site.Diags, "url.conflict"); !ok {
		t.Errorf("content/v1/ must conflict with the v1 version, got %v", site.Diags)
	}
}

// Two versions share source paths; the dev cache must keep them apart.
func TestVersionedCache(t *testing.T) {
	cfg := writeSite(t, map[string]string{
		"tryck.config.yaml": versionedConfig,
		"content/a.md":      "# New A\n",
		"versions/v1/a.md":  "# Old A\n",
	})
	opts := build.Options{Cache: build.NewCache()}
	for range 2 {
		site, err := build.Build(context.Background(), cfg, opts)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(site.Outputs["a/index.html"]), "New A") || !strings.Contains(string(site.Outputs["v1/a/index.html"]), "Old A") {
			t.Fatal("versions with the same source path got mixed up")
		}
	}
}

func TestUnversionedSiteHasNoPicker(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{"content/a.md": "# A\n"}))
	a := string(site.Outputs["a/index.html"])
	for _, s := range []string{"versions", "version-banner", "noindex"} {
		if strings.Contains(a, s) {
			t.Errorf("unversioned page contains %q", s)
		}
	}
}

func TestMissingVersionDir(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{"tryck.config.yaml": versionedConfig, "content/a.md": "# A\n"}))
	if d, ok := findDiag(site.Diags, "config.invalid"); !ok || !strings.Contains(d.Message, "versions/v1 is not a directory; create it with tryck snapshot v1") {
		t.Errorf("want config.invalid for the missing version, got %v", site.Diags)
	}
}
