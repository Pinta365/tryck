package build_test

import (
	"strings"
	"testing"
)

func TestLLMsTxt(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  title: Demo\n  description: A demo site.\n  baseUrl: https://example.com/docs/\n" +
			"plugins:\n  llmsTxt:\n    details: Read the guide first.\n    optional: [notes]\n    exclude: [/internal/]\n",
		"content/index.md":           "---\ntitle: Home\n---\nWelcome. See the [guide](guide/).\n",
		"content/guide/_index.md":    "# Guide\n\nThe guide.\n",
		"content/guide/setup.md":     setupPage,
		"content/guide/install.md":   "---\ndescription: Install it.\n---\nInstall\n=======\n\nBody.\n",
		"content/guide/shot.png":     "png",
		"content/guide/draft.md":     "---\ndraft: true\n---\n# Draft\n",
		"content/notes/_index.md":    "# Notes\n",
		"content/notes/one.md":       "# One\n",
		"content/internal/a.md":      "# Internal\n",
		"content/internal/_index.md": "# Internal\n",
	}))
	if site.HasErrors() {
		t.Fatalf("build errors: %v", site.Diags)
	}

	llms := string(site.Outputs["llms.txt"])
	want := `# Demo

> A demo site.

Read the guide first.

## Pages

- [Home](https://example.com/docs/index.md)

## Guide

- [Guide](https://example.com/docs/guide/index.md)
- [Install](https://example.com/docs/guide/install/index.md): Install it.
- [Setup \[beta\]](https://example.com/docs/guide/setup/index.md): Set it up.

## Optional

- [Notes](https://example.com/docs/notes/index.md)
- [One](https://example.com/docs/notes/one/index.md)
`
	if llms != want {
		t.Errorf("llms.txt:\n%s\nwant:\n%s", llms, want)
	}

	setup := string(site.Outputs["guide/setup/index.md"])
	wantSetup := "# Setup [beta]\n\n> Set it up.\n\n" + `Read [install](https://example.com/docs/guide/install/index.md) first, then [the API](https://example.org/api "API docs").

![Shot](https://example.com/docs/guide/shot.png)

Jump to [usage](https://example.com/docs/guide/setup/#usage) or the [home page][home], [Guide][], [notes].

` + "```markdown\n[kept](relative.md)\n```" + `

:::card{href="https://example.com/docs/guide/install/index.md" title=x}
Card.
:::

## Usage

Done.

[home]: ../index.md "Home"
[guide]: ./
[notes]: ../notes/one.md
`
	wantSetup = strings.Replace(wantSetup, "[home page][home]", `[home page](https://example.com/docs/index.md "Home")`, 1)
	wantSetup = strings.Replace(wantSetup, "[Guide][]", "[Guide](https://example.com/docs/guide/index.md)", 1)
	wantSetup = strings.Replace(wantSetup, "[notes].", "[notes](https://example.com/docs/notes/one/index.md).", 1)
	if setup != wantSetup {
		t.Errorf("guide/setup/index.md:\n%s\nwant:\n%s", setup, wantSetup)
	}
	if got := string(site.Outputs["guide/install/index.md"]); got != "# Install\n\n> Install it.\n\nBody.\n" {
		t.Errorf("setext title not replaced:\n%q", got)
	}
	for _, p := range []string{"guide/draft/index.md", "internal/a/index.md", "internal/index.md", "llms-full.txt"} {
		if _, ok := site.Outputs[p]; ok {
			t.Errorf("%s is written", p)
		}
	}

	// Pages link their copy and llms.txt in <head> (llms.txt v2).
	links := "<link rel=\"alternate\" type=\"text/markdown\" href=\"/docs/guide/setup/index.md\">\n<link rel=\"describedby\" href=\"/docs/llms.txt\">\n"
	if html := string(site.Outputs["guide/setup/index.html"]); !strings.Contains(html, links) {
		t.Errorf("guide/setup/index.html has no llms links in <head>:\n%s", html)
	}
	if html := string(site.Outputs["internal/a/index.html"]); strings.Contains(html, "describedby") {
		t.Error("an excluded page links llms.txt")
	}
}

const setupPage = `---
title: Setup [beta]
description: Set it up.
---
# Setup

Read [install](install.md) first, then [the API](https://example.org/api "API docs").

![Shot](shot.png)

Jump to [usage](#usage) or the [home page][home], [Guide][], [notes].

` + "```markdown\n[kept](relative.md)\n```" + `

:::card{href=install.md title=x}
Card.
:::

## Usage

Done.

[home]: ../index.md "Home"
[guide]: ./
[notes]: ../notes/one.md
`

func TestLLMsTxtFullWithoutPages(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  title: Demo\n  baseUrl: https://example.com/\nplugins:\n  llmsTxt:\n    pages: false\n    full: true\n",
		"content/a.md":      "# A\n\nSee [B](b.md).\n",
		"content/b.md":      "# B\n\nBee.\n",
	}))
	if got := string(site.Outputs["llms.txt"]); !strings.Contains(got, "- [A](https://example.com/a/)\n- [B](https://example.com/b/)\n") {
		t.Errorf("llms.txt without pages links the pages:\n%s", got)
	}
	if _, ok := site.Outputs["a/index.md"]; ok {
		t.Error("a/index.md is written without pages")
	}
	if html := string(site.Outputs["a/index.html"]); strings.Contains(html, "text/markdown") || !strings.Contains(html, `<link rel="describedby" href="/llms.txt">`) {
		t.Errorf("without pages, a page links only llms.txt:\n%s", html)
	}
	want := "# Demo\n\n---\n\nSource: https://example.com/a/\n\n# A\n\nSee [B](https://example.com/b/).\n\n---\n\nSource: https://example.com/b/\n\n# B\n\nBee.\n"
	if got := string(site.Outputs["llms-full.txt"]); got != want {
		t.Errorf("llms-full.txt:\n%q\nwant:\n%q", got, want)
	}
}

func TestLLMsTxtVersions(t *testing.T) {
	files := map[string]string{
		"tryck.config.yaml": "site:\n  title: Demo\n  baseUrl: https://example.com/\nversions:\n  current: v2\n  older:\n    - name: v1\n" +
			"plugins:\n  llmsTxt:\n    full: true\n    details: About v2.\n    exclude: [/internal/]\n",
		"content/setup.md":          "# Setup\n\nSee [install](install.md).\n",
		"content/install.md":        "# Install\n",
		"versions/v1/setup.md":      "---\ndescription: The old way.\n---\n# Setup\n\nSee [install](install.md).\n",
		"versions/v1/install.md":    "# Install\n",
		"versions/v1/internal/x.md": "# Internal\n",
	}
	site := mustBuild(t, writeSite(t, files))
	if site.HasErrors() {
		t.Fatalf("build errors: %v", site.Diags)
	}
	if got := string(site.Outputs["llms.txt"]); strings.Contains(got, "/v1/") || !strings.Contains(got, "About v2.") {
		t.Errorf("llms.txt is the current version's:\n%s", got)
	}
	want := `# Demo (v1)

These are the docs for version v1. The current docs are described in [llms.txt](https://example.com/llms.txt).

## Pages

- [Install](https://example.com/v1/install/index.md)
- [Setup](https://example.com/v1/setup/index.md): The old way.
`
	if got := string(site.Outputs["v1/llms.txt"]); got != want {
		t.Errorf("v1/llms.txt:\n%s\nwant:\n%s", got, want)
	}
	if got := string(site.Outputs["v1/setup/index.md"]); !strings.Contains(got, "See [install](https://example.com/v1/install/index.md).") {
		t.Errorf("v1 copies link v1 copies:\n%s", got)
	}
	if got := string(site.Outputs["v1/llms-full.txt"]); !strings.HasPrefix(got, "# Demo (v1)\n") || !strings.Contains(got, "Source: https://example.com/v1/setup/") {
		t.Errorf("v1/llms-full.txt:\n%s", got)
	}
	if _, ok := site.Outputs["v1/internal/x/index.md"]; ok {
		t.Error("exclude does not apply within v1")
	}
	if html := string(site.Outputs["v1/setup/index.html"]); !strings.Contains(html, `<link rel="describedby" href="/v1/llms.txt">`) {
		t.Errorf("v1 pages link v1/llms.txt:\n%s", html)
	}

	files["tryck.config.yaml"] += "    versions: false\n"
	off := mustBuild(t, writeSite(t, files))
	for _, p := range []string{"v1/llms.txt", "v1/setup/index.md", "v1/llms-full.txt"} {
		if _, ok := off.Outputs[p]; ok {
			t.Errorf("versions: false writes %s", p)
		}
	}
	if html := string(off.Outputs["v1/setup/index.html"]); strings.Contains(html, "describedby") {
		t.Error("versions: false still links llms.txt from v1 pages")
	}
}
