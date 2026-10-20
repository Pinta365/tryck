---
title: tryck
description: A single-binary static site generator for technical documentation.
layout: landing
---

# tryck

tryck builds static documentation sites from Markdown. It is one Go binary with no runtime dependencies: no Node, no Python, nothing to install in CI. *Tryck* is Swedish for "print".

- **Markdown plus directives**: admonitions, collapsible details and tabs that work without JavaScript.
- **Clear errors**: every problem is printed as `file:line:col: severity[code]: message`, and a build reports all of them at once instead of stopping at the first.
- **Correct output**: link and anchor checking, `.md` links rewritten to page URLs, clean URLs, sitemap, a search index, redirects and subpath deploys.
- **Fast**: parallel parsing and rendering; about 80 ms for 1,000 pages on a laptop.
- **Themes**: configure the built-in theme or ship your own layouts, static files and defaults in a theme folder.
- **Extensible without a build step**: add your own `:::` directives with a template, or plugins in JavaScript. Go plugins for everything else.
- **Live reload**: `tryck dev` rebuilds in memory, reloads the browser and shows build errors as an overlay.

## Quick start

```sh
tryck init my-docs
cd my-docs
tryck dev
tryck build
```

## Explore the guide

::::cards
:::card[Installation]{href="/getting-started/installation/"}
Download a binary or install with `go install`.
:::

:::card[Quick start]{href="/getting-started/quick-start/"}
Create a site, run the dev server and build it.
:::

:::card[Writing pages]{href="/writing/pages-and-urls/"}
How files become URLs, front matter, links and directives.
:::

:::card[Configuration]{href="/configuration/config-file/"}
Every config key and every command-line flag.
:::

:::card[Themes]{href="/themes/built-in-theme/"}
Restyle the built-in theme or package your own.
:::

:::card[Extending tryck]{href="/extending/"}
Custom directives, JavaScript plugins and Go plugins.
:::
::::
