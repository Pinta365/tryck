package markdown

import (
	"fmt"
	"strings"
	"testing"
)

func render(t *testing.T, p *Processor, src string) string {
	t.Helper()
	doc, _ := p.Parse([]byte(src))
	out, err := p.Render(doc, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRawHTML(t *testing.T) {
	src := "<div class=\"x\">hi</div>\n\ntext <b>bold</b>\n"
	if out := render(t, New(Options{}), src); strings.Contains(out, "<div") || strings.Contains(out, "<b>") {
		t.Errorf("raw HTML not stripped by default:\n%s", out)
	}
	if out := render(t, New(Options{UnsafeHTML: true}), src); !strings.Contains(out, `<div class="x">hi</div>`) || !strings.Contains(out, "<b>bold</b>") {
		t.Errorf("raw HTML stripped with UnsafeHTML:\n%s", out)
	}
}

func TestHeadingIDs(t *testing.T) {
	out := render(t, New(Options{}), "## Install\n\n## Install\n\n## Install\n\n## Other {#custom}\n")
	for _, want := range []string{`id="install"`, `id="install-1"`, `id="install-2"`, `id="custom"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

func TestGFM(t *testing.T) {
	out := render(t, New(Options{}), "| a |\n| - |\n| 1 |\n\n~~x~~ https://example.org\n\n- [x] done\n")
	for _, want := range []string{"<table>", "<del>x</del>", `<a href="https://example.org">`, `type="checkbox"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in:\n%s", want, out)
		}
	}
}

func TestSuffixedIDs(t *testing.T) {
	src := "## A\n\n## Step 1\n\n## A\n\n## a\n"
	_, issues := New(Options{}).Parse([]byte(src))
	var got []string
	for _, is := range issues {
		got = append(got, fmt.Sprintf("%d:%s", strings.Count(src[:is.Offset], "\n")+1, is.Message))
	}
	want := []string{
		`5:heading ID "a-1" needed a suffix to be unique; add {#id} to choose one`,
		`7:heading ID "a-2" needed a suffix to be unique; add {#id} to choose one`,
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q", got)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Setup & Install":        "setup-install",
		"  Hello,   World!  ":    "hello-world",
		"Översikt 2.0":           "översikt-20",
		"snake_case--and--dash":  "snake-case-and-dash",
		"C++":                    "c",
		"!!!":                    "",
		"The `tryck` *engine*":   "the-tryck-engine",
		"-leading and trailing-": "leading-and-trailing",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderInline(t *testing.T) {
	p := New(Options{})
	for in, want := range map[string]string{
		"Some *emphasis* & `code`": "Some <em>emphasis</em> &amp; <code>code</code>",
		"":                         "",
		"para one\n\npara two":     "<p>para one</p>\n<p>para two</p>",
		"<b>raw</b>":               "<!-- raw HTML omitted -->raw<!-- raw HTML omitted -->",
	} {
		got, err := p.RenderInline(in)
		if err != nil || got != want {
			t.Errorf("RenderInline(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
