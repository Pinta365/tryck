---
title: Installation
description: Download a release binary or install tryck with go install.
---

tryck is one static binary with no runtime dependencies. Every release archive contains the `tryck` executable (or `tryck.exe` on Windows).

## Download a release binary

Pick the archive for your platform, unpack it and put the binary on your `PATH`. The archives are named `tryck_<os>_<arch>` and hold no version in the file name.

:::tabs{group=os}
@tab Linux (amd64)
```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_linux_amd64.tar.gz | tar xz tryck
sudo mv tryck /usr/local/bin/
```
@tab Linux (arm64)
```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_linux_arm64.tar.gz | tar xz tryck
sudo mv tryck /usr/local/bin/
```
@tab macOS (arm64)
```sh
curl -sSL https://github.com/Pinta365/tryck/releases/latest/download/tryck_darwin_arm64.tar.gz | tar xz tryck
sudo mv tryck /usr/local/bin/
```
@tab Windows (amd64)
Download `tryck_windows_amd64.zip`, unpack it and put `tryck.exe` on your `PATH`.
:::

Releases are published for Linux amd64 and arm64, macOS arm64 and Windows amd64. For another platform, build from source.

## Install with Go

With Go 1.26 or later you can install straight from the module:

```sh
go install github.com/Pinta365/tryck/cmd/tryck@latest
```

This puts `tryck` in `$(go env GOPATH)/bin`, which is usually on your `PATH`.

## Check the version

```sh
tryck version
```

This prints `tryck <version>`, for example `tryck v0.3.2`. A custom binary built with [plugins](../05-extending/05-go-plugins/01-using-go-plugins.md) reports the tryck version it was built with.

Next: [create a site](02-quick-start.md).
