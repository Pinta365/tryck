---
title: Config file
description: Every section and key of tryck.config.yaml.
---

tryck reads `tryck.config.yaml` from the current directory unless `--config` names another file. The directory that holds the config file is the project root: relative directories such as `contentDir` resolve against it.

Every key has a default, so a site only needs to set what it changes. This is the complete default configuration:

```yaml
site:
  title: ""
  description: ""
  baseUrl: "/"
  language: "en"

build:
  contentDir: "./content"
  outputDir: "./dist"
  staticDir: "./static"
  layoutsDir: "./layouts"
  pluginsDir: "./plugins"
  cleanOutput: true
  drafts: false
  unsafeHTML: false
  lastUpdated: false
  workers: 0

defaults:
  layout: "doc"
  toc: true
  tocDepth: [2, 3]
  dateFormat: "2006-01-02"
  headingAnchors: true
  imageAttributes: true

images:
  resize: false
  widths: [480, 800, 1600]
  quality: 85
  sizes: "(min-width: 48rem) 46rem, calc(100vw - 2rem)"

versions:
  current: ""
  older: []

theme:
  path: ""
  accent: ""
  accentDark: ""
  logo: ""
  colorScheme: "auto"
  nav: []
  footer: ""
  editUrl: ""
  css: []
  params: {}

plugins:
  readingTime:
    enabled: true
    wordsPerMinute: 200
  syntaxHighlighting:
    enabled: true
    theme: "github"
    darkTheme: "github-dark"
    lineNumbers: true
  linkCheck:
    enabled: true
    failOnBroken: true
  sitemap:
    enabled: true
  search:
    enabled: true
  mermaid:
    enabled: true
  feed:
    enabled: true
```

The `toc` and `redirects` plugins are enabled as well; they need no section. The [plugin reference](../05-extending/01-built-in-plugins.md) lists every plugin, its options and defaults.

## Precedence

Values are applied in this order, each layer winning over the one below:

1. a command-line flag,
2. the config file,
3. the built-in default.

Setting one option of a plugin does not drop its other defaults; plugin sections are merged key by key.

## site

| Key | Type | Default | Meaning |
| :- | :- | :- | :- |
| `title` | string | `""` | Site name, used in the header, `<title>` and as a title fallback. |
| `description` | string | `""` | Default `<meta name="description">` for pages without one. |
| `baseUrl` | string | `"/"` | The site's URL or path. Always ends in `/`. Prefixes internal links and builds absolute URLs for the sitemap. |
| `language` | string | `"en"` | Value of the `<html lang>` attribute. |

`baseUrl` may be an absolute `http(s)://` URL, or a path starting with `/`. Anything else, or an `http(s)` URL without a host, is a `config.invalid` error. The sitemap needs absolute URLs, so with a path-only `baseUrl` it is skipped with a `sitemap.baseUrl` warning.

## build

| Key | Type | Default | Meaning |
| :- | :- | :- | :- |
| `contentDir` | string | `"./content"` | Directory scanned for Markdown pages and their assets. |
| `outputDir` | string | `"./dist"` | Where `tryck build` writes the site. |
| `staticDir` | string | `"./static"` | Files copied to the site root unchanged. |
| `layoutsDir` | string | `"./layouts"` | Layout overrides. |
| `pluginsDir` | string | `"./plugins"` | [Script plugins](../05-extending/03-script-plugins.md). |
| `cleanOutput` | bool | `true` | Empty the output directory before writing. |
| `drafts` | bool | `false` | Include pages with `draft: true`. |
| `unsafeHTML` | bool | `false` | Pass raw HTML in Markdown through instead of replacing it with a comment. |
| `lastUpdated` | bool | `false` | Read each page's last commit date from git: shown as "Last updated" on pages and used as the sitemap's `lastmod`. Needs the full git history, so CI must not use a shallow clone. |
| `workers` | int | `0` | Parallel workers for parsing and rendering; `0` uses one per CPU. |

The directory keys must not be empty, and `workers` must be `0` or positive.

## defaults

| Key | Type | Default | Meaning |
| :- | :- | :- | :- |
| `layout` | string | `"doc"` | Layout for pages that do not set one. |
| `toc` | bool | `true` | Show a table of contents; a page's `toc` front matter overrides it. |
| `tocDepth` | `[min, max]` | `[2, 3]` | Heading levels included in the table of contents. Each is `1`–`6` and `min ≤ max`. |
| `dateFormat` | string | `"2006-01-02"` | Go time layout used by `dateFormat` when no layout is passed. |
| `headingAnchors` | bool | `true` | Add a `#` link to every heading below the page title, to link to that section. |
| `imageAttributes` | bool | `true` | Set `width`, `height` and `loading="lazy"` on Markdown images. See [Images](../02-writing/03-links.md#images). |

## images

Resized copies of Markdown images. See [Resized copies](../02-writing/03-links.md#resized-copies).

| Key | Type | Default | Meaning |
| :- | :- | :- | :- |
| `resize` | bool | `false` | Write smaller copies of PNG and JPEG images and list them in a `srcset`. |
| `widths` | list | `[480, 800, 1600]` | Widths of the copies, in pixels. Each is `1`–`10000`; at least one is needed. |
| `quality` | int | `85` | JPEG quality of the copies, `1`–`100`. |
| `sizes` | string | `"(min-width: 48rem) 46rem, calc(100vw - 2rem)"` | The `sizes` attribute: how wide images are shown. The default matches the built-in theme. |

## versions

Makes the site [versioned](../02-writing/06-versioned-docs.md). Without it the site has one version.

| Key | Type | Default | Meaning |
| :- | :- | :- | :- |
| `current` | string | `""` | Name of the version `content/` holds, served at the root. Required when `older` is set. |
| `older` | list | `[]` | Older versions, newest first. Each has a `name` (its URL segment) and an optional `dir` (default `versions/<name>`). |

Names may contain letters, digits, `.`, `-` and `_`, and must be unique. A listed version whose directory does not exist is a `config.invalid` error.

## theme

The `theme` section configures the built-in theme and selects an optional theme folder. See [The built-in theme](../04-themes/01-built-in-theme.md) and [Theme folders](../04-themes/02-theme-folders.md).

| Key | Type | Default | Meaning |
| :- | :- | :- | :- |
| `path` | string | `""` | A theme folder with `theme.yaml`, `layouts/` and `static/`. |
| `accent` | string | `""` | CSS colour for links, active navigation and tabs. |
| `accentDark` | string | `""` | Accent in dark mode; empty derives one from `accent`. |
| `logo` | string | `""` | Site path of an image shown in the header. |
| `colorScheme` | string | `"auto"` | `auto` (follows the system, with a toggle), `light` or `dark`. |
| `nav` | list | `[]` | Header links, each with `title` and `url` (both required). |
| `footer` | string | `""` | Inline Markdown shown in the site footer. |
| `editUrl` | string | `""` | Adds an "Edit this page" link. The page's file path from the project root is appended, such as `content/guide/setup.md`. |
| `css` | list | `[]` | Extra stylesheets, linked after the theme's. With a theme folder, they are added after the folder's own `css`. |
| `params` | map | `{}` | Free-form values for custom themes, read as `.Theme.Params`. |

`accent` and `accentDark` must be CSS colours: a hex value, a colour name, or a function such as `rgb(...)`. `colorScheme` must be `auto`, `light` or `dark`.

## plugins

`plugins` maps a plugin's name to its own settings section, which each plugin reads. Every plugin also understands `enabled`, a boolean that turns the plugin off:

```yaml
plugins:
  linkCheck:
    enabled: false
  readingTime:
    wordsPerMinute: 250
```

A key a plugin does not know is a `config.unknownKey` warning, as is a plugin name that does not exist.

## Unknown keys and other diagnostics

A key that is not in a section's list above is reported as `config.unknownKey`, a warning that names the full path `section.key`. YAML that does not parse is `config.yaml`, and a value that decodes but makes no sense is `config.invalid`. See [Diagnostics](../07-diagnostics.md).
