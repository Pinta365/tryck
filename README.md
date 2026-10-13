# tryck

A single-binary static site generator for technical documentation, written in Go. *Tryck* is Swedish for "print".

- **Plain Markdown plus `:::` directives**: admonitions, collapsible details and tabs that work without JavaScript.
- **Clear errors**: every problem is reported as `file:line:col: severity[code]: message`, and the build reports all of them at once.
- **Correct docs output**: link and anchor checking, `.md` links rewritten to page URLs, clean URLs, sitemap, search index, redirects, and subpath deploys such as GitHub Pages project sites.
- **Fast**: parallel parsing and rendering; about 80 ms for 1,000 pages on a laptop.
- **Live reload**: `tryck dev` rebuilds in memory, reloads the browser and shows build errors as an overlay.
- **No runtime dependencies**: one static binary, no Node or Python in CI.

## Quick start

```sh
tryck init my-docs
cd my-docs
tryck dev            # http://127.0.0.1:8080/
tryck build          # writes dist/
```

Download a binary for Linux, macOS or Windows from the [releases page](https://github.com/Pinta365/tryck/releases), or build from source with Go 1.23 or later:

```sh
go install github.com/Pinta365/tryck/cmd/tryck@latest
```

## Commands

```
tryck build [--config tryck.config.yaml] [--output ./dist] [--drafts] [--base-url URL] [--workers N] [--verbose]
tryck dev   [--config tryck.config.yaml] [--port 8080] [--host 127.0.0.1] [--open] [--verbose]
tryck init  [dir]
tryck version
```

Exit codes: `0` success, `1` build errors, `2` usage or config errors. CLI flags override the config file, which overrides built-in defaults.

## Content

```
content/
├── index.md                    → /
└── 01-guide/
    ├── _index.md               → /guide/          (section title and weight)
    ├── 01-install.md           → /guide/install/
    └── diagram.png             → /guide/diagram.png
```

- Numeric prefixes order pages and are dropped from URLs. `weight` in front matter wins over them.
- Link to other pages by their Markdown file, relative to the current file: `[Install](01-install.md#download)`.
- Pages with `draft: true` appear in `tryck dev` with a banner and are left out of `tryck build` unless you pass `--drafts`.

Directives:

````markdown
:::note[Optional title]
Any **Markdown**.
:::

::::tabs{group=lang}
@tab Go
```go
x := 1
```
@tab TypeScript
```ts
const x = 1;
```
::::
````

## Layouts

Default layouts are built in. Put a file with the same name in `layouts/` to override one, for example `layouts/partials/header.html`; see [internal/render/layouts](internal/render/layouts). Layouts are Go `html/template` files with the functions `relURL`, `absURL`, `dateFormat`, `markdownify`, `slugify`, `partial`, `default`, `dict` and `jsonify`.

## Plugins

Plugins are compiled in and hook into the build phases through the public API in [`pkg/plugin`](pkg/plugin). The built-ins use the same API:

| Plugin | Config key | Output |
| :- | :- | :- |
| toc | `defaults.toc`, `defaults.tocDepth` | Table of contents per page |
| readingTime | `plugins.readingTime` | `.Page.Data.readingTime` in minutes |
| syntaxHighlighting | `plugins.syntaxHighlighting` | chroma highlighting with CSS classes, `/css/chroma.css` |
| linkCheck | `plugins.linkCheck` | Broken link and anchor diagnostics |
| sitemap | `plugins.sitemap` | `/sitemap.xml` |
| search | `plugins.search` | `/search-index.json` |
| redirects | | A redirect page for each `aliases` entry |

## Deploying

`tryck init` includes `.github/workflows/pages.yml`, which downloads a release binary and deploys to GitHub Pages with the right `--base-url`. For Cloudflare Pages, use this build command with the output directory `dist`:

```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_linux_amd64.tar.gz | tar xz tryck && ./tryck build
```

## Development

```sh
go test -race ./...                      # all tests
TRYCK_UPDATE=1 go test ./internal/build  # accept new golden output in testdata/sites/*/expected
go test -run '^$' -bench . ./internal/build
```

The design is specified in [docs/SPEC.md](docs/SPEC.md); decisions made while implementing it are logged at the end of that file.
