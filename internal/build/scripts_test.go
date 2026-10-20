package build_test

import (
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/diag"
)

func TestScriptPlugin(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  title: Docs\n  baseUrl: https://example.com/\nplugins:\n  hello:\n    greeting: Hej\n",
		"plugins/hello.js": `var configKeys = ["greeting"];
var stylesheets = ["/css/hello.css"];

function init(config, site) {
  return { greeting: config.greeting ?? "Hello", site: site.title };
}

function onPageParsed(page, state) {
  page.data.greeting = ` + "`${state.greeting} from ${state.site}`" + `;
  page.data.headings = page.headings.map((h) => h.level + ":" + h.id + ":" + h.text).join(",");
  page.data.status = page.front.status ?? "none";
}

function onPageRendered(page, state, site) {
  page.html = page.html.replace("</main>", "<p>rendered " + (site.page("/b/")?.title ?? "?") + "</p></main>");
}

function onBuildComplete(site, out, state) {
  out.add("pages.txt", site.pages.map((p) => p.url + "=" + p.data.greeting).join("\n") + "\n");
}
`,
		"layouts/partials/footer.html": `<footer>{{ .Page.Data.greeting }} {{ .Page.Data.headings }} {{ .Page.Data.status }}</footer>`,
		"content/a.md":                 "---\nstatus: beta\n---\n# A\n\n## Install *now*\n",
		"content/b.md":                 "# B\n",
	}))
	for _, d := range site.Diags {
		if d.Severity > diag.Info {
			t.Errorf("unexpected diagnostic: %v", d)
		}
	}
	a := string(site.Outputs["a/index.html"])
	for _, want := range []string{
		"<footer>Hej from Docs 1:a:A,2:install-now:Install now beta</footer>",
		"<p>rendered B</p></main>",
		`<link rel="stylesheet" href="/css/hello.css">`,
	} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %q in:\n%s", want, a)
		}
	}
	if got, want := string(site.Outputs["pages.txt"]), "/a/=Hej from Docs\n/b/=Hej from Docs\n"; got != want {
		t.Errorf("pages.txt = %q, want %q", got, want)
	}
}

// Page hooks start from fresh script state, so a counter never sees
// another page and parallel builds stay deterministic.
func TestScriptStateIsPerPage(t *testing.T) {
	files := map[string]string{
		"plugins/count.js":             "let n = 0;\nfunction onPageParsed(page) { n++; page.data.n = n; }\n",
		"layouts/partials/footer.html": `<footer>n={{ .Page.Data.n }}</footer>`,
	}
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		files["content/"+name+".md"] = "# " + name + "\n"
	}
	site := mustBuild(t, writeSite(t, files))
	for path, out := range site.Outputs {
		if strings.HasSuffix(path, "index.html") && path != "index.html" && !strings.Contains(string(out), "<footer>n=1</footer>") {
			t.Errorf("%s: want n=1, got:\n%s", path, out)
		}
	}
}

func TestScriptDiagnostics(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml":  "site:\n  baseUrl: https://example.com/\nplugins:\n  keys:\n    colour: red\n  free:\n    anything: 1\n",
		"plugins/syntax.js":  "function onPageParsed(page) {\n  return page.(;\n}\n",
		"plugins/throws.js":  "function onPageParsed(page) {\n  return page.data.missing.x;\n}\n",
		"plugins/reports.js": "function onPageParsed(page) {\n  page.report(\"warning\", \"check this\", 2);\n}\n",
		"plugins/keys.js":    "var configKeys = [\"color\"];\n",
		"plugins/free.js":    "function init(config) { return config; }\n",
		"plugins/sitemap.js": "function onPageParsed(page) {}\n",
		"plugins/9bad.js":    "",
		"content/a.md":       "---\ntitle: A\n---\nline one\nline two\n",
	}))
	for _, want := range []string{
		"plugins/syntax.js:2:15: error[plugin.config]",
		"content/a.md: error[plugin.error]: plugin throws: onPageParsed: TypeError: Cannot read property 'x' of undefined",
		"plugins/throws.js:2:",
		"content/a.md:5: warning[reports]: check this",
		`warning[config.unknownKey]: unknown config key "plugins.keys.colour"`,
		"error[plugin.config]: plugin sitemap: duplicate plugin name",
		`plugins/9bad.js: error[plugin.config]: "9bad" is not a plugin name`,
	} {
		found := false
		for _, d := range site.Diags {
			found = found || strings.Contains(d.String(), want)
		}
		if !found {
			t.Errorf("missing %q in:\n%v", want, site.Diags)
		}
	}
	for _, d := range site.Diags {
		if strings.Contains(d.Message, "plugins.free") {
			t.Errorf("a script without configKeys must accept any key: %v", d)
		}
	}
}

func TestScriptThemeOverrideAndDisable(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml":            "site:\n  baseUrl: https://example.com/\ntheme:\n  path: themes/t\nplugins:\n  off:\n    enabled: false\n",
		"themes/t/plugins/mark.js":     "function onPageParsed(page) { page.data.mark = 'theme'; }\n",
		"themes/t/plugins/off.js":      "function onPageParsed(page) { page.data.off = 'ran'; }\n",
		"plugins/mark.js":              "function onPageParsed(page) { page.data.mark = 'site'; }\n",
		"layouts/partials/footer.html": `<footer>{{ .Page.Data.mark }}|{{ .Page.Data.off }}</footer>`,
		"content/a.md":                 "# A\n",
	}))
	if a := string(site.Outputs["a/index.html"]); !strings.Contains(a, "<footer>site|</footer>") {
		t.Errorf("want the site's script and the theme's disabled, got:\n%s", a)
	}
}
