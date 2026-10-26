---
title: Contributing
description: Report a bug, request a feature or build tryck from source.
---

tryck is developed at [github.com/Pinta365/tryck](https://github.com/Pinta365/tryck) and released under the MIT licence. Issues and pull requests are welcome.

## Reporting a bug

Open an issue at [github.com/Pinta365/tryck/issues](https://github.com/Pinta365/tryck/issues). A report that can be reproduced quickly is far easier to fix. Include:

- the output of `tryck version`,
- your operating system and architecture,
- the exact command you ran,
- the full diagnostic output, not just the last line,
- a minimal site that reproduces the problem, if you can make one. `tryck init` in an empty directory is a good starting point; add the smallest file that shows the bug.

For a problem in the built-in theme, note the theme settings and whether you have a `layouts/` or theme folder overriding anything. See [Diagnostics](07-diagnostics.md) for what the messages mean.

## Feature requests

Open an issue that describes the problem you want to solve, not only the change you have in mind. Many additions can also be a plugin or a layout override. See [Writing plugins](05-extending/05-go-plugins/02-writing-go-plugins.md).

## Security issues

Report security problems privately with the **Report a vulnerability** button on the repository's **Security** tab. Do not open a public issue for them.

## Development

Building and testing needs Go 1.26 or later.

```sh
go test -race ./...                      # all tests
gofmt -l .                               # formatting; must print nothing
go vet ./...                             # static checks
CGO_ENABLED=0 go build ./cmd/tryck       # the binary
```

The build tests compare output against golden files under `testdata/sites/*/expected`. After an intentional change, accept the new output with:

```sh
TRYCK_UPDATE=1 go test ./internal/build
```

There is a benchmark for the build pipeline:

```sh
go test -run '^$' -bench . ./internal/build
```

These are the same checks the CI workflow runs. Run them before opening a pull request.

## Building these docs

The documentation site in `docs/` is built with tryck itself:

```sh
go run ./cmd/tryck dev --config docs/tryck.config.yaml
```

Open `http://127.0.0.1:8080/`. The build must be clean, with no warnings or errors:

```sh
go run ./cmd/tryck build --config docs/tryck.config.yaml
```

## Making a release

Releases are cut by the maintainer from `main`. Pushing a `v*` tag runs the release workflow, which tests and builds the binaries with GoReleaser and publishes them on the [releases page](https://github.com/Pinta365/tryck/releases). Before tagging, the release notes and the version the docs and starter download are updated with a small tool:

```sh
go run ./internal/tools/release v0.9.0 "a short summary for the release index"
```

The first run creates `docs/content/09-releases/v0.9.0.md` with today's date. Write the notes and the `description` in it, then run the same command again. It checks that the notes are written and that the version is newer than every release before it, then:

- orders the release notes by version, newest first,
- adds the release to the list in `docs/content/09-releases/_index.md` with the summary,
- points the installation and deploying pages and the starter workflow (`examples/docs-starter/.github/workflows/pages.yml`) at the new version.

It only edits files, and running it again changes nothing. Review the diff, then commit, push and tag; the tool prints the commands. The starter workflow downloads the tagged release, so push the tag right after the commit.
