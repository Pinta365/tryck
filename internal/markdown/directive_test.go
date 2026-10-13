package markdown

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func renderMD(t *testing.T, src string) (string, []Issue) {
	t.Helper()
	p := New(Options{})
	doc, issues := p.Parse([]byte(src))
	out, err := p.Render(doc, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return string(out), issues
}

// line converts an issue offset in src to a 1-based line.
func line(src string, offset int) int {
	return bytes.Count([]byte(src[:offset]), []byte("\n")) + 1
}

func TestDirectives(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "admonition with title",
			src:  ":::note[Read this]\nAny **Markdown**.\n:::\n",
			want: "<aside class=\"admonition admonition-note\">\n<p class=\"admonition-title\">Read this</p>\n<p>Any <strong>Markdown</strong>.</p>\n</aside>\n",
		},
		{
			name: "admonition kinds without title",
			src:  ":::tip\nt\n:::\n\n:::info\ni\n:::\n\n:::warning\nw\n:::\n\n:::danger\nd\n:::\n",
			want: "<aside class=\"admonition admonition-tip\">\n<p>t</p>\n</aside>\n" +
				"<aside class=\"admonition admonition-info\">\n<p>i</p>\n</aside>\n" +
				"<aside class=\"admonition admonition-warning\">\n<p>w</p>\n</aside>\n" +
				"<aside class=\"admonition admonition-danger\">\n<p>d</p>\n</aside>\n",
		},
		{
			name: "lists and code fences inside",
			src:  ":::warning\n- one\n- two\n\n```sh\n:::\nrm -rf\n```\n:::\nafter\n",
			want: "<aside class=\"admonition admonition-warning\">\n<ul>\n<li>one</li>\n<li>two</li>\n</ul>\n" +
				"<pre><code class=\"language-sh\">:::\nrm -rf\n</code></pre>\n</aside>\n<p>after</p>\n",
		},
		{
			name: "interrupts a paragraph and closes a paragraph",
			src:  "before\n:::note\ninside\n:::\nafter\n",
			want: "<p>before</p>\n<aside class=\"admonition admonition-note\">\n<p>inside</p>\n</aside>\n<p>after</p>\n",
		},
		{
			name: "nesting with longer outer fence",
			src:  "::::note\nouter\n:::tip\ninner\n:::\nstill outer\n::::\n",
			want: "<aside class=\"admonition admonition-note\">\n<p>outer</p>\n<aside class=\"admonition admonition-tip\">\n<p>inner</p>\n</aside>\n<p>still outer</p>\n</aside>\n",
		},
		{
			name: "same length fences close innermost first",
			src:  ":::note\nouter\n:::tip\ninner\n:::\nstill outer\n:::\n",
			want: "<aside class=\"admonition admonition-note\">\n<p>outer</p>\n<aside class=\"admonition admonition-tip\">\n<p>inner</p>\n</aside>\n<p>still outer</p>\n</aside>\n",
		},
		{
			name: "longer closing fence closes",
			src:  ":::note\nx\n:::::\n",
			want: "<aside class=\"admonition admonition-note\">\n<p>x</p>\n</aside>\n",
		},
		{
			name: "shorter fence does not close",
			src:  "::::note\nx\n:::\ny\n::::\n",
			want: "<aside class=\"admonition admonition-note\">\n<p>x\n:::\ny</p>\n</aside>\n",
		},
		{
			name: "fence with trailing text is not a close",
			src:  ":::note\nx\n::: no\n:::\n",
			want: "<aside class=\"admonition admonition-note\">\n<p>x\n::: no</p>\n</aside>\n",
		},
		{
			name: "attributes",
			src:  ":::note{#heads-up .wide data-x=1 onclick=\"alert(1)\" title=\"<b>\"}\nx\n:::\n",
			want: "<aside class=\"admonition admonition-note wide\" id=\"heads-up\" data-x=\"1\" title=\"&lt;b&gt;\">\n<p>x</p>\n</aside>\n",
		},
		{
			name: "title is escaped",
			src:  ":::note[<script>]\nx\n:::\n",
			want: "<aside class=\"admonition admonition-note\">\n<p class=\"admonition-title\">&lt;script&gt;</p>\n<p>x</p>\n</aside>\n",
		},
		{
			name: "details",
			src:  ":::details[More info]\nhidden\n:::\n\n:::details{open=true}\nshown\n:::\n",
			want: "<details class=\"details\">\n<summary>More info</summary>\n<p>hidden</p>\n</details>\n" +
				"<details class=\"details\" open>\n<summary>Details</summary>\n<p>shown</p>\n</details>\n",
		},
		{
			name: "inside a list item",
			src:  "- item\n\n  :::note\n  nested\n  :::\n- next\n",
			want: "<ul>\n<li>\n<p>item</p>\n<aside class=\"admonition admonition-note\">\n<p>nested</p>\n</aside>\n</li>\n<li>\n<p>next</p>\n</li>\n</ul>\n",
		},
		{
			name: "indented four spaces is code",
			src:  "    :::note\n    x\n",
			want: "<pre><code>:::note\nx\n</code></pre>\n",
		},
		{
			name: "two colons is text",
			src:  "::note\n",
			want: "<p>::note</p>\n",
		},
		{
			name: "unknown directive",
			src:  ":::youtube[Demo]{data-id=abc}\ncaption\n:::\n",
			want: "<div class=\"directive directive-youtube\" data-id=\"abc\">\n<p class=\"directive-title\">Demo</p>\n<p>caption</p>\n</div>\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := renderMD(t, tt.src)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestTabs(t *testing.T) {
	src := "::::tabs{group=lang}\n@tab TypeScript\n```ts\nconst x = 1;\n```\n@tab Go\n```go\n@tab not a tab\n```\nText *here*.\n::::\n"
	got, issues := renderMD(t, src)
	want := `<div id="tabs-1" class="tabs" data-group="lang">
<div class="tab-list" role="tablist" hidden>
<button type="button" role="tab" id="tabs-1-tab-0" aria-controls="tabs-1-panel-0" aria-selected="true">TypeScript</button>
<button type="button" role="tab" id="tabs-1-tab-1" aria-controls="tabs-1-panel-1" aria-selected="false" tabindex="-1">Go</button>
</div>
<div class="tab-panel" role="tabpanel" id="tabs-1-panel-0" aria-labelledby="tabs-1-tab-0">
<p class="tab-label" role="heading" aria-level="4">TypeScript</p>
<pre><code class="language-ts">const x = 1;
</code></pre>
</div>
<div class="tab-panel" role="tabpanel" id="tabs-1-panel-1" aria-labelledby="tabs-1-tab-1">
<p class="tab-label" role="heading" aria-level="4">Go</p>
<pre><code class="language-go">@tab not a tab
</code></pre>
<p>Text <em>here</em>.</p>
</div>
</div>
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if len(issues) != 0 {
		t.Errorf("issues: %v", issues)
	}
}

func TestTabsEdgeCases(t *testing.T) {
	t.Run("ids are unique per document and honor #id", func(t *testing.T) {
		got, _ := renderMD(t, ":::tabs\n@tab A\na\n:::\n\n:::tabs{#install}\n@tab B\nb\n:::\n\n:::tabs\n@tab C\nc\n:::\n")
		for _, want := range []string{`id="tabs-1"`, `id="install-tab-0"`, `id="install"`, `id="tabs-3-panel-0"`} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %s in:\n%s", want, got)
			}
		}
	})
	t.Run("@tab outside tabs is text", func(t *testing.T) {
		got, _ := renderMD(t, "@tab A\n\n:::note\n@tab B\n:::\n")
		if strings.Contains(got, "tab-panel") || !strings.Contains(got, "<p>@tab A</p>") || !strings.Contains(got, "<p>@tab B</p>") {
			t.Errorf("got:\n%s", got)
		}
	})
	t.Run("@tab needs a label", func(t *testing.T) {
		got, issues := renderMD(t, ":::tabs\n@tab\n@tabX\n@tab  Real \nx\n:::\n")
		if !strings.Contains(got, ">Real</button>") || strings.Count(got, `role="tab"`) != 1 {
			t.Errorf("got:\n%s", got)
		}
		if len(issues) != 1 || issues[0].Code != "directive.tabContent" {
			t.Errorf("issues = %v", issues)
		}
	})
	t.Run("tab paragraph interrupted by next tab", func(t *testing.T) {
		got, _ := renderMD(t, ":::tabs\n@tab A\nline a\n@tab B\nline b\n:::\n")
		if !strings.Contains(got, "<p>line a</p>\n</div>") || strings.Count(got, `role="tabpanel"`) != 2 {
			t.Errorf("got:\n%s", got)
		}
	})
	t.Run("directive nested in tab", func(t *testing.T) {
		got, issues := renderMD(t, "::::tabs\n@tab A\n:::note\nn\n:::\n@tab B\nb\n::::\n")
		if len(issues) != 0 || !strings.Contains(got, "<aside class=\"admonition admonition-note\">\n<p>n</p>\n</aside>\n</div>") {
			t.Errorf("issues %v, got:\n%s", issues, got)
		}
	})
}

func TestDirectiveIssues(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // "line:severity[code]"
	}{
		{"none", ":::note\nx\n:::\n", nil},
		{"unclosed at EOF", "# T\n\ntext\n\n:::warning\nrest of\nthe file\n", []string{"5:error[directive.unclosed]"}},
		{"unclosed inner, closed outer", "::::note\n:::tip\nx\n::::\n", []string{"2:error[directive.unclosed]"}},
		{"shorter close leaves it open", "::::note\nx\n:::\n", []string{"1:error[directive.unclosed]"}},
		{"longer inner takes its own fence", ":::note\n::::tip\nx\n::::\n:::\n", nil},
		{"space after colons is not a directive", "::: note\nx\n", nil},
		{"unknown", "\n:::youtube\nx\n:::\n", []string{"2:warning[directive.unknown]"}},
		{"content before first tab", ":::tabs\nstray\n\n@tab A\na\n:::\n", []string{"2:warning[directive.tabContent]"}},
		{"tabs without tabs", ":::tabs\n:::\n", []string{"1:warning[directive.tabContent]"}},
		{"duplicate heading", "## A\n\n## A\n", []string{"3:info[heading.duplicateId]"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, issues := renderMD(t, tt.src)
			var got []string
			for _, is := range issues {
				got = append(got, fmt.Sprintf("%d:%s[%s]", line(tt.src, is.Offset), is.Severity, is.Code))
			}
			if strings.Join(got, " ") != strings.Join(tt.want, " ") {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnclosedDoesNotHideStructure(t *testing.T) {
	got, _ := renderMD(t, ":::note\n# Heading\n\npara\n")
	if !strings.Contains(got, "<h1 id=\"heading\">Heading</h1>") {
		t.Errorf("content of an unclosed directive should still render:\n%s", got)
	}
}
