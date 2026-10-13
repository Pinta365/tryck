# Docs starter

A minimal [tryck](https://github.com/Pinta365/tryck) site.

```sh
tryck dev     # live reload on http://127.0.0.1:8080/
tryck build   # write the site to dist/
```

- Pages live in `content/`. Folders become sidebar sections; `_index.md` titles a section.
- Files in `static/` are copied to the site root.
- To change the look, copy a file from [tryck's built-in layouts](https://github.com/Pinta365/tryck/tree/main/internal/render/layouts) into `layouts/` under the same name (for example `layouts/partials/header.html`) and edit it.
- `.github/workflows/pages.yml` deploys to GitHub Pages on every push to `main`.
