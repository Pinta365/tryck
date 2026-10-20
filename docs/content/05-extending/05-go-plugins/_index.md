---
title: Go plugins
weight: 5
---

Go plugins are compiled into your own tryck binary. They can do everything the built-in plugins do: change pages, add files, extend the Markdown parser and report diagnostics. Most sites do not need them: [directive templates](../02-custom-directives.md) and [script plugins](../03-script-plugins.md) run with the stock binary. Use Go plugins to extend the Markdown parser, to use Go libraries, or to ship a plugin as a Go package.
