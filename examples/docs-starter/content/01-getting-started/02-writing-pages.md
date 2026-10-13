---
title: Writing pages
description: Front matter, links, directives and code.
---

## Front matter

Pages start with optional YAML front matter:

```yaml
---
title: Writing pages     # defaults to the first # heading, then the file name
weight: 2                # sidebar order; numeric file prefixes like 02- work too
navTitle: Writing        # shorter sidebar label
slug: writing            # replaces the last URL segment
aliases: [/old-url/]     # old URLs that redirect here
draft: true
---
```

## Links

Link to other pages by their Markdown file, relative to the current file: [Installation](01-installation.md), or to a heading: [Build](01-installation.md#build). tryck turns these into the right URLs, including on subpath deploys, and `tryck build` fails on broken links.

## Directives

:::warning[Be careful]
Admonitions come in `note`, `tip`, `info`, `warning` and `danger`.
:::

:::details[Click to expand]
Collapsible content, with **Markdown** inside.
:::

## Code

```go
package main

import "fmt"

func main() {
	fmt.Println("Hello from tryck")
}
```
