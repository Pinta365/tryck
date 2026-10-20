---
title: Custom directives
description: "Add your own ::: directives with a template file, no code or build step."
---

A template file in `layouts/directives/` adds a directive. `layouts/directives/card.html` makes `:::card` available on every page, rendered by that template. There is nothing to compile, and `tryck dev` picks up new and changed templates as you save.

Use custom directives for reusable pieces of a page: cards, callouts with a fixed layout, embeds, badges, API boxes.

## An example

This site has a `card` directive. Its template is `layouts/directives/card.html`:

```html
<div class="card{{ with .Attrs.class }} {{ . }}{{ end }}"{{ with .Attrs.id }} id="{{ . }}"{{ end }}>
{{- with .Title }}
<p class="card-title">{{ with $.Attrs.href }}<a href="{{ relURL . }}">{{ $.Title }}</a>{{ else }}{{ . }}{{ end }}</p>
{{- end }}
{{ .Content }}
</div>
```

and a few lines of CSS in `static/css/docs.css`, linked with `theme.css`. In a page:

:::::tabs{group=example}
@tab Result
:::card[Custom directives]{href="/extending/custom-directives/"}
Add your own `:::` blocks with a template file. **Markdown** works inside.
:::

:::card[Go plugins]{href="/extending/go-plugins/"}
Compile plugins into your own tryck binary when a template is not enough.
:::
@tab Markdown
````markdown
:::card[Custom directives]{href="/extending/custom-directives/"}
Add your own `:::` blocks with a template file. **Markdown** works inside.
:::

:::card[Go plugins]{href="/extending/go-plugins/"}
Compile plugins into your own tryck binary when a template is not enough.
:::
````
:::::

## Template data

A directive template is a Go `html/template`, like a [layout](../04-themes/03-layouts-and-templates.md). It gets:

| Field | Type | Meaning |
| :- | :- | :- |
| `.Name` | string | The directive name, `card`. |
| `.Title` | string | The text in `[...]`, or empty. |
| `.Attrs` | map of strings | Every attribute in `{...}`: `.Attrs.id` from `#id`, `.Attrs.class` from `.class` (several are joined with spaces), and any `key=value`. A bare name such as `{wide}` is `"true"`. |
| `.Content` | HTML | The rendered Markdown body. |
| `.Site` | SiteInfo | `Title`, `Description`, `BaseURL`, `Language`, `Dev`. |

The template functions (`relURL`, `markdownify`, `default` and the others) and your partials are available. Values are escaped for where they appear, so a title or attribute cannot inject HTML.

Unlike the built-in directives, which only pass through a few safe attributes, a template receives all of them. It decides what to output.

## Where the body goes

Write `{{ .Content }}` once, where the body belongs. Everything the template outputs before it comes before the body, and everything after comes after. The body can hold any Markdown, including other directives.

- `.Content` must be in the HTML itself, not inside an attribute value.
- `.Content` marks a position; it is not the body as a value. Use it as shown, not in `if`, `len` or other functions.

A template without `.Content` drops the body, which suits embeds and other self-contained pieces. A YouTube embed, `layouts/directives/youtube.html`:

```html
<div class="video">
<iframe src="https://www.youtube-nocookie.com/embed/{{ .Attrs.id }}" title="{{ default "Video" .Title }}" allowfullscreen loading="lazy"></iframe>
</div>
```

```markdown
:::youtube[Getting started]{id=dQw4w9WgXcQ}
:::
```

## Links in attributes

Markdown links in the body are rewritten and checked like any other link. Attribute values are passed as written, so an `href` attribute takes a site path such as `"/extending/go-plugins/"`. Values with spaces, `/` or other punctuation need double quotes; single quotes are not accepted. Pass it through `relURL` in the template so it gets the base path. Links in attributes are not checked.

## Where templates are found

Directive templates follow the same lookup as other layouts: a file in the site's `layouts/directives/` replaces one with the same name in the theme folder's `layouts/directives/`. A [theme folder](../04-themes/02-theme-folders.md) can therefore ship directives along with its layouts, and a site can override them one by one.

## Names and errors

- The file name is the directive name: letters, digits, `-` and `_`, starting with a letter.
- The built-in names (`note`, `tip`, `info`, `warning`, `danger`, `details` and `tabs`) cannot be replaced; a template with one of them is a `template.parse` error.
- A name rendered by both a template and a [Go plugin](05-go-plugins/02-writing-go-plugins.md) is a `plugin.config` error.
- A template that fails to parse is a `template.parse` error pointing at the template. One that fails while rendering is a `markdown.render` error on the page that uses it.
- A `:::` name with no template or plugin renders as a plain `<div>` with a `directive.unknown` warning. Adding the template removes the warning.

See [Diagnostics](../07-diagnostics.md) for all codes.
