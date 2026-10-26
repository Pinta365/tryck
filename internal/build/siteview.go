package build

import (
	"strings"

	"github.com/Pinta365/tryck/internal/nav"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// siteView is the frozen, read-only site plugins and templates see. It
// holds copies, not pointers into build state.
type siteView struct {
	pages      []*plugin.PageInfo
	info       map[string]*plugin.PageInfo // by URL
	nav        *plugin.NavNode             // the current version's
	navs       map[string]*plugin.NavNode  // by version
	anchors    map[string]map[string]struct{}
	site       plugin.SiteInfo
	prev, next map[string]*plugin.PageInfo // nav order
	copied     map[string]bool             // output paths of assets and static files
	outputs    map[string][]byte           // generated files; only grows while finalizing
}

func newSiteView(s *Site, site plugin.SiteInfo, staticFiles []string) *siteView {
	v := &siteView{
		info:    make(map[string]*plugin.PageInfo, len(s.Pages)),
		nav:     s.Nav,
		navs:    s.Navs,
		anchors: s.Anchors,
		site:    site,
		prev:    map[string]*plugin.PageInfo{},
		next:    map[string]*plugin.PageInfo{},
		copied:  map[string]bool{},
		outputs: s.Outputs,
	}
	for _, a := range s.Assets {
		v.copied[a.OutPath] = true
	}
	for _, f := range staticFiles {
		v.copied[f] = true
	}
	for _, p := range s.Pages {
		pi := pageInfo(p)
		v.pages = append(v.pages, pi)
		v.info[p.URL] = pi
	}
	// Prev and next stay within a version.
	for _, tree := range s.Navs {
		order := nav.Walk(tree)
		for i := 1; i < len(order); i++ {
			v.prev[order[i]] = v.info[order[i-1]]
			v.next[order[i-1]] = v.info[order[i]]
		}
	}
	return v
}

func (v *siteView) Pages() []*plugin.PageInfo { return v.pages }

func (v *siteView) PageByURL(url string) (*plugin.PageInfo, bool) {
	p, ok := v.info[url]
	return p, ok
}

func (v *siteView) Nav() *plugin.NavNode { return v.nav }

// VersionNav returns the navigation tree of a version by name. It is not
// part of plugin.SiteView; built-in plugins that write a file per
// version look for it.
func (v *siteView) VersionNav(name string) *plugin.NavNode { return v.navs[name] }
func (v *siteView) Config() plugin.SiteInfo                { return v.site }

func (v *siteView) HasAnchor(url, id string) bool {
	_, ok := v.anchors[url][id]
	return ok
}

func (v *siteView) HasFile(sitePath string) bool {
	if _, ok := v.info[sitePath]; ok {
		return true
	}
	rel := strings.TrimPrefix(sitePath, "/")
	if strings.HasSuffix(rel, "/") || rel == "" {
		rel += "index.html"
	}
	if _, ok := v.outputs[rel]; ok {
		return true
	}
	return v.copied[rel]
}

var _ plugin.SiteView = (*siteView)(nil)
