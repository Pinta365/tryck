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

## Images

tryck sets three attributes on every Markdown image:

```html
<img src="/guide/diagram.png" alt="Diagram" width="1200" height="800" loading="lazy">
```

- `width` and `height` are read from the image file, so the browser reserves the image's space before it loads and the text below does not jump. This works for PNG, JPEG, GIF and WebP images in `content/` or `static/`; other images, such as SVG or external ones, get no size. The built-in theme scales images down to fit the page and keeps their aspect ratio.
- `loading="lazy"` makes the browser load images further down the page only when the reader scrolls near them.

Set `defaults.imageAttributes: false` to turn this off, for example in a theme whose stylesheet has no `img { height: auto; }`. Images written as HTML `<img>` tags are left unchanged.

### Resized copies

A large screenshot or photo is usually shown much smaller than it is. With `images.resize: true`, tryck writes smaller copies of PNG and JPEG images next to the original and lists them in a `srcset`, so each browser downloads only the size it needs:

```html
<img src="/guide/shot.png" alt="Shot" width="2400" height="1500"
  srcset="/guide/shot-480w.png 480w, /guide/shot-800w.png 800w, /guide/shot-1600w.png 1600w, /guide/shot.png 2400w"
  sizes="(min-width: 48rem) 46rem, calc(100vw - 2rem)" loading="lazy">
```

- A copy is made for each width in `images.widths` (default `[480, 800, 1600]`) that is smaller than the image. An image narrower than every width is left as it is.
- A copy that is not smaller in bytes than the original is skipped. This happens with some PNGs that use a small color palette.
- Copies keep the original's format. JPEG copies use `images.quality` (default `85`). JPEG photos taken on their side are turned the way browsers show them.
- GIF images (which may be animated), WebP images and SVG images are not resized.
- `sizes` tells the browser how wide the image is shown. The default matches the built-in theme's content column; set `images.sizes` if your theme is wider or narrower.
- A copy that would overwrite a file in `content/` or `static/` is not written, with an `image.conflict` warning.

Resizing a large image takes a moment, so copies are kept in tryck's cache directory (`tryck/images` in your user cache directory) and reused by later builds, including `tryck dev`. See [Deploying](../06-deploying.md#resized-images) to keep the cache between CI builds.

## Link checking

The `linkCheck` plugin checks every internal link after all pages and generated files are known.

- A link to a page or file that does not exist is `link.broken`. It is an **error** during `tryck build` (so the build fails) and a **warning** in `tryck dev`. Set `plugins.linkCheck.failOnBroken: false` to make it a warning everywhere.
- A `#fragment` that is not a heading ID or directive ID on the target page is `link.anchor`, a warning.

When a broken link is close to an existing page, the message suggests the closest URL. See [Diagnostics](../07-diagnostics.md) for the message format.

:::tip
Run `tryck build --strict` in CI for pull requests. A broken link fails any build with the default settings; `--strict` also fails on warnings such as a missing `#fragment`.
:::

## Subpath deploys

When a site is served from a subpath, such as a GitHub Pages project site at `https://user.github.io/repo/`, set `site.baseUrl` to that URL. tryck prefixes every internal link with the path part of `baseUrl`, so links stay correct in the browser. The `relURL` and `absURL` template functions do the same for links you write in a layout:

```html
<a href="{{ relURL "/guide/" }}">Guide</a>
```

`relURL` adds the base path; `absURL` adds the scheme and host as well. Absolute URLs you pass are returned unchanged. See [Layouts and templates](../04-themes/03-layouts-and-templates.md).
