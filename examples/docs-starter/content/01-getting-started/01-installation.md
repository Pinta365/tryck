---
title: Installation
description: Install tryck and build this site.
---

## Download

tryck is one static binary with no runtime dependencies. Install it with a package manager, or download it for your platform from the [releases page](https://github.com/Pinta365/tryck/releases).

:::tabs{group=os}
@tab Linux
```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_linux_amd64.tar.gz | tar xz tryck
```
@tab macOS
```sh
brew install pinta365/tap/tryck
```
@tab Windows
```sh
scoop bucket add pinta365 https://github.com/Pinta365/scoop-bucket
scoop install tryck
```
:::

## Build

```sh
tryck dev     # serve with live reload on http://127.0.0.1:8080/
tryck build   # write the site to dist/
```

:::note
Draft pages (`draft: true`) show up in `tryck dev` with a banner, and are left out of `tryck build` unless you pass `--drafts`.
:::
