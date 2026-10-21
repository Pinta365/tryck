---
title: Command line
description: Commands, flags and exit codes.
---

```
tryck build [flags]
tryck dev   [flags]
tryck init  [dir]
tryck snapshot <name>
tryck version
tryck help
```

Every command prints diagnostics to standard error and its result line to standard output. Run `tryck <command> -h` for the flags of one command.

## build

Builds the site and writes it to `outputDir`.

| Flag | Type | Default | Meaning |
| :- | :- | :- | :- |
| `--config` | string | `tryck.config.yaml` | Config file to read. Must exist when given. |
| `--output` | string | | Output directory; overrides `build.outputDir`. |
| `--drafts` | bool | `false` | Include draft pages; overrides `build.drafts`. |
| `--base-url` | string | | Site base URL; overrides `site.baseUrl`. |
| `--workers` | int | `0` | Parallel workers; `0` uses one per CPU. Overrides `build.workers`. |
| `--verbose` | bool | `false` | Debug logging with per-stage timings, and info diagnostics. |

The final line reports the result:

```
Built 4 pages, copied 1 files, 0 warnings, 0 errors in 12ms
```

## dev

Builds the site into memory, serves it, watches for changes and reloads the browser.

| Flag | Type | Default | Meaning |
| :- | :- | :- | :- |
| `--config` | string | `tryck.config.yaml` | Config file to read. |
| `--port` | int | `8080` | Port to listen on. |
| `--host` | string | `127.0.0.1` | Address to bind to. |
| `--open` | bool | `false` | Open the site in a browser. |
| `--verbose` | bool | `false` | Debug logging and info diagnostics. |

Two settings are overridden in dev, after the config is loaded:

- `build.drafts` is forced to `true`, so drafts are visible while you write.
- `site.baseUrl` is set to `http://<host>:<port>/`, so links point at the dev server.

The dev server watches `content/`, `layouts/`, `static/`, `plugins/`, the config file and the theme folder. A port outside `1`–`65535` is an error.

## init

```
tryck init [dir]
```

Copies the starter site into `dir` (the current directory by default), creating it if needed. It writes nothing if any file it would create already exists. `dir` must be at most one directory. See [Quick start](../01-getting-started/02-quick-start.md).

## snapshot

```
tryck snapshot [--config tryck.config.yaml] <name>
```

Copies `content/` to `versions/<name>/`, making the current docs an older version, and prints the lines to add to the config. It refuses a name that is already an older version, or a directory that already exists, and it does not edit the config file. Copying the folder by hand works the same. See [Versioned docs](../02-writing/06-versioned-docs.md).

## version

```
tryck version
```

Prints `tryck <version>`. `tryck --version` and `tryck -version` do the same.

## help

`tryck help`, `tryck -h` and `tryck --help` print the usage text and exit 0. The short forms also work as flags on a command, for example `tryck build -h`.

## Exit codes

| Code | Meaning |
| :- | :- |
| `0` | Success. The build may still have reported warnings. |
| `1` | Build errors: the build reported errors and nothing was written, or writing the output failed. `tryck init` also exits `1` when it cannot create the site. |
| `2` | Usage or config errors: an unknown command, a bad flag, an unreadable or invalid config. |

An unknown command prints `tryck: unknown command "<name>"` and the usage text, then exits `2`.
