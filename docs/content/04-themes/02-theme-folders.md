---
title: Theme folders
description: Package layouts, static files and defaults in a reusable theme.
---

A theme folder packages a look for reuse: layouts, static files and default theme settings. Point `theme.path` at one:

```yaml
theme:
  path: themes/acme
```

```
themes/acme/
├── theme.yaml      defaults for the theme section
├── layouts/        layout files, by name
├── static/         files copied to the site root
└── plugins/        script plugins
```

The path must be a directory; anything else is a `config.invalid` error.

## theme.yaml

`theme.yaml` supplies default values for the `theme` section. It accepts the same keys as `theme` in the site config, except `path`:

```yaml
accent: "#0b7a5a"
colorScheme: auto
footer: "Built with tryck."
params:
  repo: https://github.com/you/project
```

Settings in `tryck.config.yaml` win over `theme.yaml` key by key, so a site can change the accent of a theme without restating anything else. A key that is not a theme setting produces a `config.unknownKey` warning.

## Lookup order

For layouts, static files and script plugins, tryck looks in this order, later entries winning:

1. the built-in theme,
2. the theme folder (if `theme.path` is set),
3. your site's `layouts/` and `static/`.

So a file in your site's `layouts/` or `static/` replaces the theme's file with the same path, which replaces the built-in version. You only need to override the files you change.

Files in a theme's `layouts/` and `static/` are keyed by their path relative to the folder, so `themes/acme/layouts/partials/header.html` replaces the built-in `partials/header.html`, and `themes/acme/static/css/tryck.css` replaces the built-in stylesheet.

## Params

Anything a theme's layouts need from the site goes under `theme.params` and is available as `.Theme.Params`:

```yaml
theme:
  path: themes/acme
  params:
    repo: https://github.com/you/project
```

```html
<a href="{{ .Theme.Params.repo }}">Source</a>
```

## Inside a site

A theme does not have to be a separate repository. A folder next to `content/` works the same way, which is a convenient way to keep a site's custom layouts and defaults together. See [Layouts and templates](03-layouts-and-templates.md) for how layouts are named.
