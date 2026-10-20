# tryck

A single-binary static site generator for technical documentation, written in Go. *Tryck* is Swedish for "print".

Documentation: <https://pinta365.github.io/tryck/>.

- **Plain Markdown plus `:::` directives**: admonitions, collapsible details and tabs that work without JavaScript.
- **Clear errors**: every problem is reported as `file:line:col: severity[code]: message`, and the build reports all of them at once.
- **Correct docs output**: link and anchor checking, `.md` links rewritten to page URLs, clean URLs, sitemap, search index, redirects, and subpath deploys such as GitHub Pages project sites.
- **Fast**: parallel parsing and rendering; about 80 ms for 1,000 pages on a laptop.
- **Themes**: configure the built-in theme (accent, logo, light/dark, header links, footer, extra CSS) or package your own in a theme folder.
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

:::tabs{group=lang}
@tab Go
```go
x := 1
```
@tab TypeScript
```ts
const x = 1;
```
:::
````

## Themes

The built-in theme is configured in `tryck.config.yaml`; most sites need nothing else:

```yaml
theme:
  accent: "#0b7a5a"          # links, active nav, tabs
  logo: /img/logo.svg        # from static/
  colorScheme: auto          # auto (follows the system, with a toggle), light or dark
  nav:                       # header links
    - title: GitHub
      url: https://github.com/you/project
  footer: "© 2026 You. Built with [tryck](https://github.com/Pinta365/tryck)."
  css: [/css/custom.css]     # extra stylesheets, loaded last
```

Its styles live in `/css/tryck.css` and use CSS variables (`--fg`, `--bg`, `--font`, `--accent`, ...), so a stylesheet in `theme.css` can restyle the site without touching templates.

**Theme folders** package layouts, static files and defaults for reuse. Point `theme.path` at one:

```
themes/acme/
├── theme.yaml        # defaults for the theme section (accent, logo, footer, params, ...)
├── layouts/          # replaces built-in layouts with the same name
└── static/           # copied to the output root
```

Files are looked up in the site first, then the theme, then the built-in theme: `layouts/partials/footer.html` in your site beats the theme's, which beats tryck's. Settings in `tryck.config.yaml` win over `theme.yaml` key by key. Custom values for a theme's templates go in `theme.params` and are available as `.Theme.Params`.

## Layouts

Layouts are Go `html/template` files; see the built-in set in [internal/render/layouts](internal/render/layouts). Templates get `.Site`, `.Page`, `.Content`, `.Nav`, `.TOC`, `.Prev`, `.Next` and `.Theme`, and the functions `relURL`, `absURL`, `dateFormat`, `markdownify`, `slugify`, `partial`, `default`, `dict` and `jsonify`.

## Custom directives

A template in `layouts/directives/` adds a directive: `layouts/directives/card.html` renders `:::card[Title]{href="/guide/"}` blocks, with the title, attributes and rendered body as template data. No code or build step. See [Custom directives](https://pinta365.github.io/tryck/extending/custom-directives/).

## Plugins

Plugins are compiled in and hook into the build phases through the public API in [`pkg/plugin`](pkg/plugin). The built-ins use the same API:

| Plugin | Config key | Output |
| :- | :- | :- |
| toc | `defaults.toc`, `defaults.tocDepth` | Table of contents per page |
| readingTime | `plugins.readingTime` | `.Page.Data.readingTime` in minutes |
| syntaxHighlighting | `plugins.syntaxHighlighting` | chroma highlighting with CSS classes; `theme` for light and `darkTheme` for dark mode, in `/css/chroma.css` |
| linkCheck | `plugins.linkCheck` | Broken link and anchor diagnostics |
| sitemap | `plugins.sitemap` | `/sitemap.xml` |
| search | `plugins.search` | `/search-index.json` |
| redirects | | A redirect page for each `aliases` entry |
| mermaid | `plugins.mermaid` | ` ```mermaid ` blocks drawn as diagrams; the script loads from `url` (jsDelivr by default) only on pages that use it |

### Writing a plugin

A plugin is a Go type implementing `plugin.Plugin` plus any of the phase interfaces in [`pkg/plugin`](pkg/plugin). Custom binaries compile plugins in and hand them to `tryck.Main`:

```go
package main

import (
	"github.com/Pinta365/tryck/pkg/plugin"
	"github.com/Pinta365/tryck/pkg/tryck"
)

type Hello struct{ greeting string }

func (*Hello) Name() string { return "hello" }

// Declare the keys of the "plugins.hello" section, so a typo warns.
func (*Hello) ConfigKeys() []string { return []string{"greeting"} }

func (h *Hello) Init(cfg map[string]any, _ plugin.SiteInfo) error {
	h.greeting = "Hello"
	if v, ok := cfg["greeting"].(string); ok {
		h.greeting = v
	}
	return nil
}

func (h *Hello) OnPageParsed(p *plugin.Page) error {
	p.Data["greeting"] = h.greeting
	return nil
}

func main() { tryck.Main(&Hello{}) }
```

Build it with `go build`, for example `go build ./examples/plugin-hello`. See [`examples/plugin-hello`](examples/plugin-hello) for a complete, tested plugin, and [Using plugins](https://pinta365.github.io/tryck/extending/go-plugins/using-go-plugins/) for setting up a site with a custom binary. `ConfigKeys` is optional; declaring it turns typos in the plugin's section into `config.unknownKey` warnings.

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

## License

MIT. See [LICENSE](LICENSE).
