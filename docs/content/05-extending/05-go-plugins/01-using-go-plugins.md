---
title: Using Go plugins
description: Build your own tryck binary with extra plugins and use it for a site.
---

The `tryck` binary from the releases page contains only the [built-in plugins](../01-built-in-plugins.md). Plugins are compiled in, so to use another plugin you build your own binary: a short Go program that calls `tryck.Main` with the plugins you want. It has every tryck command, and the plugins run next to the built-ins.

You need Go 1.26 or later.

## Set up the binary

Create a Go module, usually in the repository that holds the site, and add tryck:

```sh
go mod init example.com/mysite
go get github.com/Pinta365/tryck@latest
```

Add `cmd/site/main.go`. It lists the plugins to include:

```go
package main

import (
	"github.com/Pinta365/tryck/pkg/tryck"

	"example.com/hello" // a plugin package
)

func main() {
	tryck.Main(&hello.Hello{})
}
```

A plugin can come from another module, as above (`go get` it like any Go package), or be defined in the same program. [Writing plugins](02-writing-go-plugins.md) shows a complete one in a single `main.go`.

Then fetch the dependencies and build:

```sh
go mod tidy
go build -o site ./cmd/site
```

The project now looks like this:

```
mysite/
├── go.mod
├── go.sum
├── cmd/site/main.go     the custom binary
└── docs/                the tryck site
    ├── tryck.config.yaml
    └── content/
```

## Use it like tryck

The custom binary takes the same commands and flags as `tryck`:

```sh
./site init docs
cd docs
../site dev
../site build
```

Or skip the separate build step and run it with Go from the module root:

```sh
go run ./cmd/site dev --config docs/tryck.config.yaml
```

`./site version` reports the tryck version the binary was built with.

:::warning
Build the site with your binary, not the stock `tryck`. The stock binary does not know the plugin: it warns `unknown plugin "hello" in plugins` and builds the site without it.
:::

## Configure the plugin

A plugin is configured in the `plugins` section of `tryck.config.yaml`, under its name, like the built-ins:

```yaml
plugins:
  hello:
    greeting: Hej
  syntaxHighlighting:
    theme: "github"
```

It is enabled by default; `enabled: false` turns it off. If the plugin declares its config keys, a misspelled key is a `config.unknownKey` warning. The plugin's own documentation lists its options.

## Use its output

What a plugin adds depends on the plugin:

- **Page data** appears in layouts as `.Page.Data.<key>`. A plugin that sets `greeting` on every page can be shown by overriding a partial, here `layouts/partials/footer.html`:

  ```html
  <footer class="site-footer">{{ .Page.Data.greeting }} from {{ .Site.Title }}</footer>
  ```

  See [Layouts and templates](../../04-themes/03-layouts-and-templates.md) for overriding layouts.
- **Files** a plugin writes, such as a feed or an index, are written to `dist/` with the rest of the site.
- **Directives** a plugin renders are written in Markdown like the built-in ones, for example `:::youtube{id=abc}`.
- **Stylesheets** a plugin needs are linked in `<head>` by the built-in layouts.
- **Diagnostics** a plugin reports are printed like tryck's own, with the plugin's codes.

## Deploy with the custom binary

CI has to build the site with your binary too. In the GitHub Pages workflow from `tryck init`, replace the step that downloads tryck with Go, and build with `go run`:

```yaml
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - id: pages
        uses: actions/configure-pages@v6
      - name: Build
        run: go run ./cmd/site build --config docs/tryck.config.yaml --base-url "${{ steps.pages.outputs.base_url }}/"
      - uses: actions/upload-pages-artifact@v5
        with:
          path: docs/dist
```

The rest of the workflow stays the same. See [Deploying](../../06-deploying.md).

## Update tryck

The tryck version is pinned in `go.mod` like any dependency. To update it:

```sh
go get github.com/Pinta365/tryck@latest
go mod tidy
```

Then rebuild the binary.
