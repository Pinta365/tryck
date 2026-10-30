---
title: Unreleased
draft: true
description: Changes on main since the last release.
---

<!-- Add a line for each change that lands on main, as in the release notes. The release tool moves them into the next release's notes. -->

- **Share image descriptions.** The `social` plugin's `imageAlt` option, and `imageAlt` front matter next to a page's own `image`, describe the share image for screen readers (`og:image:alt`, `twitter:image:alt`).
- **Images without alt text** get an `image.alt` note with `--verbose`, so you can find images screen readers skip. It is a note, not a warning, since an empty alt text also marks a decorative image.
