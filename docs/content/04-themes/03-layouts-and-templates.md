---
title: Layouts and templates
description: The built-in layouts, partials, template data and functions.
---

Layouts are Go `html/template` files. A page renders through one layout, which usually fills a block in the shared HTML shell.

## The built-in layouts

| Layout | Used by |
| :- | :- |
| `baseof.html` | The HTML shell: `<head>`, header, footer and the theme script. Not selected directly. |
| `doc` | The default layout. Sidebar, article, table of contents and previous/next links. |
| `landing` | A wide page without a sidebar, for a home or landing page. |
| `post` | An article with its date and reading time. |
| `404` | The not-found page, always written to `404.html`. |

A page selects a layout with `layout:` in its front matter; otherwise `defaults.layout` applies (default `doc`). A layout that does not exist is a `layout.missing` error.

```
---
title: Home
layout: landing
---
```

`baseof.html` contains a `main` block. A layout that defines `{{ define "main" }}` fills that block and is wrapped in the shell; a layout with no `main` block is a complete page on its own. `404.html` also defines a `title` block for the browser tab.

## Partials

Shared fragments live in `partials/` and are called with the `partial` function. The built-in theme has `header`, `sidebar`, `toc`, `footer` and `search` (the search box, empty when the search plugin is off), plus two empty ones for your own additions: `head-end` and `body-end` (see [Adding scripts and analytics](#adding-scripts-and-analytics)).

```html
{{ partial "header" . }}
```

```html
{{ partial "toc" . }}
```

`partial` takes the partial's name (`.html` optional) and one optional data value. Without a value the partial receives `nil`.

## Adding scripts and analytics

Every page built with `baseof.html` includes two partials that are empty by default:

- `partials/head-end.html`, at the end of `<head>`: analytics, web fonts, meta tags, favicons,
- `partials/body-end.html`, at the end of `<body>`: scripts that should run after the page.

Create the one you need in your site's `layouts/partials/`; nothing else has to be copied. For example, an analytics script that is left out while you run `tryck dev`:

```html
{{ if not .Dev }}
<script defer src="https://analytics.example.com/script.js" data-site="docs.example.com"></script>
{{ end }}
```

The partials get the same data as the layout, so they can check `.Page`, `.Site` or `.Theme.Params`. A theme folder can ship its own versions; a site's file replaces the theme's.

## Directive templates

Templates in `directives/` render `:::` directives instead of pages: `layouts/directives/card.html` makes `:::card` available. See [Custom directives](../05-extending/02-custom-directives.md).

## Overriding a file

Add a file with the same name under your site's `layouts/` and it replaces the built-in version. You only override what you change: `layouts/partials/footer.html` changes the footer and leaves everything else alone, and `layouts/doc.html` replaces the whole default layout. A theme folder sits between the two (see [Theme folders](02-theme-folders.md)).

The simplest useful override is a copy of a built-in file. Start from the file in `internal/render/layouts/`, copy it under `layouts/` with the same name and edit it.

## Template data

Every layout is executed with these fields:

| Field | Type | Meaning |
| :- | :- | :- |
| `.Site` | SiteInfo | `Title`, `Description`, `BaseURL` (ends in `/`), `Language`, `Dev`. |
| `.Page` | PageInfo | The current page: `Title`, `NavTitle`, `Description`, `URL`, `Date`, `Draft`, `Weight`, `TitleInBody`, `Aliases`, `Params`, `Data`, `SourcePath`. |
| `.Content` | HTML | The rendered Markdown body. |
| `.Nav` | NavNode | The navigation tree, with `Active` and `InPath` set for this page. Children are in `.Children`. |
| `.TOC` | list of TOCEntry | The table of contents (`.Level`, `.ID`, `.Text`, `.Children`), or empty. |
| `.Prev`, `.Next` | PageInfo | Neighbouring pages in navigation order, or nil. |
| `.Stylesheets` | list of strings | Site paths of plugin CSS, for example `/css/chroma.css`. |
| `.SearchIndex` | string | Site path of the search index, or empty when the search plugin is off. |
| `.Theme` | ThemeData | `Accent`, `AccentDark`, `Logo`, `ColorScheme`, `Nav`, `Footer`, `CSS`, `Params`. |
| `.Dev` | bool | True while `tryck dev` is serving. |

`.Page.Data` holds values plugins added for the page, such as `readingTime` and `toc`.

## Template functions

| Function | Example | Result |
| :- | :- | :- |
| `relURL` | `{{ relURL "/guide/" }}` | `/guide/` with the base path added, e.g. `/tryck/guide/`. |
| `absURL` | `{{ absURL "/guide/" }}` | An absolute URL, e.g. `https://example.com/tryck/guide/`. |
| `dateFormat` | `{{ dateFormat "2006-01-02" .Page.Date }}` | The date in that Go layout. An empty layout uses `defaults.dateFormat`. A zero date gives `""`. |
| `markdownify` | `{{ markdownify "**bold**" }}` | `**bold**` rendered to HTML. Useful for a footer or a description. |
| `slugify` | `{{ slugify "Setup & Install" }}` | `setup-install`. |
| `default` | `{{ default "Untitled" .Page.Title }}` | The value, or the fallback when it is empty or zero. |
| `dict` | `{{ $d := dict "title" "Hi" }}` | A map from key/value pairs, to pass several values to a partial. |
| `jsonify` | `{{ jsonify .Page.Data }}` | The value as JSON, safe to embed in a page. |
| `partial` | `{{ partial "footer" . }}` | Another template, with optional data. |

All the standard `html/template` functions (`and`, `or`, `with`, `range`, `eq`, `printf`, `len`, ...) are available as well.

## A small example

A layout that wraps the content and adds a source link from theme params:

```html
{{ define "main" }}
<main class="content">
  <h1>{{ .Page.Title }}</h1>
  {{ .Content }}
  {{ with .Theme.Params.repo }}
  <p><a href="{{ . }}">Edit this page</a></p>
  {{ end }}
</main>
{{ end }}
```

Save it as `layouts/doc.html` to use it for every page that has no other layout.
