// Package plugin is tryck's public plugin API.
//
// Plugins are compiled into a custom binary with tryck.Main (package
// pkg/tryck). Every plugin implements Plugin plus any of the phase
// interfaces below. A build parses every page, then renders every page,
// then finalizes once; each phase calls the plugins that implement it,
// ordered by Priority (lower first, default 100) and then by Name.
//
// Plugins see copies and read-only views, never the build's own state.
package plugin

import (
	"io"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
)

// Plugin is implemented by every plugin.
type Plugin interface {
	// Name is the plugin's config key under "plugins:", e.g. "readingTime".
	Name() string
	// Init receives the plugin's own config section (nil if absent) and
	// the site settings. It runs once per build before any phase.
	Init(cfg map[string]any, site SiteInfo) error
}

// Ordered sets the order within a phase. Lower runs first; default 100.
type Ordered interface{ Priority() int }

// DefaultPriority applies to plugins that do not implement Ordered.
const DefaultPriority = 100

// PageParser runs per page and in parallel, after the page's
// Markdown is parsed. It may modify its own page only, and must not
// modify p.AST: tryck dev reuses parsed ASTs between builds. Change the
// document with a MarkdownExtender (an AST transformer) instead.
type PageParser interface {
	OnPageParsed(p *Page) error
}

// PageRenderer runs per page and in parallel, after the
// layout has been executed. It may modify p.HTML and p.Data. The site
// is read-only.
type PageRenderer interface {
	OnPageRendered(p *Page, s SiteView) error
}

// Finalizer runs once, after every page is rendered. It may
// add output files.
type Finalizer interface {
	OnBuildComplete(s SiteView, out Outputs) error
}

// MarkdownExtender extends the goldmark instance (parsers, AST
// transformers, renderers). It runs once per build, before parsing.
type MarkdownExtender interface {
	Extend(m goldmark.Markdown)
}

// DirectiveRenderer renders ::: directives with names tryck does not
// know. Registered names do not produce directive.unknown warnings.
type DirectiveRenderer interface {
	// DirectiveNames lists the directive names this plugin renders.
	DirectiveNames() []string
	// RenderDirective writes the opening HTML (entering) or closing HTML
	// (!entering) of d. The directive's Markdown content is rendered
	// between the two calls.
	RenderDirective(w io.Writer, d Directive, entering bool) error
}

// Stylesheets lists site paths of CSS files a plugin needs on every
// page, such as "/css/chroma.css". Layouts link them in <head>.
type Stylesheets interface {
	Stylesheets() []string
}

// ConfigKeys is implemented by plugins that declare their config keys,
// so typos in their section produce config.unknownKey warnings.
// "enabled" is always allowed.
type ConfigKeys interface{ ConfigKeys() []string }

// Directive describes a ::: block for a DirectiveRenderer.
type Directive struct {
	Name  string
	Title string            // from :::name[title]
	Attrs map[string]string // from {key=value .class #id}; class and id included
}

// Severity of a Diagnostic.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

// Diagnostic is a problem a plugin reports about a page. Line and Col
// are 1-based file positions, or 0 when unknown.
type Diagnostic struct {
	Severity Severity
	Line     int
	Col      int
	Code     string // stable id, e.g. "link.broken"
	Message  string
}

// Reporter receives diagnostics for one page. It is safe for concurrent
// use.
type Reporter interface {
	Report(d Diagnostic)
}

// Page is a page being built. A plugin receives only the page it is
// called for.
type Page struct {
	SourcePath string         // relative to its version's content directory, forward slashes
	Version    string         // the page's version; "" on an unversioned site
	URL        string         // "/docs/setup/"
	Front      FrontMatter    // typed front matter
	Data       map[string]any // plugin-added values, e.g. "readingTime", "toc"
	AST        ast.Node       // goldmark document; read-only for plugins
	Source     []byte         // body without front matter
	BodyLine   int            // file line on which Source starts
	HTML       []byte         // the rendered page, set after the layout runs
	Diags      Reporter       // per-page diagnostics
}

// LineAt converts a byte offset in Source to a 1-based file line.
func (p *Page) LineAt(offset int) int {
	offset = min(max(offset, 0), len(p.Source))
	line := p.BodyLine
	for _, c := range p.Source[:offset] {
		if c == '\n' {
			line++
		}
	}
	return line
}

// FrontMatter is the YAML header of a page.
type FrontMatter struct {
	Title       string         `yaml:"title" json:"title,omitempty"`
	Description string         `yaml:"description" json:"description,omitempty"`
	Layout      string         `yaml:"layout" json:"layout,omitempty"`
	Date        time.Time      `yaml:"date" json:"date,omitempty"`
	Draft       bool           `yaml:"draft" json:"draft,omitempty"`
	Weight      int            `yaml:"weight" json:"weight,omitempty"`     // nav ordering
	NavTitle    string         `yaml:"navTitle" json:"navTitle,omitempty"` // shorter sidebar label
	TOC         *bool          `yaml:"toc" json:"toc,omitempty"`           // nil = config default
	Slug        string         `yaml:"slug" json:"slug,omitempty"`         // overrides URL segment
	Aliases     []string       `yaml:"aliases" json:"aliases,omitempty"`   // old URLs -> redirect pages
	Params      map[string]any `yaml:",inline" json:"params,omitempty"`
}

// SiteInfo holds site-wide settings.
type SiteInfo struct {
	Title       string
	Description string
	BaseURL     string // always ends in "/"
	Language    string
	Version     string // the current version's name; "" on an unversioned site
	Dev         bool   // building for tryck dev
}

// PageInfo is a read-only copy of a page for templates and SiteView.
// Changing it does not affect the build.
type PageInfo struct {
	SourcePath  string
	Version     string // "" on an unversioned site
	Title       string
	NavTitle    string
	Description string
	URL         string
	Date        time.Time
	Draft       bool
	Weight      int
	TitleInBody bool           // the body has its own h1, so layouts skip theirs
	Aliases     []string       // old URLs that redirect here
	Params      map[string]any // front matter keys tryck does not know
	Data        map[string]any // plugin-added values as of the end of parsing
}

// NavNode is an entry in the navigation tree. The tree is shared and
// must not be modified.
type NavNode struct {
	Title    string
	URL      string // "" for a folder without an index page
	Draft    bool
	Section  bool // a folder, possibly with an index page
	Children []*NavNode
	Active   bool // the page being rendered (per-page views only)
	InPath   bool // the active page or one of its ancestors
}

// TOCEntry is a heading in a page's table of contents, stored by the
// toc plugin as Data["toc"] ([]TOCEntry).
type TOCEntry struct {
	Level    int
	ID       string
	Text     string
	Children []TOCEntry
}

// SiteView is a read-only view of the built site. It is safe for
// concurrent use.
type SiteView interface {
	Pages() []*PageInfo // sorted by URL
	PageByURL(url string) (*PageInfo, bool)
	Nav() *NavNode
	HasAnchor(url, id string) bool
	// HasFile reports whether a site path ("/img/logo.png", "/docs/")
	// is served: a page, a copied asset or static file, or a generated
	// file added so far.
	HasFile(sitePath string) bool
	Config() SiteInfo
}

// Outputs collects extra output files and site-wide diagnostics.
type Outputs interface {
	// Add writes data to path, relative to outputDir ("sitemap.xml").
	// It fails if the path is invalid or already produced.
	Add(path string, data []byte) error
	// Report records a diagnostic that is not about one page.
	Report(d Diagnostic)
}
