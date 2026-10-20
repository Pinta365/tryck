package plugins

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/pkg/plugin"
)

// LinkCheck reports internal links to pages or files that do not exist
// (link.broken) and #fragments missing on their target page
// (link.anchor). Links are recorded per page in stage 2, after tryck has
// rewritten them to site URLs, and checked once every page is known.
type LinkCheck struct {
	FailOnBroken bool `yaml:"failOnBroken"`

	site  plugin.SiteInfo
	mu    sync.Mutex
	pages []linkedPage
}

type linkedPage struct {
	url   string
	diags plugin.Reporter
	links []pageLink
}

type pageLink struct {
	href string
	line int
}

func (*LinkCheck) Name() string { return "linkCheck" }

// Priority runs the check after other finalizers have added their files.
func (*LinkCheck) Priority() int        { return 1000 }
func (*LinkCheck) ConfigKeys() []string { return []string{"failOnBroken"} }

func (l *LinkCheck) Init(cfg map[string]any, site plugin.SiteInfo) error {
	l.FailOnBroken = true
	l.site = site
	return decode(cfg, l)
}

func (l *LinkCheck) OnPageParsed(p *plugin.Page) error {
	lp := linkedPage{url: p.URL, diags: p.Diags}
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var dest []byte
		switch t := n.(type) {
		case *ast.Link:
			dest = t.Destination
		case *ast.Image:
			dest = t.Destination
		default:
			return ast.WalkContinue, nil
		}
		lp.links = append(lp.links, pageLink{href: string(dest), line: inlineLine(p, n)})
		return ast.WalkContinue, nil
	})
	if len(lp.links) > 0 {
		l.mu.Lock()
		l.pages = append(l.pages, lp)
		l.mu.Unlock()
	}
	return nil
}

// inlineLine finds the file line of an inline node through its first
// text descendant, or its block's first line.
func inlineLine(p *plugin.Page, n ast.Node) int {
	offset := -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			offset = t.Segment.Start
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	for b := n; offset < 0 && b != nil; b = b.Parent() {
		if b.Type() == ast.TypeBlock && b.Lines().Len() > 0 {
			offset = b.Lines().At(0).Start
		}
	}
	if offset < 0 {
		return 0
	}
	return p.LineAt(offset)
}

func (l *LinkCheck) OnBuildComplete(s plugin.SiteView, _ plugin.Outputs) error {
	broken := plugin.Warning
	if l.FailOnBroken && !l.site.Dev {
		broken = plugin.Error
	}
	var known []string
	for _, p := range s.Pages() {
		known = append(known, p.URL)
	}
	base := basePath(l.site.BaseURL)
	slices.SortFunc(l.pages, func(a, b linkedPage) int { return strings.Compare(a.url, b.url) })
	for _, lp := range l.pages {
		for _, link := range lp.links {
			target, frag, internal := l.target(lp.url, link.href, base)
			if !internal {
				continue
			}
			if target != lp.url && !s.HasFile(target) {
				msg := fmt.Sprintf("%s does not exist", target)
				if s := suggest(target, known); s != "" {
					msg += fmt.Sprintf(" (did you mean %s?)", s)
				}
				lp.diags.Report(plugin.Diagnostic{Severity: broken, Line: link.line, Code: "link.broken", Message: msg})
				continue
			}
			if frag == "" {
				continue
			}
			if _, isPage := s.PageByURL(target); isPage && !s.HasAnchor(target, frag) {
				lp.diags.Report(plugin.Diagnostic{Severity: plugin.Warning, Line: link.line, Code: "link.anchor",
					Message: fmt.Sprintf("#%s not found on %s", frag, target)})
			}
		}
	}
	return nil
}

// target resolves href as a browser would on pageURL and returns the
// site path (without baseUrl's path) and fragment. External links
// return internal=false. Links to baseUrl count as internal.
func (l *LinkCheck) target(pageURL, href, base string) (site, frag string, internal bool) {
	u, err := url.Parse(href)
	if err != nil {
		return "", "", false
	}
	if u.Scheme != "" || u.Host != "" {
		bu, err := url.Parse(l.site.BaseURL)
		if err != nil || bu.Host == "" || !strings.EqualFold(u.Host, bu.Host) || !strings.HasPrefix(u.Path, base) {
			return "", "", false
		}
		u.Scheme, u.Host = "", ""
	}
	p := u.Path
	switch {
	case p == "":
		p = pageURL
	case strings.HasPrefix(p, "/"):
		if !strings.HasPrefix(p, base) && p+"/" != base {
			return p, u.Fragment, true // outside the site's path: reported as missing
		}
		p = "/" + strings.TrimPrefix(p, base)
	default:
		p = path.Join(pageURL, p)
		if strings.HasSuffix(u.Path, "/") {
			p += "/"
		}
	}
	return p, u.Fragment, true
}

// basePath is baseUrl's path, always ending in "/".
func basePath(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Path == "" {
		return "/"
	}
	if !strings.HasSuffix(u.Path, "/") {
		return u.Path + "/"
	}
	return u.Path
}

// suggest returns the known URL closest to target, if it is close.
func suggest(target string, known []string) string {
	best, bestDist := "", len(target)/5+2
	for _, k := range known {
		if d := levenshtein(target, k); d < bestDist {
			best, bestDist = k, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
