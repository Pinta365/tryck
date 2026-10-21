package render

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// searchJS loads js/search.js into a JavaScript runtime with a sample
// index prepared as pages. Without a document, only tryckSearch runs.
func searchJS(t *testing.T) *goja.Runtime {
	t.Helper()
	src, err := fs.ReadFile(Static(), "js/search.js")
	if err != nil {
		t.Fatal(err)
	}
	rt := goja.New()
	if _, err := rt.RunScript("search.js", string(src)); err != nil {
		t.Fatal(err)
	}
	index := []map[string]any{
		{"url": "/plugins/", "title": "Script plugins", "headings": []map[string]string{{"id": "hooks", "text": "The hooks"}}, "text": "A script plugin runs in the browser engine. Plugins plugins plugins."},
		{"url": "/config/", "title": "Config file", "headings": []map[string]string{{"id": "plugins", "text": "plugins"}, {"id": "theme-plugins", "text": "Theme plugins and scripts"}}, "text": "The plugins section configures every plugin."},
		{"url": "/overview/", "title": "Översikt", "headings": []map[string]string{}, "text": "Ett <b>Café</b> med plugin-stöd."},
		{"url": "/other/", "title": "Other", "headings": []map[string]string{}, "text": "Nothing to see."},
	}
	b, _ := json.Marshal(index)
	if _, err := rt.RunString("var pages = tryckSearch.prepare(" + string(b) + ");"); err != nil {
		t.Fatal(err)
	}
	return rt
}

// hits runs a search and returns "url#heading" for each hit, best first.
func hits(t *testing.T, rt *goja.Runtime, query string) []string {
	t.Helper()
	q, _ := json.Marshal(query)
	v, err := rt.RunString(`tryckSearch.search(pages, ` + string(q) + `).map((h) => h.page.url + (h.heading ? "#" + h.heading.id : ""))`)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	if err := rt.ExportTo(v, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSearchRanking(t *testing.T) {
	rt := searchJS(t)
	for _, tt := range []struct {
		query string
		want  []string
	}{
		// A title match ranks first; the heading link needs every word.
		{"plugin", []string{"/plugins/", "/config/#plugins", "/overview/"}},
		// Every word must match somewhere.
		{"script plugin", []string{"/plugins/", "/config/#theme-plugins"}},
		{"theme plugins", []string{"/config/#theme-plugins"}},
		// Case and accents are ignored, in the query and the index.
		{"OVERSIKT", []string{"/overview/"}},
		{"café", []string{"/overview/"}},
		{"cafe", []string{"/overview/"}},
		{"nomatch", nil},
		{"   ", nil},
	} {
		got := hits(t, rt, tt.query)
		if strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("search %q = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestSearchResultHTML(t *testing.T) {
	rt := searchJS(t)
	v, err := rt.RunString(`tryckSearch.search(pages, "cafe").map(tryckSearch.resultHTML)[0]`)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	// Matches are marked in the original text; text from the index is escaped.
	for _, want := range []string{
		`<a href="/overview/">`,
		`<span class="search-snippet">Ett &lt;b&gt;<mark>Café</mark>&lt;/b&gt; med plugin-stöd.</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, "<b>") {
		t.Errorf("index text not escaped: %s", html)
	}

	v, err = rt.RunString(`tryckSearch.search(pages, "theme plugins").map(tryckSearch.resultHTML)[0]`)
	if err != nil {
		t.Fatal(err)
	}
	if want := `<a href="/config/#theme-plugins"><span class="search-title">Config file</span><span class="search-heading"><mark>Theme</mark> <mark>plugins</mark> and scripts</span>`; !strings.HasPrefix(v.String(), want) {
		t.Errorf("result = %s\nwant prefix %s", v, want)
	}
}

func TestSearchSnippetIsShortened(t *testing.T) {
	rt := searchJS(t)
	long := strings.Repeat("filler words here ", 30) + "needle " + strings.Repeat("more filler text ", 30)
	b, _ := json.Marshal([]map[string]any{{"url": "/long/", "title": "Long", "text": long}})
	v, err := rt.RunString(`tryckSearch.search(tryckSearch.prepare(` + string(b) + `), "needle").map(tryckSearch.resultHTML)[0]`)
	if err != nil {
		t.Fatal(err)
	}
	s := v.String()
	start := strings.Index(s, `<span class="search-snippet">`)
	snippet := s[start:]
	if !strings.Contains(snippet, "<mark>needle</mark>") || !strings.HasPrefix(snippet, `<span class="search-snippet">… `) || !strings.HasSuffix(snippet, " …</span></a>") || len(snippet) > 260 {
		t.Errorf("snippet should be a short excerpt around the match with ellipses: %s", snippet)
	}
}
