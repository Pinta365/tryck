---
title: Deploying
description: Publish a tryck site to GitHub Pages, Cloudflare Pages or any static host.
---

`tryck build` writes a plain directory of HTML, CSS and assets to `dist/` (or `build.outputDir`). Any static host can serve it. Set `site.baseUrl` to where the site will live so internal links and the sitemap are correct.

## GitHub Pages

`tryck init` writes `.github/workflows/pages.yml`, which builds the site with a released tryck binary and deploys it with `actions/deploy-pages`.

To use it:

1. Push the site to a GitHub repository.
2. In the repository's **Settings → Pages**, set the source to **GitHub Actions**.
3. Push to `main`.

The workflow downloads `tryck_linux_amd64.tar.gz` for a pinned `TRYCK_VERSION`, builds with the Pages base URL, and uploads `dist/`:

```yaml
env:
  TRYCK_VERSION: v0.4.0
```

```sh
./tryck build --base-url "${{ steps.pages.outputs.base_url }}/"
```

`configure-pages` provides `base_url`, so a project site at `https://user.github.io/repo/` gets the right base path without editing the config. A custom domain gets its own base URL. See [Links](02-writing/03-links.md) for how `baseUrl` affects links.

## Cloudflare Pages

Create a Pages project from the repository and use this build command, with `dist` as the output directory:

```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_linux_amd64.tar.gz | tar xz tryck && ./tryck build
```

Cloudflare serves the site at the project's `pages.dev` URL or a custom domain. Set `site.baseUrl` accordingly, or pass `--base-url` in the build command.

## Any static host

Build the site and upload `dist/`:

```sh
tryck build
```

A host that serves from the site root needs only `baseUrl: "/"`. A host that serves from a subpath needs the full subpath, for example `baseUrl: "/docs/"` or `https://example.com/docs/`.

## Subpath sites

When the base URL has a path, tryck prefixes every internal link and the sitemap with it. Everything else stays the same: content, layouts and directives do not need to know where the site is served. The `relURL` and `absURL` template functions apply the same prefix inside layouts.

## The 404 page

tryck always writes `404.html` to the output root. Cloudflare Pages and GitHub Pages serve it for unknown paths without extra configuration. A custom `404` layout changes the page.

## Checking pull requests

Build the site in CI for every pull request, with `--strict` so warnings fail the check too:

```yaml
      - name: Check the docs
        run: ./tryck build --strict
```

Broken links, broken anchors, unknown config keys and other problems then show up in the pull request, with their file and line, before they reach the published site.

