package plugins

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Redirects writes a meta-refresh page for every front matter alias:
// "/old/" becomes old/index.html, "/old.html" becomes old.html.
type Redirects struct {
	site plugin.SiteInfo
}

func (*Redirects) Name() string         { return "redirects" }
func (*Redirects) ConfigKeys() []string { return nil }

func (r *Redirects) Init(_ map[string]any, site plugin.SiteInfo) error {
	r.site = site
	return nil
}

var redirectPage = template.Must(template.New("redirect").Parse(`<!DOCTYPE html>
<html lang="{{ .Lang }}">
<head>
<meta charset="utf-8">
<title>Redirecting to {{ .Title }}</title>
<link rel="canonical" href="{{ .Abs }}">
<meta name="robots" content="noindex">
<meta http-equiv="refresh" content="0; url={{ .Rel }}">
</head>
<body>
<p>This page has moved to <a href="{{ .Rel }}">{{ .Title }}</a>.</p>
</body>
</html>
`))

func (r *Redirects) OnBuildComplete(v plugin.SiteView, out plugin.Outputs) error {
	for _, p := range v.Pages() {
		for _, alias := range p.Aliases {
			if !strings.HasPrefix(alias, "/") {
				out.Report(plugin.Diagnostic{Severity: plugin.Error, Code: "redirect.alias",
					Message: fmt.Sprintf("alias %q of %s must start with /", alias, p.URL)})
				continue
			}
			file := strings.TrimPrefix(alias, "/")
			if file == "" || strings.HasSuffix(file, "/") {
				file += "index.html"
			} else if !strings.HasSuffix(file, ".html") {
				file += "/index.html"
			}
			var buf strings.Builder
			err := redirectPage.Execute(&buf, map[string]string{
				"Lang":  r.site.Language,
				"Title": p.Title,
				"Rel":   render.RelURL(r.site.BaseURL, p.URL),
				"Abs":   render.AbsURL(r.site.BaseURL, p.URL),
			})
			if err != nil {
				return err
			}
			if err := out.Add(file, []byte(buf.String())); err != nil {
				out.Report(plugin.Diagnostic{Severity: plugin.Error, Code: "redirect.alias",
					Message: fmt.Sprintf("alias %s of %s: %v", alias, p.URL, err)})
			}
		}
	}
	return nil
}
