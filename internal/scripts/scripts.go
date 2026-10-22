// Package scripts runs JavaScript plugins: plugins/<name>.js files that
// define the same hooks as Go plugins as plain functions.
//
// Data crosses between Go and JavaScript as JSON, so scripts only see
// copies. Page hooks run in a fresh runtime for every page, so no state
// carries over from one page to the next and parallel builds stay
// deterministic; what init returns is passed to every hook instead.
package scripts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Dir is a plugins directory and how it is named in diagnostics.
type Dir struct {
	Path  string
	Label string
}

var nameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// syntaxPosRE reads the position from goja's "file: Line 2:15 message".
var syntaxPosRE = regexp.MustCompile(`Line (\d+):(\d+) (.*)$`)

// Load compiles the .js files in dirs (lowest priority first: the
// theme's, then the site's; a later file replaces an earlier one with
// the same name) and returns them as plugins, sorted by name. Missing
// directories are skipped.
func Load(dirs []Dir) ([]plugin.Plugin, []diag.Diagnostic) {
	type file struct{ path, label string }
	files := map[string]file{}
	var dc diag.Collector
	for _, d := range dirs {
		entries, err := os.ReadDir(d.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			dc.Errorf(d.Label, 0, 0, "plugin.config", "cannot read plugins: %v", err)
			continue
		}
		for _, e := range entries {
			name, ok := strings.CutSuffix(e.Name(), ".js")
			if !ok || e.IsDir() {
				continue
			}
			label := path.Join(d.Label, e.Name())
			if !nameRE.MatchString(name) {
				dc.Errorf(label, 0, 0, "plugin.config", "%q is not a plugin name: use letters, digits, - and _, starting with a letter", name)
				continue
			}
			files[name] = file{filepath.Join(d.Path, e.Name()), label}
		}
	}

	var out []plugin.Plugin
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for n := range files {
			if !yield(n) {
				return
			}
		}
	}) {
		f := files[name]
		s, err := load(name, f.path, f.label)
		if err != nil {
			d := diag.Diagnostic{Severity: diag.Error, File: f.label, Code: "plugin.config", Message: err.Error()}
			if m := syntaxPosRE.FindStringSubmatch(err.Error()); m != nil {
				d.Line, _ = strconv.Atoi(m[1])
				d.Col, _ = strconv.Atoi(m[2])
				d.Message = "syntax error: " + m[3]
			}
			dc.Add(d)
			continue
		}
		if s.configKeys != nil {
			out = append(out, keyed{s})
		} else {
			out = append(out, s)
		}
	}
	return out, dc.All()
}

// script is one loaded plugin. Everything except state is fixed after
// load; state is set by Init, before any page hook runs.
type script struct {
	name, label string
	prog        *goja.Program
	hooks       map[string]bool // which hook functions the script defines
	priority    int
	configKeys  []string // nil: the script declares none
	stylesheets []string
	state       []byte // JSON of what init returned
}

// keyed is a script that declares configKeys, so unknown keys in its
// section warn.
type keyed struct{ *script }

func (k keyed) ConfigKeys() []string { return k.configKeys }

var hookNames = []string{"init", "onPageParsed", "onPageRendered", "onBuildComplete"}

func load(name, file, label string) (*script, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	prog, err := goja.Compile(label, string(src), true)
	if err != nil {
		return nil, err
	}
	s := &script{name: name, label: label, prog: prog, hooks: map[string]bool{}, priority: plugin.DefaultPriority}
	rt, err := s.runtime()
	if err != nil {
		return nil, err
	}
	for _, h := range hookNames {
		_, s.hooks[h] = goja.AssertFunction(rt.Get(h))
	}
	if v := rt.Get("priority"); v != nil && !goja.IsUndefined(v) {
		n, ok := v.Export().(int64)
		if !ok {
			return nil, fmt.Errorf("priority must be a whole number, got %v", v)
		}
		s.priority = int(n)
	}
	if s.configKeys, err = stringList(rt, "configKeys"); err != nil {
		return nil, err
	}
	if s.stylesheets, err = stringList(rt, "stylesheets"); err != nil {
		return nil, err
	}
	return s, nil
}

// stringList reads a top-level array of strings; nil if it is not set.
func stringList(rt *goja.Runtime, name string) ([]string, error) {
	v := rt.Get(name)
	if v == nil || goja.IsUndefined(v) {
		return nil, nil
	}
	var out []string
	if err := rt.ExportTo(v, &out); err != nil {
		return nil, fmt.Errorf("%s must be an array of strings", name)
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

// runtime returns a new runtime with the script loaded.
func (s *script) runtime() (*goja.Runtime, error) {
	rt := goja.New()
	if _, err := rt.RunProgram(s.prog); err != nil {
		return nil, err
	}
	return rt, nil
}

func (s *script) Name() string          { return s.name }
func (s *script) Priority() int         { return s.priority }
func (s *script) Stylesheets() []string { return s.stylesheets }

// Init runs the script's init(config, site) and keeps its result as the
// state passed to every other hook.
func (s *script) Init(cfg map[string]any, site plugin.SiteInfo) error {
	s.state = []byte("null")
	if !s.hooks["init"] {
		return nil
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	rt, err := s.runtime()
	if err != nil {
		return err
	}
	conf := map[string]any{}
	for k, v := range cfg {
		if k != "enabled" {
			conf[k] = v
		}
	}
	v, err := s.call(rt, "init", fromJSON(rt, conf), fromJSON(rt, siteJSON(site)))
	if err != nil {
		return err
	}
	s.state, err = toJSON(rt, v)
	return err
}

// pageObject is what page hooks receive as page.
func (s *script) pageObject(rt *goja.Runtime, p *plugin.Page) *goja.Object {
	obj := fromJSON(rt, map[string]any{
		"url":        p.URL,
		"sourcePath": p.SourcePath,
		"version":    p.Version,
		"title":      p.Front.Title,
		"front":      frontJSON(p.Front),
		"source":     string(p.Source),
		"headings":   headings(p),
		"data":       p.Data,
	}).ToObject(rt)
	// line counts within page.source; diagnostics use file lines.
	_ = obj.Set("report", func(severity, message string, line int) {
		if line > 0 {
			line += p.BodyLine - 1
		}
		p.Diags.Report(plugin.Diagnostic{Severity: parseSeverity(severity), Line: line, Code: s.name, Message: message})
	})
	return obj
}

// readData copies the page.data a hook left behind into p.Data: new and
// changed keys are set, removed keys deleted. Unchanged values keep their
// Go types, which layouts may depend on.
func (s *script) readData(rt *goja.Runtime, page *goja.Object, p *plugin.Page) error {
	b, err := toJSON(rt, page.Get("data"))
	if err != nil {
		return fmt.Errorf("page.data: %w", err)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(b, &data); err != nil {
		return errors.New("page.data must be an object")
	}
	for k := range p.Data {
		if _, ok := data[k]; !ok {
			delete(p.Data, k)
		}
	}
	for k, raw := range data {
		if old, err := json.Marshal(p.Data[k]); err == nil && string(old) == string(raw) {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		p.Data[k] = v
	}
	return nil
}

func (s *script) OnPageParsed(p *plugin.Page) error {
	if !s.hooks["onPageParsed"] {
		return nil
	}
	rt, err := s.runtime()
	if err != nil {
		return err
	}
	page := s.pageObject(rt, p)
	if _, err := s.call(rt, "onPageParsed", page, s.stateValue(rt)); err != nil {
		return err
	}
	return s.readData(rt, page, p)
}

func (s *script) OnPageRendered(p *plugin.Page, v plugin.SiteView) error {
	if !s.hooks["onPageRendered"] {
		return nil
	}
	rt, err := s.runtime()
	if err != nil {
		return err
	}
	page := s.pageObject(rt, p)
	_ = page.Set("html", string(p.HTML))
	site := fromJSON(rt, siteJSON(v.Config())).ToObject(rt)
	_ = site.Set("page", func(url string) goja.Value {
		if info, ok := v.PageByURL(url); ok {
			return fromJSON(rt, pageInfoJSON(info))
		}
		return goja.Null()
	})
	_ = site.Set("hasFile", v.HasFile)
	_ = site.Set("hasAnchor", v.HasAnchor)
	if _, err := s.call(rt, "onPageRendered", page, s.stateValue(rt), site); err != nil {
		return err
	}
	if html := page.Get("html"); html != nil && html.String() != string(p.HTML) {
		p.HTML = []byte(html.String())
	}
	return s.readData(rt, page, p)
}

func (s *script) OnBuildComplete(v plugin.SiteView, out plugin.Outputs) error {
	if !s.hooks["onBuildComplete"] {
		return nil
	}
	rt, err := s.runtime()
	if err != nil {
		return err
	}
	site := siteJSON(v.Config())
	var pages []map[string]any
	for _, info := range v.Pages() {
		pages = append(pages, pageInfoJSON(info))
	}
	site["pages"] = pages
	site["nav"] = navJSON(v.Nav())
	o := rt.NewObject()
	_ = o.Set("add", func(path, data string) error { return out.Add(path, []byte(data)) })
	_ = o.Set("report", func(severity, message string) {
		out.Report(plugin.Diagnostic{Severity: parseSeverity(severity), Code: s.name, Message: message})
	})
	_, err = s.call(rt, "onBuildComplete", fromJSON(rt, site), o, s.stateValue(rt))
	return err
}

func (s *script) stateValue(rt *goja.Runtime) goja.Value {
	v, err := parseJSON(rt, string(s.state))
	if err != nil {
		return goja.Null()
	}
	return v
}

// call runs a hook. Script errors carry their position in the script.
func (s *script) call(rt *goja.Runtime, hook string, args ...goja.Value) (goja.Value, error) {
	fn, _ := goja.AssertFunction(rt.Get(hook))
	v, err := fn(goja.Undefined(), args...)
	var ex *goja.Exception
	if errors.As(err, &ex) {
		msg := ex.Value().String()
		for _, f := range ex.Stack() {
			if p := f.Position(); p.Filename != "" {
				msg += fmt.Sprintf(" (%s:%d:%d)", p.Filename, p.Line, p.Column)
				break
			}
		}
		return nil, fmt.Errorf("%s: %s", hook, msg)
	}
	return v, err
}

func siteJSON(site plugin.SiteInfo) map[string]any {
	return map[string]any{"title": site.Title, "description": site.Description, "baseUrl": site.BaseURL, "language": site.Language, "version": site.Version, "dev": site.Dev}
}

// frontJSON is the front matter with the page's own keys at the top
// level, next to the known ones.
func frontJSON(f plugin.FrontMatter) map[string]any {
	m := map[string]any{}
	for k, v := range f.Params {
		m[k] = v
	}
	b, _ := json.Marshal(f)
	var known map[string]any
	_ = json.Unmarshal(b, &known)
	delete(known, "params")
	for k, v := range known {
		m[k] = v
	}
	return m
}

func pageInfoJSON(p *plugin.PageInfo) map[string]any {
	m := map[string]any{
		"url": p.URL, "sourcePath": p.SourcePath, "version": p.Version, "title": p.Title, "navTitle": p.NavTitle,
		"description": p.Description, "draft": p.Draft, "weight": p.Weight,
		"aliases": p.Aliases, "params": p.Params, "data": p.Data,
	}
	if !p.Date.IsZero() {
		m["date"] = p.Date
	}
	if !p.LastUpdated.IsZero() {
		m["lastUpdated"] = p.LastUpdated
	}
	return m
}

func navJSON(n *plugin.NavNode) map[string]any {
	if n == nil {
		return nil
	}
	children := []map[string]any{}
	for _, c := range n.Children {
		children = append(children, navJSON(c))
	}
	return map[string]any{"title": n.Title, "url": n.URL, "draft": n.Draft, "section": n.Section, "children": children}
}

// headings lists the page's headings in document order.
func headings(p *plugin.Page) []map[string]any {
	out := []map[string]any{}
	if p.AST == nil {
		return out
	}
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		id, _ := h.AttributeString("id")
		idStr := ""
		if b, ok := id.([]byte); ok {
			idStr = string(b)
		}
		out = append(out, map[string]any{"level": h.Level, "id": idStr, "text": content.PlainText(h, p.Source)})
		return ast.WalkSkipChildren, nil
	})
	return out
}

func parseSeverity(s string) plugin.Severity {
	switch s {
	case "error":
		return plugin.Error
	case "info":
		return plugin.Info
	default:
		return plugin.Warning
	}
}

// fromJSON copies a Go value into the runtime as plain JavaScript data.
func fromJSON(rt *goja.Runtime, v any) goja.Value {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("scripts: %v", err)) // only tryck's own data is passed here
	}
	out, err := parseJSON(rt, string(b))
	if err != nil {
		panic(fmt.Sprintf("scripts: %v", err))
	}
	return out
}

func parseJSON(rt *goja.Runtime, s string) (goja.Value, error) {
	parse, _ := goja.AssertFunction(rt.Get("JSON").ToObject(rt).Get("parse"))
	return parse(goja.Undefined(), rt.ToValue(s))
}

// toJSON copies a JavaScript value out as JSON. undefined becomes null.
func toJSON(rt *goja.Runtime, v goja.Value) ([]byte, error) {
	if v == nil || goja.IsUndefined(v) {
		return []byte("null"), nil
	}
	stringify, _ := goja.AssertFunction(rt.Get("JSON").ToObject(rt).Get("stringify"))
	out, err := stringify(goja.Undefined(), v)
	if err != nil {
		return nil, fmt.Errorf("not plain data: %v", err)
	}
	if goja.IsUndefined(out) {
		return []byte("null"), nil
	}
	return []byte(out.String()), nil
}
