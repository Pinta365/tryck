---
title: Installation
description: Install tryck and build this site.
---

## Download

tryck is one static binary with no runtime dependencies. Download it for your platform from the [releases page](https://github.com/Pinta365/tryck/releases).

::::tabs{group=os}
@tab Linux
```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_linux_amd64.tar.gz | tar xz tryck
```
@tab macOS
```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_darwin_arm64.tar.gz | tar xz tryck
```
@tab Windows
Download `tryck_windows_amd64.zip` and put `tryck.exe` on your `PATH`.
::::

## Build

```sh
tryck dev     # serve with live reload on http://127.0.0.1:8080/
tryck build   # write the site to dist/
```

:::note
Draft pages (`draft: true`) show up in `tryck dev` with a banner, and are left out of `tryck build` unless you pass `--drafts`.
:::
