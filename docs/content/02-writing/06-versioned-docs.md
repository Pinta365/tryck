---
title: Versioned docs
description: Keep the documentation of older releases next to the current one.
---

A versioned site keeps the docs of older releases online next to the current ones. `content/` is always the current version, served at the site root. Each older version is a snapshot of the content in its own folder, served under its name:

```
content/            → /          the current version, for example v0.3
versions/v0.2/      → /v0.2/
versions/v0.1/      → /v0.1/
```

Every version is built in the same run and deployed together. Layouts, static files, the theme and plugins are shared; only the content is versioned.

## Set it up

Name the current version and list the older ones in `tryck.config.yaml`, newest first:

```yaml
versions:
  current: v0.3
  older:
    - name: v0.2
    - name: v0.1
      dir: ./archive/v0.1   # optional; the default is versions/<name>
```

A version name is used as a URL segment, so it may contain letters, digits, `.`, `-` and `_`. Without a `versions` section the site is unversioned and nothing changes.

## Release a version

When a release is done and work on the next one begins, freeze the current docs as an older version:

```sh
tryck snapshot v0.3
```

This copies `content/` to `versions/v0.3/` and prints the config to add. It does not edit `tryck.config.yaml`, so your comments and layout stay as they are. Then:

1. add `- name: v0.3` at the top of `versions.older`,
2. set `versions.current` to the version `content/` holds from now on, such as `v0.4`.

:::tip[Doing it by hand]
The command is only a convenience. A version is just a folder, so copying `content/` to `versions/v0.3/` yourself, with `cp -r`, a file manager or a script in CI, works exactly the same. You can also fix an old version later by editing its files directly.
:::

## Links

Links inside a version stay inside it:

- A relative link to a Markdown file, such as `[Setup](setup.md)`, resolves within the page's own version.
- A root-absolute link such as `/guide/` in an older version points to that version's `/v0.2/guide/` when the page exists there. Other root-absolute paths, such as `/img/logo.png` from `static/`, stay at the site root, since static files are shared.
- `aliases` in an older version's front matter are paths within that version.

Link checking covers every version. A broken link in an old version is reported with its real file, such as `versions/v0.2/guide/setup.md:12`.

## What readers see

- **The version picker** next to the search box lists every version. Each entry links to the same page in that version, matched by its file path, or to the version's home page when the page does not exist there. It works without JavaScript.
- **A banner** on pages of older versions says which version they belong to and links to the same page in the current version.
- **Search** in the site's search box covers the version the reader is in. Each version has its own index, `/v0.2/search-index.json` for an older one.
- **AI tools** get an `llms.txt` per version: `/llms.txt` for the current one and `/v0.2/llms.txt` for an older one, each with Markdown copies of its pages. See [llmsTxt](../05-extending/01-built-in-plugins.md#versioned-docs).
- **Search engines** only see the current version. Older versions are left out of `sitemap.xml`, and their pages carry `<meta name="robots" content="noindex">`, so search results lead to up-to-date docs.

## Templates and plugins

Layouts get `.Version` (the page's version: `Name`, `URL`, `Current`) and `.Versions` (every version, the current one first, each with `Name`, `URL`, `Current` and `Active`). `URL` is the page's address in that version, or the version's home page. Both are empty on an unversioned site.

The picker and the banner are the partials `versions.html` and `version-banner.html`; override them like any other partial. See [Layouts and templates](../04-themes/03-layouts-and-templates.md).

Plugins see a page's version as `Version` on `plugin.Page` and `plugin.PageInfo` (`page.version` in scripts), and the current version's name as `SiteInfo.Version` (`site.version`).

## Limits

- There is no unreleased "next" version next to the current one: `content/` is always the current version.
- All versions share the layouts and the theme. A layout change applies to old versions too.
