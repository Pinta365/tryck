package plugins

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Feed writes an Atom (or RSS) feed of the pages that have a date, newest
// first: one for each section in Sections, at <section>feed.xml. Drafts
// and older versions are left out, and a section without dated pages
// gets no feed. Pages link the feeds of their sections in <head>.
type Feed struct {
	Sections []string `yaml:"sections"`
	Limit    int      `yaml:"limit"`
	Format   string   `yaml:"format"`
	Author   string   `yaml:"author"`

	site      plugin.SiteInfo
	mu        sync.Mutex
	summaries map[string]string // page URL -> first paragraph
	once      sync.Once
	feeds     []feed
}

// feed is one section's feed: its file and entries.
type feed struct {
	section string // site path, "/" or "/blog/"
	file    string // output path, "feed.xml" or "blog/feed.xml"
	title   string
	pages   []*plugin.PageInfo
}

func (*Feed) Name() string         { return "feed" }
func (*Feed) ConfigKeys() []string { return []string{"sections", "limit", "format", "author"} }

func (f *Feed) Init(cfg map[string]any, site plugin.SiteInfo) error {
	f.Sections, f.Limit, f.Format, f.Author = []string{"/"}, 20, "atom", ""
	f.site = site
	f.summaries = map[string]string{}
	f.once = sync.Once{}
	f.feeds = nil
	if err := decode(cfg, f); err != nil {
		return err
	}
	if f.Format != "atom" && f.Format != "rss" {
		return fmt.Errorf("format must be atom or rss, got %q", f.Format)
	}
	if f.Limit <= 0 {
		return fmt.Errorf("limit must be positive, got %d", f.Limit)
	}
	for i, s := range f.Sections {
		s = "/" + strings.Trim(s, "/") + "/"
		f.Sections[i] = strings.Replace(s, "//", "/", 1)
	}
	return nil
}

// OnPageParsed keeps the page's first paragraph as its summary.
func (f *Feed) OnPageParsed(p *plugin.Page) error {
	if p.Front.Date.IsZero() {
		return nil
	}
	var summary string
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if para, ok := n.(*ast.Paragraph); ok && entering {
			summary = strings.Join(strings.Fields(content.PlainText(para, p.Source)), " ")
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	f.mu.Lock()
	f.summaries[p.URL] = summary
	f.mu.Unlock()
	return nil
}

// collect works out the feeds once every page is known.
func (f *Feed) collect(v plugin.SiteView) []feed {
	f.once.Do(func() {
		for _, section := range f.Sections {
			fd := feed{section: section, file: strings.TrimPrefix(section, "/") + "feed.xml", title: f.site.Title}
			if p, ok := v.PageByURL(section); ok && section != "/" {
				fd.title = strings.TrimSpace(f.site.Title + " · " + p.Title)
			}
			for _, p := range v.Pages() {
				if !p.Date.IsZero() && !p.Draft && p.Version == f.site.Version && strings.HasPrefix(p.URL, section) {
					fd.pages = append(fd.pages, p)
				}
			}
			slices.SortStableFunc(fd.pages, func(a, b *plugin.PageInfo) int {
				return cmp.Or(b.Date.Compare(a.Date), strings.Compare(a.URL, b.URL))
			})
			if len(fd.pages) > f.Limit {
				fd.pages = fd.pages[:f.Limit]
			}
			f.feeds = append(f.feeds, fd)
		}
	})
	return f.feeds
}

// OnPageRendered links the feeds of the page's sections in <head>.
func (f *Feed) OnPageRendered(p *plugin.Page, v plugin.SiteView) error {
	var links strings.Builder
	typ := "application/atom+xml"
	if f.Format == "rss" {
		typ = "application/rss+xml"
	}
	for _, fd := range f.collect(v) {
		if len(fd.pages) == 0 || !strings.HasPrefix(p.URL, fd.section) {
			continue
		}
		fmt.Fprintf(&links, "<link rel=\"alternate\" type=\"%s\" title=\"%s\" href=\"%s\">\n",
			typ, template.HTMLEscapeString(fd.title), template.HTMLEscapeString(render.RelURL(f.site.BaseURL, "/"+fd.file)))
	}
	p.HTML = addToHead(p.HTML, links.String())
	return nil
}

func (f *Feed) OnBuildComplete(v plugin.SiteView, out plugin.Outputs) error {
	feeds := f.collect(v)
	if u, err := url.Parse(f.site.BaseURL); err != nil || u.Host == "" {
		for _, fd := range feeds {
			if len(fd.pages) > 0 {
				out.Report(plugin.Diagnostic{Severity: plugin.Warning, Code: "feed.baseUrl",
					Message: "feeds need absolute URLs; set site.baseUrl to an http(s) URL or disable plugins.feed"})
				return nil
			}
		}
		return nil
	}
	for _, fd := range feeds {
		if len(fd.pages) == 0 {
			if fd.section != "/" {
				out.Report(plugin.Diagnostic{Severity: plugin.Warning, Code: "feed.section",
					Message: fmt.Sprintf("plugins.feed.sections: %s has no pages with a date, so it gets no feed", fd.section)})
			}
			continue
		}
		var data []byte
		var err error
		if f.Format == "rss" {
			data, err = f.rss(fd)
		} else {
			data, err = f.atom(fd)
		}
		if err != nil {
			return err
		}
		if err := out.Add(fd.file, data); err != nil {
			return err
		}
	}
	return nil
}

func (f *Feed) abs(sitePath string) string { return render.AbsURL(f.site.BaseURL, sitePath) }

func (f *Feed) summary(p *plugin.PageInfo) string {
	if p.Description != "" {
		return p.Description
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.summaries[p.URL]
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
}

type atomEntry struct {
	Title   string   `xml:"title"`
	Link    atomLink `xml:"link"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Summary string   `xml:"summary,omitempty"`
}

func (f *Feed) atom(fd feed) ([]byte, error) {
	author := cmp.Or(f.Author, f.site.Title, "unknown")
	doc := struct {
		XMLName xml.Name    `xml:"feed"`
		NS      string      `xml:"xmlns,attr"`
		Title   string      `xml:"title"`
		Links   []atomLink  `xml:"link"`
		ID      string      `xml:"id"`
		Updated string      `xml:"updated"`
		Author  string      `xml:"author>name"`
		Entries []atomEntry `xml:"entry"`
	}{
		NS:      "http://www.w3.org/2005/Atom",
		Title:   fd.title,
		Links:   []atomLink{{Href: f.abs(fd.section)}, {Href: f.abs("/" + fd.file), Rel: "self"}},
		ID:      f.abs(fd.section),
		Updated: fd.pages[0].Date.UTC().Format(time.RFC3339),
		Author:  author,
	}
	for _, p := range fd.pages {
		doc.Entries = append(doc.Entries, atomEntry{
			Title: p.Title, Link: atomLink{Href: f.abs(p.URL)}, ID: f.abs(p.URL),
			Updated: p.Date.UTC().Format(time.RFC3339), Summary: f.summary(p),
		})
	}
	return encodeXML(doc)
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description,omitempty"`
}

func (f *Feed) rss(fd feed) ([]byte, error) {
	type channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Items       []rssItem `xml:"item"`
	}
	doc := struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel channel  `xml:"channel"`
	}{Version: "2.0", Channel: channel{Title: fd.title, Link: f.abs(fd.section), Description: cmp.Or(f.site.Description, fd.title)}}
	for _, p := range fd.pages {
		doc.Channel.Items = append(doc.Channel.Items, rssItem{
			Title: p.Title, Link: f.abs(p.URL), GUID: f.abs(p.URL),
			PubDate: p.Date.UTC().Format(time.RFC1123Z), Description: f.summary(p),
		})
	}
	return encodeXML(doc)
}

func encodeXML(v any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
