// Package build turns a site's inputs into an immutable Site snapshot.
// Writing it to disk (build) or serving it from memory (dev) happens
// afterwards, so both share this code path.
package build

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/markdown"
	"github.com/Pinta365/tryck/internal/nav"
	"github.com/Pinta365/tryck/internal/plugins"
	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/internal/workpool"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// NotFoundPath is the output path of the 404 page, served by both
// Cloudflare Pages and GitHub Pages from the output root.
const NotFoundPath = "404.html"

// Site is the result of a build. It is not modified after Build returns.
type Site struct {
	Config  *config.Config
	Pages   []*content.Page // sorted by URL
	ByURL   map[string]*content.Page
	Nav     *nav.Node                      // read-only after stage 3
	Anchors map[string]map[string]struct{} // URL -> set of element IDs
	Assets  []content.Asset                // files from contentDir copied next to pages
	Outputs map[string][]byte              // output path -> bytes (pages + generated files)
	Diags   []diag.Diagnostic              // sorted
	BuiltAt time.Time
}

// HasErrors reports whether the build produced error-level diagnostics.
func (s *Site) HasErrors() bool { return diag.HasErrors(s.Diags) }

// Options adjust a build.
type Options struct {
	Dev     bool            // building for tryck dev
	Logger  *slog.Logger    // debug logs with per-stage timings; nil = silent
	Plugins []plugin.Plugin // compiled-in plugins in addition to the built-ins
	Cache   *Cache          // parsed pages reused between builds; nil = none
}

type builder struct {
	cfg     *config.Config
	opts    Options
	dc      diag.Collector
	md      *markdown.Processor
	tmpl    *render.Templates
	plugins *plugins.Set
	site    plugin.SiteInfo
	styles  []string
	theme   render.ThemeData
	view    *siteView // frozen after stage 3
	pages   []*content.Page
}

// Build runs the pipeline. It always returns a Site; check HasErrors.
// It returns ctx.Err() if the build was cancelled.
func Build(ctx context.Context, cfg *config.Config, opts Options) (*Site, error) {
	b := &builder{
		cfg:  cfg,
		opts: opts,
		site: plugin.SiteInfo{
			Title:       cfg.Site.Title,
			Description: cfg.Site.Description,
			BaseURL:     cfg.Site.BaseURL,
			Language:    cfg.Site.Language,
			Dev:         opts.Dev,
		},
	}
	s := &Site{Config: cfg, ByURL: map[string]*content.Page{}, Outputs: map[string][]byte{}}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil)) // debug is below its level
	}
	stageStart := time.Now()
	stage := func(name string, args ...any) {
		log.Debug("stage done", append([]any{"stage", name, "duration", time.Since(stageStart)}, args...)...)
		stageStart = time.Now()
	}

	// [1] Load
	set, ds := plugins.Load(cfg, b.site, opts.Plugins...)
	b.dc.Add(ds...)
	b.plugins = set
	b.styles = set.Stylesheets()
	directives, ds := set.Directives()
	b.dc.Add(ds...)
	b.md = markdown.New(markdown.Options{
		UnsafeHTML: cfg.Build.UnsafeHTML,
		Extenders:  plugins.Each[plugin.MarkdownExtender](set),
		Directives: directives,
		SitePath:   func(p string) string { return render.RelURL(cfg.Site.BaseURL, p) },
	})

	funcs := render.Funcs(render.FuncOptions{
		BaseURL:     cfg.Site.BaseURL,
		DateFormat:  cfg.Defaults.DateFormat,
		Markdownify: b.md.RenderInline,
	})
	var layoutDirs []render.Dir
	for _, d := range cfg.LayoutDirs() {
		layoutDirs = append(layoutDirs, render.Dir{Path: d, Label: b.rel(d)})
	}
	tmpl, ds := render.Load(layoutDirs, funcs)
	b.dc.Add(ds...)
	b.tmpl = tmpl
	b.theme = b.themeData()

	contentDir := cfg.Dir(cfg.Build.ContentDir)
	src, err := content.Discover(contentDir)
	if err != nil {
		b.dc.Errorf(b.rel(contentDir), 0, 0, "content.read", "cannot read content: %v", err)
	}
	s.Assets = src.Assets
	stage("load", "pages", len(src.Pages), "assets", len(src.Assets), "plugins", len(set.Plugins()))

	// [2] Parse, in parallel. A page only touches its own Page. Front
	// matter is read first so that link rewriting knows every page's URL.
	workers := cfg.WorkerCount()
	read := make([]*content.Page, len(src.Pages))
	err = workpool.Run(ctx, len(src.Pages), workers, func(i int) {
		read[i] = b.readPage(contentDir, src.Pages[i])
	})
	if err != nil {
		return nil, err
	}
	links := &linkResolver{baseURL: cfg.Site.BaseURL, pages: map[string]string{}, assets: map[string]string{}}
	for _, p := range read {
		if p != nil {
			b.pages = append(b.pages, p)
			links.pages[p.SourcePath] = p.URL
		}
	}
	for _, a := range src.Assets {
		links.assets[a.Rel] = a.OutPath
	}
	err = workpool.Run(ctx, len(b.pages), workers, func(i int) {
		b.parsePage(b.pages[i], links)
	})
	if err != nil {
		return nil, err
	}
	stage("parse", "workers", workers)

	// [3] Index, sequential. Pages, ByURL, Nav, Anchors and the plugin
	// view are read-only from here on.
	slices.SortFunc(b.pages, func(x, y *content.Page) int {
		return cmp.Or(strings.Compare(x.URL, y.URL), strings.Compare(x.SourcePath, y.SourcePath))
	})
	for _, p := range b.pages {
		if prev, ok := s.ByURL[p.URL]; ok {
			b.dc.Errorf(p.File, 0, 0, "url.conflict", "%s maps to %s, which is already used by %s", p.File, p.URL, prev.File)
			continue
		}
		s.ByURL[p.URL] = p
		s.Pages = append(s.Pages, p)
	}
	s.Nav = nav.Build(s.Pages, cfg.Site.Title)
	s.Anchors = make(map[string]map[string]struct{}, len(s.Pages))
	for _, p := range s.Pages {
		s.Anchors[p.URL] = anchors(p.AST)
	}
	static := map[string]bool{}
	for _, dir := range cfg.StaticDirs() {
		if err := content.WalkFiles(dir, func(_, rel string) { static[filepath.ToSlash(rel)] = true }); err != nil {
			b.dc.Errorf(b.rel(dir), 0, 0, "content.read", "cannot read static files: %v", err)
		}
	}
	b.addBuiltinStatic(s, static)
	b.view = newSiteView(s, b.site, slices.Sorted(maps.Keys(static)))
	stage("index")

	// [4] Render, in parallel.
	rendered := make([][]byte, len(s.Pages))
	err = workpool.Run(ctx, len(s.Pages), workers, func(i int) {
		if out, ok := b.render(s.Pages[i]); ok {
			rendered[i] = out
		}
	})
	if err != nil {
		return nil, err
	}
	for i, p := range s.Pages {
		if rendered[i] != nil {
			s.Outputs[p.OutPath] = rendered[i]
		}
	}
	b.renderNotFound(s)
	stage("render", "workers", workers)

	// [5] Finalize, sequential.
	for _, a := range s.Assets {
		if _, ok := s.Outputs[a.OutPath]; ok {
			b.dc.Errorf(b.rel(a.Src), 0, 0, "url.conflict", "%s would overwrite generated %s", b.rel(a.Src), a.OutPath)
		}
	}
	out := &outputs{files: s.Outputs, assets: map[string]bool{}, dc: &b.dc}
	for _, a := range s.Assets {
		out.assets[a.OutPath] = true
	}
	for _, f := range plugins.Each[plugin.Finalizer](set) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := f.OnBuildComplete(b.view, out); err != nil {
			b.dc.Errorf("", 0, 0, "plugin.error", "plugin %s: %v", f.(plugin.Plugin).Name(), err)
		}
	}
	stage("finalize")

	s.Diags = b.dc.All()
	s.BuiltAt = time.Now()
	opts.Cache.prune()
	return s, nil
}

// rel makes a path relative to the project root for diagnostics.
func (b *builder) rel(p string) string {
	if r, err := filepath.Rel(b.cfg.Root, p); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

// reporter collects plugin diagnostics for one page.
type reporter struct {
	dc   *diag.Collector
	file string
}

func (r reporter) Report(d plugin.Diagnostic) {
	r.dc.Add(diag.Diagnostic{Severity: diag.Severity(d.Severity), File: r.file, Line: d.Line, Col: d.Col, Code: d.Code, Message: d.Message})
}

// readPage reads one Markdown file and its front matter. It returns nil
// for drafts that are excluded and for files that cannot be read.
func (b *builder) readPage(contentDir, rel string) *content.Page {
	abs := filepath.Join(contentDir, filepath.FromSlash(rel))
	file := b.rel(abs)
	raw, err := os.ReadFile(abs)
	if err != nil {
		b.dc.Errorf(file, 0, 0, "content.read", "cannot read page: %v", err)
		return nil
	}
	fm, body, bodyLine, err := content.SplitFrontMatter(raw)
	if err != nil {
		b.dc.Errorf(file, 1, 1, "frontmatter.yaml", "%v", err)
	}
	front, ds := content.ParseFrontMatter(file, fm)
	b.dc.Add(ds...)
	if front.Draft && !b.cfg.Build.Drafts {
		return nil
	}

	p := &content.Page{File: file, Hash: sha256.Sum256(raw)}
	p.SourcePath = rel
	p.Front = front
	p.Source = body
	p.BodyLine = bodyLine
	p.Data = map[string]any{}
	p.Diags = reporter{&b.dc, file}
	p.URL = content.URLFor(rel, front.Slug)
	p.OutPath = content.OutputPath(p.URL)
	return p
}

// parsePage parses the Markdown body, rewrites links and runs the
// PageParser plugins.
func (b *builder) parsePage(p *content.Page, links *linkResolver) {
	file, body := p.File, p.Source
	e := b.opts.Cache.get(p.SourcePath, p.Hash)
	if e == nil {
		doc, issues := b.md.Parse(body)
		e = &parsed{sum: p.Hash, doc: doc, issues: issues, h1: content.FirstH1(doc, body), links: collectLinks(doc)}
		b.opts.Cache.put(p.SourcePath, e)
	}
	p.AST = e.doc
	for _, is := range e.issues {
		line := 0
		if is.Offset >= 0 {
			line = p.LineAt(is.Offset)
		}
		b.dc.Add(diag.Diagnostic{Severity: is.Severity, File: file, Line: line, Code: is.Code, Message: is.Message})
	}
	p.HasH1 = e.h1 != ""
	p.Title = cmp.Or(p.Front.Title, e.h1, content.TitleFromPath(p.SourcePath), b.cfg.Site.Title, "Home")
	links.rewrite(p.SourcePath, e.links)

	for _, pp := range plugins.Each[plugin.PageParser](b.plugins) {
		if err := pp.OnPageParsed(&p.Page); err != nil {
			b.dc.Errorf(file, 0, 0, "plugin.error", "plugin %s: %v", pp.(plugin.Plugin).Name(), err)
		}
	}
}

// anchors collects the element IDs of a document: heading IDs and
// directive {#id}s.
func anchors(doc ast.Node) map[string]struct{} {
	ids := map[string]struct{}{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if v, ok := n.AttributeString("id"); ok {
				if b, ok := v.([]byte); ok {
					ids[string(b)] = struct{}{}
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return ids
}

func (b *builder) render(p *content.Page) ([]byte, bool) {
	body, err := b.md.Render(p.AST, p.Source)
	if err != nil {
		b.dc.Errorf(p.File, 0, 0, "markdown.render", "%v", err)
		return nil, false
	}

	layout := cmp.Or(p.Front.Layout, b.cfg.Defaults.Layout)
	if !b.tmpl.Has(layout) {
		b.dc.Errorf(p.File, 0, 0, "layout.missing", "page %s uses layout %q, but %s does not exist", p.File, layout,
			path.Join(b.rel(b.cfg.Dir(b.cfg.Build.LayoutsDir)), layout+".html"))
		return nil, false
	}
	toc, _ := p.Data["toc"].([]plugin.TOCEntry)
	data := &render.TemplateData{
		Site:        b.site,
		Page:        b.view.info[p.URL],
		Content:     template.HTML(body), // trusted: rendered by goldmark
		Nav:         nav.ForPage(b.view.nav, p.URL),
		TOC:         toc,
		Prev:        b.view.prev[p.URL],
		Next:        b.view.next[p.URL],
		Stylesheets: b.styles,
		Theme:       b.theme,
		Dev:         b.opts.Dev,
	}
	var buf bytes.Buffer
	if err := b.tmpl.Execute(&buf, layout, data); err != nil {
		b.dc.Errorf(p.File, 0, 0, "template.exec", "%v", err)
		return nil, false
	}
	p.HTML = buf.Bytes()

	for _, pr := range plugins.Each[plugin.PageRenderer](b.plugins) {
		if err := pr.OnPageRendered(&p.Page, b.view); err != nil {
			b.dc.Errorf(p.File, 0, 0, "plugin.error", "plugin %s: %v", pr.(plugin.Plugin).Name(), err)
		}
	}
	return p.HTML, true
}

// renderNotFound always emits 404.html.
func (b *builder) renderNotFound(s *Site) {
	data := &render.TemplateData{
		Site:        b.site,
		Page:        &plugin.PageInfo{Title: "Page not found", URL: "/" + NotFoundPath, TitleInBody: true},
		Nav:         s.Nav,
		Stylesheets: b.styles,
		Theme:       b.theme,
		Dev:         b.opts.Dev,
	}
	var buf bytes.Buffer
	if err := b.tmpl.Execute(&buf, "404", data); err != nil {
		if errors.Is(err, render.ErrMissingLayout) {
			// A broken 404 layout was already reported at parse time.
			buf.Reset()
			buf.WriteString("<!DOCTYPE html><title>Page not found</title><h1>Page not found</h1>\n")
		} else {
			b.dc.Errorf(b.rel(b.cfg.Dir(b.cfg.Build.LayoutsDir)), 0, 0, "template.exec", "404 page: %v", err)
			return
		}
	}
	s.Outputs[NotFoundPath] = buf.Bytes()
}

// themeData prepares the theme config for templates.
func (b *builder) themeData() render.ThemeData {
	t := b.cfg.Theme
	d := render.ThemeData{
		Accent:      template.CSS(t.Accent), // validated by config.Validate
		AccentDark:  template.CSS(t.AccentDark),
		Logo:        t.Logo,
		ColorScheme: t.ColorScheme,
		CSS:         t.CSS,
		Params:      t.Params,
	}
	if d.Accent != "" && d.AccentDark == "" {
		d.AccentDark = template.CSS("color-mix(in srgb, " + t.Accent + " 60%, white)") // readable on dark backgrounds
	}
	for _, l := range t.Nav {
		d.Nav = append(d.Nav, render.NavLink{Title: l.Title, URL: l.URL})
	}
	if t.Footer != "" {
		html, err := b.md.RenderInline(t.Footer)
		if err != nil {
			b.dc.Errorf(b.cfg.Path, 0, 0, "config.invalid", "theme.footer: %v", err)
		}
		d.Footer = template.HTML(html) // rendered by goldmark; raw HTML only with unsafeHTML
	}
	return d
}

// addBuiltinStatic adds the built-in theme's static files (css/tryck.css)
// unless the theme or the site has a file with the same path.
func (b *builder) addBuiltinStatic(s *Site, static map[string]bool) {
	_ = fs.WalkDir(render.Static(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || static[p] {
			return err
		}
		data, err := fs.ReadFile(render.Static(), p)
		if err == nil {
			s.Outputs[p] = data
		}
		return err
	})
}

// pageInfo copies the template- and plugin-visible fields of p.
func pageInfo(p *content.Page) *plugin.PageInfo {
	return &plugin.PageInfo{
		SourcePath:  p.SourcePath,
		Title:       p.Title,
		NavTitle:    p.Front.NavTitle,
		Description: p.Front.Description,
		URL:         p.URL,
		Date:        p.Front.Date,
		Draft:       p.Front.Draft,
		Weight:      p.Front.Weight,
		TitleInBody: p.HasH1,
		Aliases:     p.Front.Aliases,
		Params:      p.Front.Params,
		Data:        maps.Clone(p.Data),
	}
}

// outputs lets Finalizer plugins add files and site-wide diagnostics.
type outputs struct {
	files  map[string][]byte
	assets map[string]bool
	dc     *diag.Collector
}

func (o *outputs) Report(d plugin.Diagnostic) {
	o.dc.Add(diag.Diagnostic{Severity: diag.Severity(d.Severity), Line: d.Line, Col: d.Col, Code: d.Code, Message: d.Message})
}

func (o *outputs) Add(p string, data []byte) error {
	clean := path.Clean("/" + strings.ReplaceAll(p, "\\", "/"))[1:]
	if clean == "" || clean != strings.TrimPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return fmt.Errorf("invalid output path %q", p)
	}
	if _, ok := o.files[clean]; ok || o.assets[clean] {
		return fmt.Errorf("output path %q is already produced", clean)
	}
	o.files[clean] = data
	return nil
}
