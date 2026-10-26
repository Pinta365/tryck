---
title: Front matter
description: The YAML header of a page and every field it understands.
---

A page can start with a YAML front matter block delimited by `---` on its own line at the very start of the file:

```markdown
---
title: Installing
weight: 2
---

Page content starts here.
```

A leading UTF-8 byte order mark is ignored. If the file has no front matter, `---` is ordinary text. An opening `---` with no closing `---` is an error (`frontmatter.yaml`), and so is YAML that does not parse.

## Fields

| Field | Type | Meaning |
| :- | :- | :- |
| `title` | string | Page title for the navigation, `<title>` and the heading the layouts add. |
| `description` | string | Used for the `<meta name="description">` tag; falls back to `site.description`. |
| `date` | date | Publication date, shown by the `post` layout, written to the sitemap as `lastmod`, and what puts a page in the [feed](../05-extending/01-built-in-plugins.md#feed). |
| `draft` | bool | A draft is left out of a normal build and included in `tryck dev`. |
| `weight` | int | Ordering among siblings in the navigation; lower comes first. |
| `navTitle` | string | A shorter label for the sidebar and previous/next links. |
| `layout` | string | Layout name (file without `.html`); overrides `defaults.layout`. |
| `toc` | bool | Show the table of contents; when absent, `defaults.toc` applies. |
| `slug` | string | Replaces the last URL segment. |
| `aliases` | list of strings | Old URLs; each becomes a redirect page. |

A page's front matter is also available to templates through `.Page`, for example `.Page.Title`, `.Page.Date`, `.Page.Aliases` and `.Page.Params`.

## Unknown keys

Keys that are not in the table above are kept as page parameters, not rejected. This is how you attach your own data to a page:

```yaml
---
title: Widget
toc: false
status: beta
reviewers:
  - Ada
  - Grace
---
```

Use them in a custom layout as `.Page.Params.status` and `.Page.Params.reviewers`.

The [social](../05-extending/01-built-in-plugins.md#social) plugin reads one such parameter: `image`, the picture shown when the page is shared, such as `image: /img/widget-card.png`.

## Drafts

A page with `draft: true` is:

- left out of `tryck build` unless you pass `--drafts` or set `build.drafts: true`,
- included by `tryck dev`, with a banner at the top of the page,
- left out of the sitemap and the search index even when drafts are built.

## Aliases and redirects

Each entry in `aliases` becomes a small HTML page that redirects to the page. An alias must start with `/`.

```yaml
---
title: Installation
aliases:
  - /install/
  - /setup.html
---
```

`/install/` is written to `dist/install/index.html` and `/setup.html` to `dist/setup.html`. An alias that does not start with `/` is an error (`redirect.alias`).
