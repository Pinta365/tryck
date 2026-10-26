package build_test

import (
	"strings"
	"testing"
)

func feedSite(t *testing.T, config string) map[string]string {
	t.Helper()
	files := map[string]string{
		"tryck.config.yaml":      "site:\n  title: Docs\n  baseUrl: https://example.com/\n" + config,
		"content/index.md":       "# Home\n",
		"content/guide.md":       "# Guide without a date\n",
		"content/blog/_index.md": "---\ntitle: Blog\n---\n",
		"content/blog/first.md":  "---\ntitle: First post\ndate: 2026-01-05\n---\n\nThe *first* paragraph.\n\nMore.\n",
		"content/blog/second.md": "---\ntitle: Second post\ndate: 2026-03-01\ndescription: About the second\n---\n\nIgnored, the description wins.\n",
		"content/blog/draft.md":  "---\ntitle: Draft\ndate: 2026-04-01\ndraft: true\n---\n",
		"content/news.md":        "---\ntitle: News & notes\ndate: 2025-12-24\n---\n\nSite news.\n",
	}
	site := mustBuild(t, writeSite(t, files))
	for _, d := range site.Diags {
		if d.Severity > 0 && d.Code != "feed.section" {
			t.Errorf("unexpected diagnostic: %v", d)
		}
	}
	out := map[string]string{}
	for k, v := range site.Outputs {
		out[k] = string(v)
	}
	return out
}

func TestFeedAtom(t *testing.T) {
	out := feedSite(t, "")
	atom := out["feed.xml"]
	for _, want := range []string{
		`<feed xmlns="http://www.w3.org/2005/Atom">`,
		`<title>Docs</title>`,
		`<link href="https://example.com/feed.xml" rel="self"></link>`,
		`<updated>2026-03-01T00:00:00Z</updated>`,
		`<name>Docs</name>`,
		`<summary>About the second</summary>`,
		`<summary>The first paragraph.</summary>`,
		`<title>News &amp; notes</title>`,
	} {
		if !strings.Contains(atom, want) {
			t.Errorf("missing %s in:\n%s", want, atom)
		}
	}
	// Newest first; undated pages and drafts are left out.
	second, first, news := strings.Index(atom, "Second post"), strings.Index(atom, "First post"), strings.Index(atom, "News")
	if !(second < first && first < news) || strings.Contains(atom, "Guide") || strings.Contains(atom, "Draft") {
		t.Errorf("entries wrong or out of order:\n%s", atom)
	}
	if !strings.Contains(out["guide/index.html"], `<link rel="alternate" type="application/atom+xml" title="Docs" href="/feed.xml">`+"\n</head>") {
		t.Error("pages must link the feed in <head>")
	}
}

func TestFeedSectionsRSSAndLimit(t *testing.T) {
	out := feedSite(t, "plugins:\n  feed:\n    sections: [/blog/, /missing/]\n    format: rss\n    limit: 1\n")
	if _, ok := out["feed.xml"]; ok {
		t.Error("only the listed sections get feeds")
	}
	rss := out["blog/feed.xml"]
	for _, want := range []string{`<rss version="2.0">`, `<title>Docs · Blog</title>`, `<link>https://example.com/blog/</link>`, `<pubDate>Sun, 01 Mar 2026 00:00:00 +0000</pubDate>`} {
		if !strings.Contains(rss, want) {
			t.Errorf("missing %s in:\n%s", want, rss)
		}
	}
	if strings.Contains(rss, "First post") {
		t.Error("limit: 1 must keep only the newest entry")
	}
	if strings.Contains(out["guide/index.html"], "application/rss+xml") || !strings.Contains(out["blog/first/index.html"], `href="/blog/feed.xml"`) {
		t.Error("only pages in the section link its feed")
	}
}

func TestFeedNothingToDo(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{"content/a.md": "# A\n"}))
	if _, ok := site.Outputs["feed.xml"]; ok || strings.Contains(string(site.Outputs["a/index.html"]), "application/atom+xml") {
		t.Error("a site without dated pages gets no feed and no feed link")
	}
	if len(site.Diags) != 0 {
		t.Errorf("diagnostics = %v", site.Diags)
	}
}
