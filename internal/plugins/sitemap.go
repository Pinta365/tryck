package plugins

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"net/url"

	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Sitemap writes /sitemap.xml with the absolute URL of every page that
// is not a draft and belongs to the current version. It needs a baseUrl
// with a host.
type Sitemap struct {
	site plugin.SiteInfo
}

func (*Sitemap) Name() string         { return "sitemap" }
func (*Sitemap) ConfigKeys() []string { return nil }

func (s *Sitemap) Init(_ map[string]any, site plugin.SiteInfo) error {
	s.site = site
	return nil
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func (s *Sitemap) OnBuildComplete(v plugin.SiteView, out plugin.Outputs) error {
	if u, err := url.Parse(s.site.BaseURL); err != nil || u.Host == "" {
		out.Report(plugin.Diagnostic{Severity: plugin.Warning, Code: "sitemap.baseUrl",
			Message: "sitemap.xml needs absolute URLs; set site.baseUrl to an http(s) URL or disable plugins.sitemap"})
		return nil
	}
	var urls []sitemapURL
	for _, p := range v.Pages() {
		// Older versions stay out of search engines.
		if p.Draft || p.Version != s.site.Version {
			continue
		}
		e := sitemapURL{Loc: render.AbsURL(s.site.BaseURL, p.URL)}
		if d := cmp.Or(p.Date, p.LastUpdated); !d.IsZero() {
			e.LastMod = d.Format("2006-01-02")
		}
		urls = append(urls, e)
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	err := enc.Encode(struct {
		XMLName xml.Name     `xml:"urlset"`
		NS      string       `xml:"xmlns,attr"`
		URLs    []sitemapURL `xml:"url"`
	}{NS: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls})
	if err != nil {
		return err
	}
	buf.WriteByte('\n')
	return out.Add("sitemap.xml", buf.Bytes())
}
