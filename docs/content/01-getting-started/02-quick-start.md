---
title: Quick start
description: Create a site, run the dev server and build it.
---

## Create a site

```sh
tryck init my-docs
cd my-docs
```

`tryck init` copies a starter site into the given directory (the current directory when you omit it). It refuses to write anything if a file it would create already exists, so you never overwrite existing content by accident.

The starter contains:

| Path | What it is |
| :- | :- |
| `content/index.md` | The home page. |
| `content/01-getting-started/` | A section with `_index.md` and two pages. |
| `static/robots.txt` | A file copied to the site root. |
| `tryck.config.yaml` | The site's configuration. |
| `.github/workflows/pages.yml` | A GitHub Actions workflow that deploys to GitHub Pages. |
| `README.md`, `.gitignore` | A short readme, and an ignore file for `dist/` and the `tryck` binary. |

## Run the dev server

```sh
tryck dev
```

This builds the site into memory and serves it at `http://127.0.0.1:8080/`. It watches `content/`, `layouts/`, `static/` and the theme folder, rebuilds on every change and reloads the browser over a WebSocket. Build problems appear as an overlay instead of breaking the page.

`tryck dev` always includes draft pages and points the site at its own address, so it ignores `draft` and `baseUrl` in the config. Use `--port`, `--host` and `--open` to change where it listens. See [Command line](../03-configuration/02-command-line.md) for the flags.

## Build

```sh
tryck build
```

This writes the finished site to `dist/` (configurable with `build.outputDir` or `--output`). The last line of the output reports what happened:

```
Built 4 pages, copied 1 files, 0 warnings, 0 errors in 12ms
```

Warnings are printed and the site is still written. Errors, such as broken links, a missing layout or bad front matter, are all printed, nothing is written, and tryck exits with code 1.

Next: learn the [project layout](03-project-layout.md), or how [pages and URLs](../02-writing/01-pages-and-urls.md) work.
