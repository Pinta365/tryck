// Package build turns a site's inputs into an immutable Site snapshot.
// Writing it to disk (build) or serving it from memory (dev) happens
// afterwards, so both share this code path.
package build

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/markdown"
	"github.com/Pinta365/tryck/internal/nav"
	"github.com/Pinta365/tryck/internal/render"
	"github.com/Pinta365/tryck/internal/workpool"
)

// NotFoundPath is the output path of the 404 page, served by both
// Cloudflare Pages and GitHub Pages from the output root.
const NotFoundPath = "404.html"

// Site is the result of a build. It is not modified after Build returns.
type Site struct {
	Config  *config.Config
	Pages   []*content.Page // sorted by URL
	ByURL   map[string]*content.Page
	Nav     *nav.Node         // read-only after stage 3
	Assets  []content.Asset   // files from contentDir copied next to pages
	Outputs map[string][]byte // output path -> bytes (pages + generated files)
	Diags   []diag.Diagnostic // sorted
	BuiltAt time.Time
}

// HasErrors reports whether the build produced error-level diagnostics.
func (s *Site) HasErrors() bool { return diag.HasErrors(s.Diags) }

// Options adjust a build.
type Options struct {
	Dev    bool         // set TemplateData.Dev
	Logger *slog.Logger // debug logs with per-stage timings; nil = silent
}

type builder struct {
	cfg   *config.Config
	opts  Options
	dc    diag.Collector
	md    *markdown.Processor
	tmpl  *render.Templates
	site  render.SiteInfo
	pages []*content.Page
	// prev and next page of each URL in nav order
	prev, next map[string]*content.Page
}

// Build runs the pipeline. It always returns a Site; check HasErrors.
// It returns ctx.Err() if the build was cancelled.
func Build(ctx context.Context, cfg *config.Config, opts Options) (*Site, error) {
	b := &builder{
		cfg:  cfg,
		opts: opts,
		md:   markdown.New(markdown.Options{UnsafeHTML: cfg.Build.UnsafeHTML}),
		site: render.SiteInfo{
			Title:       cfg.Site.Title,
			Description: cfg.Site.Description,
			BaseURL:     cfg.Site.BaseURL,
			Language:    cfg.Site.Language,
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
	funcs := render.Funcs(render.FuncOptions{
		BaseURL:     cfg.Site.BaseURL,
		DateFormat:  cfg.Defaults.DateFormat,
		Markdownify: b.md.RenderInline,
	})
	tmpl, ds := render.Load(cfg.Dir(cfg.Build.LayoutsDir), b.rel(cfg.Dir(cfg.Build.LayoutsDir)), funcs)
	b.dc.Add(ds...)
	b.tmpl = tmpl

	contentDir := cfg.Dir(cfg.Build.ContentDir)
	src, err := content.Discover(contentDir)
	if err != nil {
		b.dc.Errorf(b.rel(contentDir), 0, 0, "content.read", "cannot read content: %v", err)
	}
	s.Assets = src.Assets
	stage("load", "pages", len(src.Pages), "assets", len(src.Assets))

	// [2] Parse, in parallel. A page only touches its own Page.
	workers := cfg.WorkerCount()
	parsed := make([]*content.Page, len(src.Pages))
	err = workpool.Run(ctx, len(src.Pages), workers, func(i int) {
		parsed[i] = b.parse(contentDir, src.Pages[i])
	})
	if err != nil {
		return nil, err
	}
	for _, p := range parsed {
		if p != nil {
			b.pages = append(b.pages, p)
		}
	}
	stage("parse", "workers", workers)

	// [3] Index, sequential. Pages, ByURL, Nav, prev and next are
	// read-only from here on.
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
	b.prev, b.next = map[string]*content.Page{}, map[string]*content.Page{}
	order := nav.Walk(s.Nav)
	for i := 1; i < len(order); i++ {
		b.prev[order[i]] = s.ByURL[order[i-1]]
		b.next[order[i-1]] = s.ByURL[order[i]]
	}
	stage("index")

	// [4] Render, in parallel.
	rendered := make([][]byte, len(s.Pages))
	err = workpool.Run(ctx, len(s.Pages), workers, func(i int) {
		if out, ok := b.render(s, s.Pages[i]); ok {
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

	// [5] Finalize: assets must not overwrite generated pages.
	for _, a := range s.Assets {
		if _, ok := s.Outputs[a.OutPath]; ok {
			b.dc.Errorf(b.rel(a.Src), 0, 0, "url.conflict", "%s would overwrite generated %s", b.rel(a.Src), a.OutPath)
		}
	}

	stage("finalize")

	s.Diags = b.dc.All()
	s.BuiltAt = time.Now()
	return s, nil
}

// rel makes a path relative to the project root for diagnostics.
func (b *builder) rel(p string) string {
	if r, err := filepath.Rel(b.cfg.Root, p); err == nil && !strings.HasPrefix(r, "..") {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(p)
}

// parse reads one Markdown file. It returns nil for drafts that are
// excluded and for files that cannot be read.
func (b *builder) parse(contentDir, rel string) *content.Page {
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

	p := &content.Page{
		SourcePath: rel,
		File:       file,
		Front:      front,
		Source:     body,
		BodyLine:   bodyLine,
	}
	p.URL = content.URLFor(rel, front.Slug)
	p.OutPath = content.OutputPath(p.URL)
	var issues []markdown.Issue
	p.AST, issues = b.md.Parse(body)
	for _, is := range issues {
		line := 0
		if is.Offset >= 0 {
			line = p.LineAt(is.Offset)
		}
		b.dc.Add(diag.Diagnostic{Severity: is.Severity, File: file, Line: line, Code: is.Code, Message: is.Message})
	}
	h1 := content.FirstH1(p.AST, body)
	p.HasH1 = h1 != ""
	p.Title = cmp.Or(front.Title, h1, content.TitleFromPath(rel), b.cfg.Site.Title, "Home")
	return p
}

func (b *builder) render(s *Site, p *content.Page) ([]byte, bool) {
	html, err := b.md.Render(p.AST, p.Source)
	if err != nil {
		b.dc.Errorf(p.File, 0, 0, "markdown.render", "%v", err)
		return nil, false
	}
	p.HTML = html

	layout := cmp.Or(p.Front.Layout, b.cfg.Defaults.Layout)
	if !b.tmpl.Has(layout) {
		b.dc.Errorf(p.File, 0, 0, "layout.missing", "page %s uses layout %q, but %s does not exist", p.File, layout,
			path.Join(b.rel(b.cfg.Dir(b.cfg.Build.LayoutsDir)), layout+".html"))
		return nil, false
	}
	data := &render.TemplateData{
		Site:    b.site,
		Page:    pageInfo(p),
		Content: template.HTML(html), // trusted: rendered by goldmark (spec §9.4)
		Nav:     nav.ForPage(s.Nav, p.URL),
		Prev:    pageInfo(b.prev[p.URL]),
		Next:    pageInfo(b.next[p.URL]),
		Dev:     b.opts.Dev,
	}
	var buf bytes.Buffer
	if err := b.tmpl.Execute(&buf, layout, data); err != nil {
		b.dc.Errorf(p.File, 0, 0, "template.exec", "%v", err)
		return nil, false
	}
	return buf.Bytes(), true
}

// renderNotFound always emits 404.html (spec §13).
func (b *builder) renderNotFound(s *Site) {
	data := &render.TemplateData{
		Site: b.site,
		Page: &render.PageInfo{Title: "Page not found", URL: "/" + NotFoundPath, TitleInBody: true},
		Nav:  s.Nav,
		Dev:  b.opts.Dev,
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

// pageInfo copies the template-visible fields of p. It returns nil for nil.
func pageInfo(p *content.Page) *render.PageInfo {
	if p == nil {
		return nil
	}
	return &render.PageInfo{
		Title:       p.Title,
		NavTitle:    p.Front.NavTitle,
		Description: p.Front.Description,
		URL:         p.URL,
		Date:        p.Front.Date,
		Draft:       p.Front.Draft,
		Weight:      p.Front.Weight,
		TitleInBody: p.HasH1,
		Params:      p.Front.Params,
	}
}
