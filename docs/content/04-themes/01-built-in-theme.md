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

## A note on files

A file in `static/` with the same path as a theme file replaces it. To replace the built-in stylesheet entirely, add `static/css/tryck.css`. Otherwise, list an extra stylesheet in `theme.css` and override only the variables you need. See [Theme folders](02-theme-folders.md) for the full lookup order.
