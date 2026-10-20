---
title: Pages and URLs
description: How content files map to clean URLs.
---

Every Markdown file under `content/` becomes a page. tryck maps the file path to a clean URL by dropping the extension, removing ordering prefixes and, for index files, the file name itself.

## File to URL

| File | URL |
| :- | :- |
| `index.md` | `/` |
| `_index.md` | `/` |
| `setup.md` | `/setup/` |
| `docs/setup.md` | `/docs/setup/` |
| `docs/_index.md` | `/docs/` |
| `01-intro.md` | `/intro/` |

A page is written to `dist/<url>/index.html`, so `/docs/setup/` becomes `dist/docs/setup/index.html`.

## Section indexes

`_index.md` and `index.md` are both section indexes: they map to their directory instead of a page of their own. Use `index.md` at the root for the home page and `_index.md` inside a section to give that section a title, weight and its own page. A section without an index file still appears in the navigation, but its entry has no page.

## Ordering prefixes

A leading number on a file or directory name sets its order and is removed from the URL: `02-setup.md` is served at `/setup/` and sorts before `10-advanced.md`.

- The prefix is the digits up to and including the first dash. It is removed only if something remains after it, so `07-` alone is left as it is.
- A segment that starts with a full `YYYY-MM-DD` date is left unchanged, so a dated post like `2026-03-15-release.md` keeps its date in the URL.

The ordering prefix applies to directory names too: `01-guide/02-setup.md` is `/guide/setup/`.

## URL slug

Set `slug` in the front matter to replace the last URL segment:

```yaml
---
title: Installing
slug: install
---
```

This page is served at `/install/` even if its file is `docs/setup.md`. The root index page ignores `slug`; it stays at `/`.

## Titles

A page's title, used in the navigation, `<title>` and headings, is taken from, in order:

1. `title` in the front matter,
2. the first level-1 heading (`# ...`) in the body,
3. the file or directory name, with the prefix removed and dashes as spaces (`02-writing-pages` becomes "Writing pages"),
4. the site title,
5. `Home`.

When the body has its own `# heading`, the built-in layouts do not add another one.

## Navigation order

Pages and sections are ordered within their parent by:

1. `weight` in the front matter, lower first; unset weights come last,
2. the numeric ordering prefix, lower first; names without one come last,
3. the title, compared case-insensitively,
4. the URL, for a stable order.

## Assets next to pages

A non-Markdown file in `content/` is copied to the output next to the pages, with ordering prefixes removed from its directories. `content/01-guide/diagram.png` is written to `dist/guide/diagram.png`, and a link to `diagram.png` from a page in that folder resolves to it. See [Links](03-links.md).

## Previous and next

The built-in `doc` layout shows previous and next links. They follow the navigation tree in depth-first order: a section's own page comes before its children, so a section index sits between the last page before the section and the first page inside it.
