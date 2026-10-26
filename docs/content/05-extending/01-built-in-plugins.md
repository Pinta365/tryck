---
title: Built-in plugins
description: What each built-in plugin does and how to configure it.
---

Ten plugins ship with tryck. They are all enabled by default; a plugin with no section in the config file is still active. Configure a plugin under its name in the `plugins` section, and turn it off with `enabled: false`.

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
| `feed` | `sections`, `limit`, `format`, `author` | `[/]`, `20`, `atom`, site title | Writes Atom or RSS feeds of dated pages. |
| `llmsTxt` | `pages`, `full`, `details`, `optional`, `exclude` | `true`, `false`, `""`, `[]`, `[]` | Writes `/llms.txt` and Markdown copies of pages for AI tools. |

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

## feed

Writes a feed of the pages that have a `date` in their front matter, newest first, for blog posts, release notes or a changelog. Drafts and [older versions](../02-writing/06-versioned-docs.md) are left out, and a site without dated pages gets no feed.

| Option | Default | Meaning |
| :- | :- | :- |
| `sections` | `[/]` | Site paths that get a feed, each at `<section>feed.xml`. A page belongs to a section when its URL starts with it. The default is one feed for the whole site, `/feed.xml`. |
| `limit` | `20` | The most entries per feed. |
| `format` | `atom` | `atom` or `rss` (RSS 2.0). Feed readers handle both. |
| `author` | the site title | The feed's author, for Atom. |

```yaml
plugins:
  feed:
    sections: [/blog/, /changelog/]
```

Each entry has the page's title, its absolute URL, its date and a summary: the front matter `description`, or else the page's first paragraph. Pages link the feeds of their sections in `<head>`, so browsers and feed readers find them. Feeds need a `site.baseUrl` with a host, like the sitemap; otherwise the build reports `feed.baseUrl` and writes none. A listed section without dated pages is a `feed.section` warning.

## llmsTxt

Writes [`/llms.txt`](https://llmstxt.org), a Markdown index of the site for language models and AI coding agents, and a Markdown copy of every page. An agent that reads your documentation, for example to get an API call right, gets the content without the navigation, scripts and markup around it, so more of the docs fit in the model's context. tryck follows version 2 of the llms.txt proposal.

The plugin writes these files:

| File | Default | Holds |
| :- | :- | :- |
| `/llms.txt` | on | The site's title and description, then a link to every page with its description, grouped by section. |
| `<page>/index.md` | on (`pages`) | Each page as Markdown, at the page's own URL: `/guide/setup/` gets `/guide/setup/index.md`, the home page `/index.md`. |
| `/llms-full.txt` | off (`full`) | Every page in one file, so a tool can load all the docs at once. It can be large, so it is off by default. |

Each page also links its Markdown copy and `llms.txt` in `<head>`, so an agent that lands on a page finds both:

```html
<link rel="alternate" type="text/markdown" href="/guide/setup/index.md">
<link rel="describedby" href="/llms.txt">
```

| Option | Default | Meaning |
| :- | :- | :- |
| `pages` | `true` | Write a Markdown copy of each page and link it from the page. `llms.txt` and links between pages then point at the copies; with `false` they point at the HTML pages, and pages link only `llms.txt`. |
| `full` | `false` | Write `/llms-full.txt`. |
| `details` | `""` | Markdown placed after the description in `llms.txt`: what a reader should know first, such as which version the docs describe. |
| `optional` | `[]` | Site paths of sections listed under `## Optional` instead of their own section, for secondary pages such as release notes. A page belongs to one when its URL starts with it. |
| `exclude` | `[]` | Site paths of sections left out of every file. Their pages get no `<head>` links. |

```yaml
plugins:
  llmsTxt:
    full: true
    details: "These docs describe tryck v0.9. Run `tryck help` for the commands."
    optional: [/releases/]
    exclude: [/internal/]
```

### llms.txt

The file follows the [llms.txt format](https://llmstxt.org): the site title as `#`, `site.description` as a quote, `details`, and then one `##` section per top-level folder of the navigation, in navigation order. The home page and pages outside any folder come first, in a section called `Pages`. Each entry links the page's Markdown copy and adds its front matter `description`:

```markdown
# tryck

> A single-binary static site generator for technical documentation.

## Pages

- [tryck](https://tryck.pinta.land/index.md): A single-binary static site generator for technical documentation.
- [Deploying](https://tryck.pinta.land/deploying/index.md): Publish a tryck site to GitHub Pages, Cloudflare Pages or any static host.

## Getting started

- [Getting started](https://tryck.pinta.land/getting-started/index.md)
- [Installation](https://tryck.pinta.land/getting-started/installation/index.md): Download a release binary or install tryck with go install.

## Optional

- [Release notes](https://tryck.pinta.land/releases/index.md)
```

Agents read or search `llms.txt` to decide which pages to fetch, so a page with a good `description` is easier to find. It is worth giving every page one. The `Optional` section is a convention for links an agent can do without; it is not treated differently from the others.

`llms.txt` describes the pages under its own path. A site deployed at a subpath, such as `https://example.com/docs/`, gets `/docs/llms.txt`, which covers exactly that site.

### The Markdown copies

A copy is at the page's URL with `index.md` added, the form llms.txt v2 gives for URLs that end in `/`, so an agent can work out where it is from the page URL alone. Each copy is the page's source as you wrote it, changed only where it would not work on its own:

- It starts with the page title as `#` and the `description` as a quote. Front matter is removed, and so is the page's own `#` heading if the body starts with one, so the title is not repeated.
- Every link and image becomes an absolute URL, since the copy is not where the source file was. A link to another page points at that page's copy (or at the page, with `pages: false`), and a link within the page points at the HTML page with its `#fragment`. External links are left as they are.
- Reference links such as `[text][ref]` become inline links. Their definitions stay at the bottom unchanged.
- `href` and `src` attributes of [directives](../02-writing/04-directives.md) are rewritten like links. The directives themselves are kept as written; the `:::note` syntax reads well as plain text.
- Raw HTML, including HTML comments, is removed, as it is from the HTML pages. With `build.unsafeHTML: true` it is kept.
- Code blocks and everything else are unchanged.

### llms-full.txt

The site title and description, then every page from `llms.txt` in the same order. Each page is the same Markdown as its copy, after a `---` line and a `Source:` line with the page's URL:

```markdown
---

Source: https://tryck.pinta.land/deploying/

# Deploying

> Publish a tryck site to GitHub Pages, Cloudflare Pages or any static host.
...
```

`llms-full.txt` is not part of the llms.txt proposal, but many sites publish one and some tools look for it.

### What is included

Drafts, pages of [older versions](../02-writing/06-versioned-docs.md) and pages under `exclude` are left out of every file. Everything else that is in the navigation is listed. Links in `llms.txt` and the copies are absolute when `site.baseUrl` has a host, such as `https://example.com/docs/`; with a path-only `baseUrl` they start with the base path, which works when a tool fetches them from the same site.

The `<head>` links are added to pages whose layout has a `<head>`, as the built-in layouts do. Static hosts usually serve `.md` files as plain text or Markdown, so tools can read the copies directly. To let people find `llms.txt` too, link it from your site, for example in `theme.nav` or `theme.footer`.
