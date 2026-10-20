---
title: Links
description: How tryck rewrites links, checks them and handles subpath deploys.
---

Link to other pages by their Markdown file. tryck rewrites the destination to the final URL at build time and reports every link that does not resolve.

## Relative links

A relative destination resolves against the directory of the file it is written in, the way an editor or GitHub shows it. From `content/guide/intro.md`:

```markdown
[Setup](../setup.md)
[Same folder](install.md#download)
[Asset](diagram.png)
[A section](advanced/)
```

tryck rewrites each one:

- a Markdown file becomes its page URL, honoring `slug`,
- a file in `content/` becomes its copied asset path,
- a directory becomes its section URL,
- a name without an extension becomes a page if `name.md` exists,
- anything else keeps the clean path it would have.

Fragments and query strings are preserved: `../setup.md#download` becomes `/setup/#download`.

## Root-absolute links

A destination that starts with `/` is a site path and is used as written, with the site's base path added (see below). The one exception is a `/name.md` path, which is looked up as a source file:

```markdown
[Setup](/guide/setup/)     site path
[Setup](/guide/setup.md)   source file, rewritten to the page URL
```

External links (`https://...`, `mailto:...`) and same-page fragments (`#section`) are left unchanged.

## Assets

Files next to your pages are copied to the output and can be linked directly. `content/guide/diagram.png` is served at `/guide/diagram.png`, so `![Diagram](diagram.png)` in `content/guide/intro.md` works. Directories with an ordering prefix are renamed: a file in `content/01-guide/` is served under `/guide/`.

## Link checking

The `linkCheck` plugin checks every internal link after all pages and generated files are known.

- A link to a page or file that does not exist is `link.broken`. It is an **error** during `tryck build` (so the build fails) and a **warning** in `tryck dev`. Set `plugins.linkCheck.failOnBroken: false` to make it a warning everywhere.
- A `#fragment` that is not a heading ID or directive ID on the target page is `link.anchor`, a warning.

When a broken link is close to an existing page, the message suggests the closest URL. See [Diagnostics](../07-diagnostics.md) for the message format.

:::tip
Run `tryck build` in CI for pull requests: with the default settings, a single broken link fails the build.
:::

## Subpath deploys

When a site is served from a subpath, such as a GitHub Pages project site at `https://user.github.io/repo/`, set `site.baseUrl` to that URL. tryck prefixes every internal link with the path part of `baseUrl`, so links stay correct in the browser. The `relURL` and `absURL` template functions do the same for links you write in a layout:

```html
<a href="{{ relURL "/guide/" }}">Guide</a>
```

`relURL` adds the base path; `absURL` adds the scheme and host as well. Absolute URLs you pass are returned unchanged. See [Layouts and templates](../04-themes/03-layouts-and-templates.md).
