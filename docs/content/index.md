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
- **Live reload**: `tryck dev` rebuilds in memory, reloads the browser and shows build errors as an overlay.

## Quick start

```sh
tryck init my-docs
cd my-docs
tryck dev
tryck build
```

## Explore the guide

- [Installation](01-getting-started/01-installation.md) — download a binary or install with `go install`.
- [Quick start](01-getting-started/02-quick-start.md) — create a site and run it.
- [Command line](03-configuration/02-command-line.md) — every command and flag.
- [Writing pages](02-writing/01-pages-and-urls.md) — how files become URLs.
- [Configuration](03-configuration/01-config-file.md) — every config key.
- [Extending tryck](05-extending/02-custom-directives.md) — custom directives from templates, JavaScript plugins and Go plugins.
