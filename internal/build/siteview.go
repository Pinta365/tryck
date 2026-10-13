package build

import (
	"github.com/Pinta365/tryck/internal/nav"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// siteView is the frozen, read-only site plugins and templates see. It
// holds copies, not pointers into build state.
type siteView struct {
	pages      []*plugin.PageInfo
	info       map[string]*plugin.PageInfo // by URL
	nav        *plugin.NavNode
	anchors    map[string]map[string]struct{}
	site       plugin.SiteInfo
	prev, next map[string]*plugin.PageInfo // nav order
}

func newSiteView(s *Site, site plugin.SiteInfo) *siteView {
	v := &siteView{
		info:    make(map[string]*plugin.PageInfo, len(s.Pages)),
		nav:     s.Nav,
		anchors: s.Anchors,
		site:    site,
		prev:    map[string]*plugin.PageInfo{},
		next:    map[string]*plugin.PageInfo{},
	}
	for _, p := range s.Pages {
		pi := pageInfo(p)
		v.pages = append(v.pages, pi)
		v.info[p.URL] = pi
	}
	order := nav.Walk(s.Nav)
	for i := 1; i < len(order); i++ {
		v.prev[order[i]] = v.info[order[i-1]]
		v.next[order[i-1]] = v.info[order[i]]
	}
	return v
}

func (v *siteView) Pages() []*plugin.PageInfo { return v.pages }

func (v *siteView) PageByURL(url string) (*plugin.PageInfo, bool) {
	p, ok := v.info[url]
	return p, ok
}

func (v *siteView) Nav() *plugin.NavNode    { return v.nav }
func (v *siteView) Config() plugin.SiteInfo { return v.site }

func (v *siteView) HasAnchor(url, id string) bool {
	_, ok := v.anchors[url][id]
	return ok
}

var _ plugin.SiteView = (*siteView)(nil)
