---
title: Built-in plugins
description: What each built-in plugin does and how to configure it.
---

Eight plugins ship with tryck. They are all enabled by default; a plugin with no section in the config file is still active. Configure a plugin under its name in the `plugins` section, and turn it off with `enabled: false`.

```yaml
plugins:
  mermaid:
    enabled: false
  readingTime:
    wordsPerMinute: 250
```

An unknown option in a plugin section is a `config.unknownKey` warning, and an unknown plugin name is a `config.unknownKey` warning as well.

| Plugin | Options | Default | Does |
| :- | :- | :- | :- |
| `toc` | — | on | Adds a table of contents. |
| `readingTime` | `wordsPerMinute` | `200` | Counts words and estimates reading time. |
| `syntaxHighlighting` | `theme`, `darkTheme`, `lineNumbers` | `github`, `github-dark`, `true` | Highlights code and writes `/css/chroma.css`. |
| `linkCheck` | `failOnBroken` | `true` | Checks internal links and anchors. |
| `sitemap` | — | on | Writes `/sitemap.xml`. |
| `search` | `maxTextLength` | `5000` | Adds the search box and writes `/search-index.json`. |
| `redirects` | — | on | Writes a redirect page per front matter alias. |
| `mermaid` | `url` | jsDelivr mermaid 11 | Draws ` ```mermaid ` blocks. |

## toc

Builds each page's table of contents from its headings and stores it as `.TOC` in layouts and `Data["toc"]` for plugins. Configure its depth with `defaults.tocDepth` and whether it is on with `defaults.toc`; a page's `toc: true`/`false` front matter overrides the default. The plugin itself has no options, but can be disabled with `plugins.toc.enabled: false`. See [Layouts and templates](../04-themes/03-layouts-and-templates.md).

## readingTime

Counts the words in a page's body — leaving out code blocks and raw HTML — and stores `Data["wordCount"]` and `Data["readingTime"]` (whole minutes, at least 1). The `post` layout shows it. `wordsPerMinute` must be positive.

## syntaxHighlighting

Highlights fenced code blocks with chroma using CSS classes, and writes the theme's rules to `/css/chroma.css`. `theme` styles light mode, `darkTheme` dark mode (empty uses `theme` in both), and `lineNumbers` shows line numbers. See [Code and diagrams](../02-writing/05-code-and-diagrams.md).

## linkCheck

Records internal links while pages are parsed and checks them once every page, asset and generated file is known. A link to something that does not exist is `link.broken`: an error during `tryck build` when `failOnBroken` is `true` (the default), and a warning during `tryck dev`. A missing `#fragment` is `link.anchor`, a warning. It runs after the other finalizers so it can see the files they added.

## sitemap

Writes `/sitemap.xml` with an absolute URL for every page that is not a draft. On a [versioned site](../02-writing/06-versioned-docs.md) it lists only the current version. It needs a `site.baseUrl` with a host, otherwise it reports `sitemap.baseUrl` and writes nothing. A page's `date` becomes the entry's `lastmod`.

## search

Adds a search box to the header of the built-in theme, and writes the index it searches, `/search-index.json`.

The box opens with the **Search** button, `/` or Ctrl+K (⌘K on a Mac), and Esc closes it. The index is fetched the first time the box opens; after that every search runs in the browser, with no server and no external service. A page matches when every word of the query appears in its title, headings or text, ignoring case and accents. Title matches rank first, then headings, then the text, and a result links straight to a heading that contains all the words. Without JavaScript there is no search button.

The index is an array of `{url, title, headings, text}` for every page that is not a draft. `headings` lists the page's level 2 to 6 headings as `{id, text}`. Code blocks are left out of the text, which is cut to `maxTextLength` bytes (default `5000`).

On a [versioned site](../02-writing/06-versioned-docs.md) each version has its own index, and the box searches the reader's version. `enabled: false` removes both the index and the search box. A custom header shows the box with `{{ partial "search" . }}`; see [Layouts and templates](../04-themes/03-layouts-and-templates.md).

## redirects

Writes a small meta-refresh page for every `aliases` entry in a page's front matter. An alias must start with `/`; otherwise it is a `redirect.alias` error. See [Front matter](../02-writing/02-front-matter.md).

## mermaid

Renders ` ```mermaid ` code blocks as diagrams and adds a module script to pages that contain one. The `url` option is the module to load; the default is mermaid 11 from jsDelivr. Point it at a file under `static/` to work offline. See [Code and diagrams](../02-writing/05-code-and-diagrams.md).
