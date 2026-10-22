package plugin_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/pkg/plugin"
	"github.com/Pinta365/tryck/pkg/tryck"
)

// buildSite writes files into a new directory, builds it with plugins
// and returns the directory. The output is in dist/.
func buildSite(files map[string]string, plugins ...plugin.Plugin) string {
	dir, err := os.MkdirTemp("", "tryck-example")
	if err != nil {
		panic(err)
	}
	files["tryck.config.yaml"] = "site:\n  title: Example\n  baseUrl: https://example.com/\n" + files["tryck.config.yaml"]
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			panic(err)
		}
	}
	if code := tryck.Run([]string{"build", "--config", filepath.Join(dir, "tryck.config.yaml")}, io.Discard, os.Stderr, plugins...); code != 0 {
		panic(fmt.Sprintf("build failed with exit code %d", code))
	}
	return dir
}

func read(dir, path string) string {
	b, err := os.ReadFile(filepath.Join(dir, "dist", filepath.FromSlash(path)))
	if err != nil {
		panic(err)
	}
	return string(b)
}

// greeter is configured under plugins.greeter in tryck.config.yaml.
type greeter struct{ greeting string }

func (*greeter) Name() string { return "greeter" }

// ConfigKeys makes a typo in the plugin's section a warning.
func (*greeter) ConfigKeys() []string { return []string{"greeting"} }

// Init runs at the start of every build with the plugin's section.
func (g *greeter) Init(cfg map[string]any, site plugin.SiteInfo) error {
	g.greeting = "Hello"
	if v, ok := cfg["greeting"].(string); ok {
		g.greeting = v
	}
	return nil
}

func (g *greeter) OnBuildComplete(s plugin.SiteView, out plugin.Outputs) error {
	return out.Add("greeting.txt", []byte(g.greeting+" from "+s.Config().Title+"\n"))
}

// A plugin reads its own section of the config in Init.
func ExamplePlugin() {
	dir := buildSite(map[string]string{
		"tryck.config.yaml": "plugins:\n  greeter:\n    greeting: Hej\n",
		"content/index.md":  "# Home\n",
	}, &greeter{})
	defer os.RemoveAll(dir)

	fmt.Print(read(dir, "greeting.txt"))
	// Output: Hej from Example
}

// wordCount stores each page's word count for layouts.
type wordCount struct{}

func (wordCount) Name() string                               { return "wordCount" }
func (wordCount) Init(map[string]any, plugin.SiteInfo) error { return nil }

// OnPageParsed runs for every page, in parallel. It may set p.Data but
// must not change p.AST.
func (wordCount) OnPageParsed(p *plugin.Page) error {
	words := 0
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := n.(*ast.Text); ok && entering {
			words += len(strings.Fields(string(t.Segment.Value(p.Source))))
		}
		return ast.WalkContinue, nil
	})
	p.Data["words"] = words
	return nil
}

// Values a PageParser puts in p.Data reach layouts as .Page.Data.
func ExamplePageParser() {
	dir := buildSite(map[string]string{
		"layouts/doc.html": `{{ .Page.Title }}: {{ .Page.Data.words }} words`,
		"content/intro.md": "# Intro\n\nThree short words.\n",
	}, wordCount{})
	defer os.RemoveAll(dir)

	fmt.Println(read(dir, "intro/index.html"))
	// Output: Intro: 4 words
}

// pageList writes a list of every page.
type pageList struct{}

func (pageList) Name() string                               { return "pageList" }
func (pageList) Init(map[string]any, plugin.SiteInfo) error { return nil }

// OnBuildComplete runs once, after every page is rendered.
func (pageList) OnBuildComplete(s plugin.SiteView, out plugin.Outputs) error {
	var b strings.Builder
	for _, p := range s.Pages() {
		fmt.Fprintf(&b, "%s %s\n", p.URL, p.Title)
	}
	return out.Add("pages.txt", []byte(b.String()))
}

// A Finalizer adds files to the output.
func ExampleFinalizer() {
	dir := buildSite(map[string]string{
		"content/index.md":         "# Home\n",
		"content/guide/install.md": "# Install\n",
		"content/guide/_index.md":  "# Guide\n",
	}, pageList{})
	defer os.RemoveAll(dir)

	fmt.Print(read(dir, "pages.txt"))
	// Output:
	// / Home
	// /guide/ Guide
	// /guide/install/ Install
}

// badge renders :::badge[text]{color=...} blocks.
type badge struct{}

func (badge) Name() string                               { return "badge" }
func (badge) Init(map[string]any, plugin.SiteInfo) error { return nil }
func (badge) DirectiveNames() []string                   { return []string{"badge"} }

// RenderDirective writes the opening HTML when entering and the closing
// HTML when leaving; the directive's Markdown body is rendered between.
func (badge) RenderDirective(w io.Writer, d plugin.Directive, entering bool) error {
	if !entering {
		_, err := io.WriteString(w, "</span>\n")
		return err
	}
	_, err := fmt.Fprintf(w, "<span class=\"badge badge-%s\">%s", d.Attrs["color"], d.Title)
	return err
}

// A DirectiveRenderer gives new ::: names their HTML.
func ExampleDirectiveRenderer() {
	dir := buildSite(map[string]string{
		"layouts/doc.html": `{{ .Content }}`,
		"content/a.md":     ":::badge[New]{color=green}\n:::\n",
	}, badge{})
	defer os.RemoveAll(dir)

	fmt.Print(read(dir, "a/index.html"))
	// Output: <span class="badge badge-green">New</span>
}
