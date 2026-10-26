package plugins

import (
	"cmp"
	"fmt"
	"html/template"
	"image"
	_ "image/jpeg" // card images are PNG or JPEG
	_ "image/png"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Social adds Open Graph and Twitter card tags to every page, so a link
// shared in a chat or social network shows the page's title, description
// and an image. Image is the site's card image; a page's "image" front
// matter replaces it. Both are site paths ("/img/card.png") or absolute
// URLs. The tags need absolute URLs, so a site.baseUrl without a host
// gets a social.baseUrl warning and no tags.
type Social struct {
	Image   string `yaml:"image"`
	Twitter string `yaml:"twitter"`
	// StaticDirs are searched for a local image's size, later first.
	StaticDirs []string `yaml:"-"`

	site   plugin.SiteInfo
	origin bool // site.baseUrl has a host

	mu     sync.Mutex
	images map[string]string // site path of each local image used -> a page using it
	sizes  map[string]image.Point
}

func (*Social) Name() string         { return "social" }
func (*Social) ConfigKeys() []string { return []string{"image", "twitter"} }

func (s *Social) Init(cfg map[string]any, site plugin.SiteInfo) error {
	*s = Social{StaticDirs: s.StaticDirs, site: site, images: map[string]string{}, sizes: map[string]image.Point{}}
	if err := decode(cfg, s); err != nil {
		return err
	}
	if err := checkImageRef(s.Image); err != nil {
		return fmt.Errorf("image: %w", err)
	}
	if s.Twitter != "" && !strings.HasPrefix(s.Twitter, "@") {
		s.Twitter = "@" + s.Twitter
	}
	u, err := url.Parse(site.BaseURL)
	s.origin = err == nil && u.Host != ""
	return nil
}

// checkImageRef accepts "", a site path or an http(s) URL.
func checkImageRef(ref string) error {
	if ref == "" || strings.HasPrefix(ref, "/") {
		return nil
	}
	if u, err := url.Parse(ref); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return nil
	}
	return fmt.Errorf("%q must be a site path starting with / or an http(s) URL", ref)
}

// OnPageRendered adds the tags to the page's <head>.
func (s *Social) OnPageRendered(p *plugin.Page, v plugin.SiteView) error {
	if !s.origin {
		return nil
	}
	info, ok := v.PageByURL(p.URL)
	if !ok {
		return nil
	}
	img := s.Image
	if ref, ok := info.Params["image"].(string); ok && ref != "" {
		if err := checkImageRef(ref); err != nil {
			p.Diags.Report(plugin.Diagnostic{Severity: plugin.Warning, Code: "social.image", Message: "front matter image " + err.Error()})
		} else {
			img = ref
		}
	}

	var b strings.Builder
	meta := func(attr, key, value string) {
		if value != "" {
			fmt.Fprintf(&b, "<meta %s=\"%s\" content=\"%s\">\n", attr, key, template.HTMLEscapeString(value))
		}
	}
	typ := "website"
	if !info.Date.IsZero() {
		typ = "article"
	}
	meta("property", "og:type", typ)
	meta("property", "og:site_name", s.site.Title)
	meta("property", "og:title", info.Title)
	meta("property", "og:description", oneLine(cmp.Or(info.Description, s.site.Description)))
	meta("property", "og:url", render.AbsURL(s.site.BaseURL, info.URL))
	card := "summary"
	if img != "" {
		card = "summary_large_image"
		meta("property", "og:image", render.AbsURL(s.site.BaseURL, img))
		if strings.HasPrefix(img, "/") {
			if size, ok := s.size(img, info.URL); ok {
				meta("property", "og:image:width", fmt.Sprint(size.X))
				meta("property", "og:image:height", fmt.Sprint(size.Y))
			}
		}
	}
	if typ == "article" {
		meta("property", "article:published_time", info.Date.Format(time.RFC3339))
	}
	meta("name", "twitter:card", card)
	meta("name", "twitter:site", s.Twitter)
	p.HTML = addToHead(p.HTML, b.String())
	return nil
}

// size returns the pixel size of a site path in the static directories,
// remembering that pageURL uses it so a missing file can be reported.
func (s *Social) size(sitePath, pageURL string) (image.Point, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.images[sitePath]; !ok {
		s.images[sitePath] = pageURL
	}
	if pt, ok := s.sizes[sitePath]; ok {
		return pt, pt.X > 0
	}
	var pt image.Point
	for _, dir := range slices.Backward(s.StaticDirs) {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(sitePath, "/"))))
		if err != nil {
			continue
		}
		if c, _, err := image.DecodeConfig(f); err == nil {
			pt = image.Point{X: c.Width, Y: c.Height}
		}
		f.Close()
		break
	}
	s.sizes[sitePath] = pt
	return pt, pt.X > 0
}

// OnBuildComplete warns about a baseUrl without a host and about card
// images that are not on the site.
func (s *Social) OnBuildComplete(v plugin.SiteView, out plugin.Outputs) error {
	if !s.origin {
		out.Report(plugin.Diagnostic{Severity: plugin.Info, Code: "social.baseUrl",
			Message: "social cards need absolute URLs; set site.baseUrl to an http(s) URL or disable plugins.social"})
		return nil
	}
	for _, img := range slices.Sorted(maps.Keys(s.images)) {
		if !v.HasFile(img) {
			out.Report(plugin.Diagnostic{Severity: plugin.Warning, Code: "social.image",
				Message: fmt.Sprintf("card image %s does not exist (used by %s)", img, s.images[img])})
		}
	}
	return nil
}
