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

- [Guide](https://example.com/docs/guide.md)
- [Install](https://example.com/docs/guide/install.md): Install it.
- [Setup \[beta\]](https://example.com/docs/guide/setup.md): Set it up.

## Optional

- [Notes](https://example.com/docs/notes.md)
- [One](https://example.com/docs/notes/one.md)
`
	if llms != want {
		t.Errorf("llms.txt:\n%s\nwant:\n%s", llms, want)
	}

	setup := string(site.Outputs["guide/setup.md"])
	wantSetup := "# Setup [beta]\n\n> Set it up.\n\n" + `Read [install](https://example.com/docs/guide/install.md) first, then [the API](https://example.org/api "API docs").

![Shot](https://example.com/docs/guide/shot.png)

Jump to [usage](https://example.com/docs/guide/setup/#usage) or the [home page][home], [Guide][], [notes].

` + "```markdown\n[kept](relative.md)\n```" + `

:::card{href="https://example.com/docs/guide/install.md" title=x}
Card.
:::

## Usage

Done.

[home]: ../index.md "Home"
[guide]: ./
[notes]: ../notes/one.md
`
	wantSetup = strings.Replace(wantSetup, "[home page][home]", `[home page](https://example.com/docs/index.md "Home")`, 1)
	wantSetup = strings.Replace(wantSetup, "[Guide][]", "[Guide](https://example.com/docs/guide.md)", 1)
	wantSetup = strings.Replace(wantSetup, "[notes].", "[notes](https://example.com/docs/notes/one.md).", 1)
	if setup != wantSetup {
		t.Errorf("guide/setup.md:\n%s\nwant:\n%s", setup, wantSetup)
	}
	if got := string(site.Outputs["guide/install.md"]); got != "# Install\n\n> Install it.\n\nBody.\n" {
		t.Errorf("setext title not replaced:\n%q", got)
	}
	for _, p := range []string{"guide/draft.md", "internal/a.md", "internal.md", "llms-full.txt"} {
		if _, ok := site.Outputs[p]; ok {
			t.Errorf("%s is written", p)
		}
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
	if _, ok := site.Outputs["a.md"]; ok {
		t.Error("a.md is written without pages")
	}
	want := "# Demo\n\n---\n\nSource: https://example.com/a/\n\n# A\n\nSee [B](https://example.com/b/).\n\n---\n\nSource: https://example.com/b/\n\n# B\n\nBee.\n"
	if got := string(site.Outputs["llms-full.txt"]); got != want {
		t.Errorf("llms-full.txt:\n%q\nwant:\n%q", got, want)
	}
}
