package markdown

import (
	"strings"
	"testing"
)

func render(t *testing.T, p *Processor, src string) string {
	t.Helper()
	out, err := p.Render(p.Parse([]byte(src)), []byte(src))
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
