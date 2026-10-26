package build_test

import (
	"regexp"
	"strings"
	"testing"
)

// formulas returns the TeX of each rendered formula in html, in order,
// marked "display:" for displayed ones.
func formulas(html string) []string {
	var out []string
	re := regexp.MustCompile(`(?s)<math xmlns="http://www.w3.org/1998/Math/MathML"( display="block")?>.*?<annotation encoding="application/x-tex">(.*?)</annotation>`)
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		tex := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">").Replace(m[2])
		if m[1] != "" {
			tex = "display:" + tex
		}
		out = append(out, tex)
	}
	return out
}

func TestMath(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\nplugins:\n  math:\n    macros:\n      '\\RR': '\\mathbb{R}'\n",
		"content/a.md": "# A\n\n" +
			"Inline $E = mc^2$ and $x \\in \\RR$, over\nlines $a +\nb$.\n\n" +
			"Not math: $5 and $10, \\$x\\$, `$code$`, a $ alone, $ spaced $.\n\n" +
			"In a paragraph $$\\sum_i i$$ displayed.\n\n" +
			"$$\n\\int_0^1 x\\,dx\n$$\n\n" +
			"$$ a^2 + b^2 = c^2 $$\n\n" +
			"```math\n\\frac{1}{2}\n```\n",
	}))
	if site.HasErrors() || len(site.Diags) > 0 {
		t.Fatalf("diagnostics: %v", site.Diags)
	}
	html := string(site.Outputs["a/index.html"])
	want := []string{"E = mc^2", `x \in \RR`, "a +\nb", `display:\sum_i i`, `display:\int_0^1 x\,dx`, "display:a^2 + b^2 = c^2", `display:\frac{1}{2}`}
	if got := formulas(html); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("formulas:\n%q\nwant:\n%q", got, want)
	}
	for _, s := range []string{`double-struck">R</mi>`, "Not math: $5 and $10, $x$, <code>$code$</code>, a $ alone, $ spaced $.", `<div class="math"><span class="katex">`} {
		if !strings.Contains(html, s) {
			t.Errorf("missing %q in:\n%s", s, html)
		}
	}
}

func TestMathErrors(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"content/a.md": "---\ntitle: A\n---\n# A\n\nBad $\\frac{1}{$ here.\n\n```math\n\\badcommand\n```\n",
	}))
	var got []string
	for _, d := range site.Diags {
		got = append(got, d.Code+":"+strings.TrimPrefix(d.File, "content/")+":"+string(rune('0'+d.Line)))
	}
	if strings.Join(got, " ") != "math.error:a.md:6 math.error:a.md:9" {
		t.Errorf("diagnostics: %v", site.Diags)
	}
	html := string(site.Outputs["a/index.html"])
	if !strings.Contains(html, `<code class="math-error" title="`) || !strings.Contains(html, `>\frac{1}{</code>`) ||
		!strings.Contains(html, `<pre class="math-error"`) {
		t.Errorf("errors not shown as code:\n%s", html)
	}
}

func TestMathOptions(t *testing.T) {
	src := "# A\n\nCost $x$ here, $$y$$ there.\n\n```math\nz\n```\n"
	noInline := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\nplugins:\n  math:\n    inline: false\n",
		"content/a.md":      src,
	}))
	if got := formulas(string(noInline.Outputs["a/index.html"])); strings.Join(got, "|") != "display:y|display:z" {
		t.Errorf("inline: false: formulas %q", got)
	}
	off := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\nplugins:\n  math:\n    enabled: false\n",
		"content/a.md":      src,
	}))
	html := string(off.Outputs["a/index.html"])
	if strings.Contains(html, "<math") || !strings.Contains(html, "Cost $x$ here, $$y$$ there.") || !strings.Contains(html, "language-math") {
		t.Errorf("disabled math still renders:\n%s", html)
	}
}
