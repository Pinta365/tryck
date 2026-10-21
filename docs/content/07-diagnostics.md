---
title: Diagnostics
description: The message format and every diagnostic code.
---

Every problem tryck finds while loading or building a site is a diagnostic. A build reports all of them at once instead of stopping at the first.

## Format

```
file:line:col: severity[code]: message
```

For example:

```
content/guide/intro.md:12: error[link.broken]: /guide/setup/ does not exist (did you mean /guide/setup-guide/?)
```

- `file` is relative to the project root, with forward slashes. It is omitted when the problem is not tied to one file.
- `line` and `col` are 1-based and omitted when unknown. Not every diagnostic has a position.
- `severity` is `info`, `warning` or `error`.
- `code` is a stable identifier you can search for and depend on.

## Severities

| Severity | Meaning |
| :- | :- |
| `info` | A note, such as a duplicated heading ID that was made unique. Hidden unless you pass `--verbose`. |
| `warning` | Something to look at. Warnings do not fail a build. |
| `error` | A build error. `tryck build` reports every error, writes nothing and exits with code 1. |

Diagnostics go to standard error, sorted by file, line and column.

## Codes

| Code | Severity | Meaning | Typical fix |
| :- | :- | :- | :- |
| `config.read` | error | A config or theme file could not be read. | Check the path and permissions. A `--config` file must exist. |
| `config.yaml` | error | The config file is not valid YAML. | Fix the reported line; YAML is indentation-sensitive. |
| `config.invalid` | error | A value decoded but makes no sense, such as an empty directory, a bad `baseUrl`, a negative `workers`, a bad `tocDepth` or an invalid colour. | Use a value the message names. |
| `config.unknownKey` | warning | An unknown config key, or an unknown plugin name. | Remove or rename the key; check the spelling against the [config reference](03-configuration/01-config-file.md). |
| `content.read` | error | A content or static directory could not be read. | Check the directory exists and is readable. |
| `frontmatter.yaml` | error | A page's front matter is not valid YAML, or an opening `---` has no closing one. | Fix the front matter; line numbers count from the opening `---`. |
| `url.conflict` | error | Two pages map to the same URL, or an asset would overwrite a generated file. | Give one page a different `slug` or path. |
| `layout.missing` | error | A page names a layout that does not exist. | Add the layout file, or fix the `layout` front matter. |
| `template.parse` | error | A layout file could not be parsed, its directory could not be read, or a directive template has an invalid or built-in name. | Fix the template syntax at the reported line. |
| `template.exec` | error | A layout failed while rendering a page, for example a missing field or a failing function. | Fix the template. The message names the template error. |
| `markdown.render` | error | The Markdown body could not be rendered to HTML, for example because a directive template failed. | Usually a plugin renderer; check the message. |
| `plugin.config` | error | A plugin could not be loaded, configured or initialized: a script with a syntax error or an invalid name, an `init` that threw, an unknown highlight theme, an empty mermaid URL, a duplicate plugin name, or a directive name claimed twice (by plugins or by a plugin and a template) or built in. | Fix the plugin section named in the message. |
| `plugin.error` | error | A plugin returned an error from a phase or a script threw (the message gives the position in the script), or a finalizer tried to add an invalid or already-produced file. | Fix the plugin's configuration or input. |
| `link.broken` | error | An internal link points at a page, asset or file that does not exist. It is a warning during `tryck dev`, or when `plugins.linkCheck.failOnBroken` is `false`. | Fix the link; the message suggests the closest URL when one is near. |
| `link.anchor` | warning | A `#fragment` in a link is not an ID on the target page. | Fix the fragment or add a heading with that ID. |
| `redirect.alias` | error | An alias does not start with `/`, or its redirect file could not be written. | Make every alias a root-absolute path. |
| `feed.baseUrl` | warning | Feeds need absolute URLs, but `site.baseUrl` has no host. | Set an `http(s)` `baseUrl`, or disable `plugins.feed`. |
| `feed.section` | warning | A section in `plugins.feed.sections` has no pages with a date. | Fix the section path, or add a `date` to its pages. |
| `sitemap.baseUrl` | warning | The sitemap needs absolute URLs, but `site.baseUrl` has no host. | Set an `http(s)` `baseUrl`, or disable `plugins.sitemap`. |
| `directive.unclosed` | error | A `:::` block is never closed. | Close it with a line of at least as many colons as the opening fence. |
| `directive.unknown` | warning | A `:::` name has no built-in or plugin renderer; it is rendered as a plain `<div>`. | Register a `DirectiveRenderer`, or fix the name. |
| `directive.attrs` | warning | A directive's `{...}` attribute block could not be parsed, for example an unclosed quote. The attributes are ignored. | Fix the attribute syntax. |
| `directive.tabContent` | warning | A `tabs` directive has content before the first `@tab` (shown above the tabs), or no `@tab` at all. | Put all content after an `@tab` line. |
| `heading.duplicateId` | info | Two headings wanted the same ID; the later one got a numeric suffix. | Add `{#custom-id}` to choose an ID. |

## Suppressing a diagnostic

There is no per-diagnostic suppression. Fix the cause, or disable the plugin that reports it under `plugins`:

```yaml
plugins:
  linkCheck:
    enabled: false
```

Disabling `linkCheck` also turns off anchor checking. Prefer fixing the links.
