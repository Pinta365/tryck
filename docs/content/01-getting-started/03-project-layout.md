---
title: Project layout
description: The directories of a tryck site and what belongs in each.
---

A site is a project root with a config file and a few directories. The root is the directory that holds the config file; relative paths in the config resolve against it.

```
my-docs/
├── tryck.config.yaml    site settings
├── content/             Markdown pages and their assets
├── static/              files copied to the site root
├── layouts/             template overrides (optional)
├── plugins/             script plugins (optional)
├── themes/              theme folders (optional)
├── versions/            older versions of content/ (optional)
└── dist/                build output (written by tryck build)
```

## content/

Every `.md` file in `content/` becomes a page. Directories become sidebar sections. A folder name or file name with a numeric prefix like `01-` sets its order and is dropped from the URL.

```
content/
├── index.md                    → /
└── 01-guide/
    ├── _index.md               → /guide/
    ├── 01-install.md           → /guide/install/
    └── diagram.png             → /guide/diagram.png
```

Non-Markdown files are copied next to the pages. See [Pages and URLs](../02-writing/01-pages-and-urls.md) for the full mapping.

## static/

Files in `static/` are copied to the site root unchanged. Use it for `robots.txt`, images, fonts, or a stylesheet you link from a layout. A file in `static/` with the same path as a built-in theme file replaces it.

## layouts/

Optional `html/template` files that override the built-in layouts file by file. A file named `layouts/doc.html` replaces the built-in `doc` layout; `layouts/partials/header.html` replaces the header partial. Files not overridden keep their built-in version. Templates in `layouts/directives/` add [custom directives](../05-extending/02-custom-directives.md). See [Layouts and templates](../04-themes/03-layouts-and-templates.md).

## plugins/

Optional JavaScript files, one plugin each, that add data to pages, change their HTML or write extra files. See [Script plugins](../05-extending/03-script-plugins.md).

## themes/

Theme folders, each holding `theme.yaml`, `layouts/` and `static/`. Point `theme.path` at one to reuse a whole look. See [Theme folders](../04-themes/02-theme-folders.md).

## versions/

Snapshots of `content/` for older releases, one folder per version, on a [versioned site](../02-writing/06-versioned-docs.md).

## tryck.config.yaml

The site's settings. Every key has a default, so the file can be very small. See the [config file reference](../03-configuration/01-config-file.md).

## dist/

The output of `tryck build`. It is cleaned before every build by default (`build.cleanOutput`), and pages are written as `<url>/index.html` so links work without a trailing-slash redirect. Add `dist/` to `.gitignore`.
