---
title: Unreleased
draft: true
description: Changes on main since the last release.
---

<!-- Add a line for each change that lands on main, as in the release notes. The release tool moves them into the next release's notes. -->

- **Math.** A new built-in plugin, `math`, renders TeX formulas written as `$...$`, `$$...$$` or ` ```math ` blocks to MathML at build time with KaTeX, so pages need no script or stylesheet for them. Prices like `$5 and $10` stay text. See [Math](../02-writing/05-code-and-diagrams.md#math).
- **Link previews.** A new built-in plugin, `social`, adds Open Graph and Twitter card tags to every page, so shared links show the page's title, description and an image. Set a card image for the site with `plugins.social.image`, or for one page with `image` in its front matter. See [social](../05-extending/01-built-in-plugins.md#social).
