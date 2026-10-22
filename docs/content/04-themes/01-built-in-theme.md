---
title: Built-in theme
description: The theme options and the CSS variables behind them.
---

tryck ships one theme. It is configured in the `theme` section of the config file and styled by `/css/tryck.css`, which uses CSS variables you can override from your own stylesheet.

## Options

| Key | Default | Meaning |
| :- | :- | :- |
| `accent` | `""` | CSS colour for links, the active navigation item and tabs. |
| `accentDark` | `""` | Accent in dark mode. Empty derives one by mixing `accent` with white. |
| `logo` | `""` | Site path of an image shown before the site title in the header. |
| `colorScheme` | `auto` | `auto`, `light` or `dark`. See below. |
| `nav` | `[]` | Header links, each with a `title` and `url`. |
| `footer` | `""` | Inline Markdown shown at the bottom of every page. |
| `editUrl` | `""` | Adds an "Edit this page" link to each page. See below. |
| `css` | `[]` | Extra stylesheets, linked after the theme's own, so they win. |
| `params` | `{}` | Free-form values for custom themes, available as `.Theme.Params`. |

```yaml
theme:
  accent: "#0b7a5a"
  logo: /img/logo.svg
  nav:
    - title: GitHub
      url: https://github.com/you/project
  footer: "© 2026 You. Built with [tryck](https://github.com/Pinta365/tryck)."
  css: [/css/custom.css]
```

`accent` and `accentDark` must be CSS colours: a hex value, a colour name, or a function like `rgb(11 122 90)` or `oklch(0.6 0.1 160)`. Anything that could end the CSS declaration is rejected with a `config.invalid` error.

## Page extras

The built-in theme adds a few things readers expect on a docs page:

- **Copy buttons** on code blocks copy the code without line numbers. They show on hover or keyboard focus, and always on touch screens.
- **Heading anchors**: a `#` next to every heading below the page title links to that section. Turn them off with `defaults.headingAnchors: false`.
- **Edit this page**: set `theme.editUrl` to the URL that edits a file in your repository, up to the project root. tryck appends each page's path from there:

  ```yaml
  theme:
    editUrl: https://github.com/you/project/edit/main/
  ```

  `content/guide/setup.md` then links to `https://github.com/you/project/edit/main/content/guide/setup.md`. If the site is in a subfolder of the repository, such as `docs/`, include it: `.../edit/main/docs/`.
- **Last updated**: with `build.lastUpdated: true`, each page shows the date of the last commit that changed its file. tryck reads it with one `git log` when it builds; without git or a repository, pages simply show no date.

  :::warning[Shallow clones]
  CI checkouts are often shallow, so every file looks changed by the latest commit. On GitHub Actions, check out with `fetch-depth: 0`.
  :::

## Colour scheme

- `auto` follows the operating system's light/dark preference and adds a toggle button to the header. The choice is remembered in the browser's local storage.
- `light` and `dark` force that scheme and do not show the toggle.

A forced or toggled scheme is set as a `data-theme` attribute on `<html>`. The accent, admonition colours, syntax highlighting and mermaid diagrams all follow the active scheme.

## CSS variables

Override these in a stylesheet listed in the `theme.css` option. The light values are on `:root`; dark values apply under `data-theme="dark"`, or from the system preference when `data-theme` is not `light`.

| Variable | Used for |
| :- | :- |
| `--fg`, `--muted` | Body text and secondary text. |
| `--bg`, `--bg-alt` | Page background and the background of code blocks, admonitions and inline code. |
| `--line` | Borders and table lines. |
| `--link` | Link colour when no `accent` is set. |
| `--accent` | Links, active navigation and tabs; set from `theme.accent`. |
| `--warn-bg`, `--warn-line` | The draft banner. |
| `--note`, `--tip`, `--info`, `--warning`, `--danger` | The five admonition kinds. |
| `--font` | Body font. |
| `--font-mono` | Code and monospace font. |

For example, a `static/css/custom.css` listed under `theme.css`:

```css
:root {
  --font: Georgia, "Times New Roman", serif;
  --font-mono: "JetBrains Mono", monospace;
}
```

When `theme.accent` is set, tryck writes `--accent` in an inline `<style>` after all stylesheets, so it wins over an `--accent` set in your CSS. To control the accent from a stylesheet, leave `theme.accent` empty.

## Accessibility and print

The built-in theme is checked with [axe](https://github.com/dequelabs/axe-core) against WCAG 2.2 AA in light and dark mode, including the search box and the version picker. Code blocks that scroll sideways can be scrolled with the keyboard, the search box and tabs work with the keyboard, and the search button, tabs and version picker are in place before the scripts run, so nothing jumps while the page loads.

Printed pages leave out the header, navigation, table of contents and search, show every tab with its label, and wrap long code lines.

If you change colours, keep text at a contrast of at least 4.5:1 against its background. Line numbers in code blocks use `--muted`.

## A note on files

A file in `static/` with the same path as a theme file replaces it. To replace the built-in stylesheet entirely, add `static/css/tryck.css`. Otherwise, list an extra stylesheet in `theme.css` and override only the variables you need. See [Theme folders](02-theme-folders.md) for the full lookup order.
