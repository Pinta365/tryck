---
title: Directives
description: Admonitions, collapsible details and tabs written as directive blocks.
---

Directives are block-level extras written with `:::` fences. They render as HTML without JavaScript; the tabs directive is enhanced by a small script in the built-in layout.

Each example below shows the result and the Markdown that produces it. Switching one example to **Markdown** switches them all.

## Admonitions

Five kinds are built in: `note`, `tip`, `info`, `warning` and `danger`.

:::::tabs{group=example}
@tab Result
:::note
Something worth knowing.
:::

:::tip
A shortcut or a good habit.
:::

:::info
Extra context.
:::

:::warning
Something that may cause trouble.
:::

:::danger
Something that can lose data.
:::
@tab Markdown
````markdown
:::note
Something worth knowing.
:::

:::tip
A shortcut or a good habit.
:::

:::info
Extra context.
:::

:::warning
Something that may cause trouble.
:::

:::danger
Something that can lose data.
:::
````
:::::

### Titles

A title in square brackets replaces the plain box with a labelled one. The title is plain text.

:::::tabs{group=example}
@tab Result
:::tip[Live reload]
Run `tryck dev` and edit any file in `content/`. The browser reloads on save.
:::
@tab Markdown
````markdown
:::tip[Live reload]
Run `tryck dev` and edit any file in `content/`. The browser reloads on save.
:::
````
:::::

### Any Markdown inside

The body is ordinary Markdown: lists, code blocks, tables and links all work.

:::::tabs{group=example}
@tab Result
:::warning[Before you upgrade]
1. Back up `dist/` if you deploy it by hand.
2. Run a clean build:

   ```sh
   tryck build
   ```

3. Check the output for `link.broken` errors. See [Diagnostics](../07-diagnostics.md).
:::
@tab Markdown
````markdown
:::warning[Before you upgrade]
1. Back up `dist/` if you deploy it by hand.
2. Run a clean build:

   ```sh
   tryck build
   ```

3. Check the output for `link.broken` errors. See [Diagnostics](../07-diagnostics.md).
:::
````
:::::

Each admonition renders as `<aside class="admonition admonition-<kind>">`, with a `<p class="admonition-title">` when it has a title. The colours come from the `--note`, `--tip`, `--info`, `--warning` and `--danger` CSS variables of the [built-in theme](../04-themes/01-built-in-theme.md).

## Details

`:::details[summary]` renders a native collapsible `<details>` element. Without a summary it reads "Details".

:::::tabs{group=example}
@tab Result
:::details[Show the full output]
```text
Built 24 pages, copied 1 files, 0 warnings, 0 errors in 9ms
```
:::
@tab Markdown
````markdown
:::details[Show the full output]
```text
Built 24 pages, copied 1 files, 0 warnings, 0 errors in 9ms
```
:::
````
:::::

Add `{open}` to show it expanded:

:::::tabs{group=example}
@tab Result
:::details[Expanded by default]{open}
Click the summary to fold it.
:::
@tab Markdown
````markdown
:::details[Expanded by default]{open}
Click the summary to fold it.
:::
````
:::::

## Tabs

`:::tabs` holds one or more `@tab Label` sections. A tab can contain other directives; see [Nesting](#nesting) for when a longer fence helps.

:::tabs
@tab Go
```go
x := 1
```
@tab TypeScript
```ts
const x = 1;
```
@tab Python
```python
x = 1
```
:::

The Markdown for these tabs:

````markdown
:::tabs
@tab Go
```go
x := 1
```
@tab TypeScript
```ts
const x = 1;
```
@tab Python
```python
x = 1
```
:::
````

Without JavaScript the tab list is hidden and every panel shows with its label, so the content is always readable. With JavaScript the built-in layout shows one panel at a time. A tab can hold any directive, including another set of tabs.

### Synced groups

Tab sets with the same `group` switch together: pick a tab in one and the others follow. The choice is remembered in the browser, so it holds on other pages too. Try it with these two sets:

:::tabs{group=os}
@tab Linux
```sh
tar xzf tryck_linux_amd64.tar.gz tryck
```
@tab macOS
```sh
tar xzf tryck_darwin_arm64.tar.gz tryck
```
@tab Windows
Unpack `tryck_windows_amd64.zip`.
:::

:::tabs{group=os}
@tab Linux
```sh
sudo mv tryck /usr/local/bin/
```
@tab macOS
```sh
sudo mv tryck /usr/local/bin/
```
@tab Windows
Put `tryck.exe` on your `PATH`.
:::

Both start with `::::tabs{group=os}` and use the same tab labels:

````markdown
:::tabs{group=os}
@tab Linux
```sh
tar xzf tryck_linux_amd64.tar.gz tryck
```
@tab macOS
...
:::
````

Tabs sync by label, so the labels must match exactly.

## Nesting

A closing fence needs at least as many colons as its opening fence, and closes the innermost open directive. Nesting works with equal fences; a longer outer fence is easier to read.

:::::tabs{group=example}
@tab Result
::::warning[Outer]
:::note[Inner]
Closed by the three-colon fence.
:::
Closed by the four-colon fence.
::::
@tab Markdown
````markdown
::::warning[Outer]
:::note[Inner]
Closed by the three-colon fence.
:::
Closed by the four-colon fence.
::::
````
:::::

Content inside a fenced code block or raw HTML block is never treated as a directive. That is how the **Markdown** tabs on this page show directive source.

## Attributes

Any directive takes a trailing `{...}` block:

````markdown
:::note[Title]{#custom-id .extra-class}
Content.
:::
````

| Attribute | Effect |
| :- | :- |
| `.name` or `class` | Added to the directive's own classes. |
| `#name` or `id` | Sets the element ID, which links can target. Tab sets without one get `tabs-1`, `tabs-2`, and so on. |
| `group` | Becomes `data-group`; used by tabs. |
| `open` | Adds `open` to a `<details>`. A bare name is the same as `name=true`. |
| `title`, `lang`, `dir`, `data-*` | Written through unchanged. |

Values with spaces, `/` or other punctuation need double quotes: `{title="Read this"}`. Anything else, including event handlers, is dropped. An attribute block that cannot be parsed, such as one with an unclosed quote, is ignored with a `directive.attrs` warning.

## Unknown directives

A `:::` name with no renderer still renders, as `<div class="directive directive-<name>">`, and the build reports a `directive.unknown` warning. The content is never lost. To give a name its own HTML, add a template in `layouts/directives/`; see [Custom directives](../05-extending/02-custom-directives.md).
