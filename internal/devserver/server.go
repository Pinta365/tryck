// Package devserver implements tryck dev: it builds the site into
// memory, serves it, rebuilds on file changes and tells browsers to
// reload over a WebSocket.
//
// Each build produces an immutable build.Site. Requests read the current
// one from an atomic pointer and a single builder goroutine swaps it, so
// the request path takes no locks and never sees a half-built site.
package devserver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// WSPath is where browsers connect for reload and error messages.
const WSPath = "/__tryck/ws"

// DefaultDebounce is how long the builder waits for file events to settle.
const DefaultDebounce = 75 * time.Millisecond

// Options configure a Server.
type Options struct {
	ConfigPath     string           // config file to reload on change
	ConfigExplicit bool             // --config was given, so the file must exist
	Overrides      config.Overrides // applied after every config load
	Out            io.Writer        // diagnostics and status lines
	Logger         *slog.Logger     // debug logging; nil = silent
	Verbose        bool             // also print info diagnostics
	Debounce       time.Duration    // 0 = DefaultDebounce
	Plugins        []plugin.Plugin  // extra compiled-in plugins
}

// snapshot is what the request path reads: a built site and the paths
// needed to serve files that are not in memory.
type snapshot struct {
	site       *build.Site
	staticDirs []string          // highest priority first: the site's, then the theme's
	assets     map[string]string // output path -> source file
}

// Server is the dev server. Create it with New, call Start, and serve
// Handler.
type Server struct {
	opts    Options
	log     *slog.Logger
	current atomic.Pointer[snapshot]
	hub     *hub
	cfg     *config.Config // owned by the builder goroutine after Start
	cache   *build.Cache   // owned by the builder goroutine after Start
	ctx     context.Context
	watcher *watcher
	events  chan fsnotify.Event
	done    chan struct{}

	// builds counts finished builds, for tests.
	builds atomic.Int64
}

// New returns a server for cfg, which must already be loaded with
// opts.Overrides applied.
func New(cfg *config.Config, opts Options) *Server {
	if opts.Debounce <= 0 {
		opts.Debounce = DefaultDebounce
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{opts: opts, log: log, hub: newHub(), cfg: cfg, cache: build.NewCache(), done: make(chan struct{})}
}

// Start builds the site once, then watches for changes until ctx ends.
// The first build is served even if it has errors, with the error
// overlay shown.
func (s *Server) Start(ctx context.Context) error {
	s.ctx = ctx
	w, err := newWatcher()
	if err != nil {
		return fmt.Errorf("watch files: %w", err)
	}
	s.watcher = w
	if err := s.watchRoots(); err != nil {
		w.close()
		return err
	}

	site, err := s.build(ctx)
	if err != nil {
		w.close()
		return err
	}
	s.report(site, true)

	s.events = make(chan fsnotify.Event, 256)
	go s.forward(ctx)
	go s.builder(ctx)
	return nil
}

// Done is closed when the builder goroutine has stopped.
func (s *Server) Done() <-chan struct{} { return s.done }

func (s *Server) watchRoots() error {
	c := s.cfg
	dirs := []string{c.Dir(c.Build.ContentDir), c.Dir(c.Build.LayoutsDir), c.Dir(c.Build.StaticDir)}
	if t := c.ThemeDir(); t != "" {
		dirs = append(dirs, t)
	}
	for _, dir := range dirs {
		if err := s.watcher.addTree(dir); err != nil {
			return fmt.Errorf("watch %s: %w", dir, err)
		}
	}
	if s.opts.ConfigPath != "" {
		// Watch the directory: editors replace files on save.
		if err := s.watcher.add(filepath.Dir(s.opts.ConfigPath)); err != nil {
			return fmt.Errorf("watch config: %w", err)
		}
	}
	return nil
}

// forward moves watcher events to the builder and keeps new
// directories watched.
func (s *Server) forward(ctx context.Context) {
	defer s.watcher.close()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-s.watcher.fs.Events:
			if !ok {
				return
			}
			if ignored(ev.Name) || ev.Op == fsnotify.Chmod {
				continue
			}
			s.watcher.handleCreate(ev)
			select {
			case s.events <- ev:
			case <-ctx.Done():
				return
			}
		case err, ok := <-s.watcher.fs.Errors:
			if !ok {
				return
			}
			s.log.Debug("watch error", "err", err)
		}
	}
}

type change int

const (
	changeNone    change = iota
	changeStatic         // reload browsers, no rebuild
	changeContent        // rebuild
	changeConfig         // reload config, drop the cache, rebuild
)

func (s *Server) classify(name string) change {
	abs, err := filepath.Abs(name)
	if err != nil {
		return changeNone
	}
	if s.opts.ConfigPath != "" {
		if cfgAbs, err := filepath.Abs(s.opts.ConfigPath); err == nil && abs == cfgAbs {
			return changeConfig
		}
	}
	c := s.cfg
	under := func(dir string) bool {
		d, err := filepath.Abs(dir)
		return err == nil && (abs == d || strings.HasPrefix(abs, d+string(filepath.Separator)))
	}
	if t := c.ThemeDir(); t != "" && under(filepath.Join(t, config.ThemeFile)) {
		return changeConfig
	}
	for _, d := range append(c.LayoutDirs(), c.Dir(c.Build.ContentDir)) {
		if under(d) {
			return changeContent
		}
	}
	for _, d := range c.StaticDirs() {
		if under(d) {
			return changeStatic
		}
	}
	return changeNone
}

type result struct {
	site *build.Site
	err  error
}

// builder is the only goroutine that builds and swaps the current site.
// It debounces events and cancels a running build when new events
// arrive; the next build starts after the following debounce.
func (s *Server) builder(ctx context.Context) {
	defer close(s.done)
	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	var (
		cancel        context.CancelFunc // of the running build
		done          = make(chan result, 1)
		pending       change // strongest change since the last build started
		startWhenDone bool
		superseded    bool // the running build was cancelled
	)
	start := func() {
		if pending == changeConfig && !s.reloadConfig() {
			pending = changeNone
			return
		}
		if pending == changeStatic {
			pending = changeNone
			s.hub.broadcast(message{Type: "reload"})
			return
		}
		pending = changeNone
		superseded = false
		var bctx context.Context
		bctx, cancel = context.WithCancel(ctx)
		go func() {
			site, err := s.build(bctx)
			done <- result{site, err}
		}()
	}
	for {
		select {
		case <-ctx.Done():
			if cancel != nil {
				cancel()
				<-done
			}
			return
		case ev := <-s.events:
			c := s.classify(ev.Name)
			if c == changeNone {
				continue
			}
			s.log.Debug("change", "file", ev.Name, "op", ev.Op.String())
			pending = max(pending, c)
			if cancel != nil && c >= changeContent {
				cancel() // its result is dropped; a new build follows the debounce
				superseded = true
			}
			debounce.Reset(s.opts.Debounce)
		case <-debounce.C:
			if cancel != nil {
				startWhenDone = true
				continue
			}
			start()
		case r := <-done:
			cancel = nil
			if r.err == nil && !superseded {
				s.report(r.site, false)
			}
			if startWhenDone {
				startWhenDone = false
				start()
			}
		}
	}
}

// reloadConfig loads the config file again. On errors it keeps the old
// config, shows the overlay and returns false.
func (s *Server) reloadConfig() bool {
	cfg, ds := config.Load(s.opts.ConfigPath, s.opts.ConfigExplicit)
	if !diag.HasErrors(ds) {
		cfg.Apply(s.opts.Overrides)
		ds = append(ds, cfg.Validate()...)
	}
	diag.Sort(ds)
	s.print(ds)
	if diag.HasErrors(ds) {
		s.hub.broadcast(errorMessage(ds))
		return false
	}
	s.cfg = cfg
	s.cache = build.NewCache()
	if err := s.watchRoots(); err != nil {
		s.log.Debug("rewatch", "err", err)
	}
	fmt.Fprintln(s.opts.Out, "Config reloaded")
	return true
}

func (s *Server) build(ctx context.Context) (*build.Site, error) {
	site, err := build.Build(ctx, s.cfg, build.Options{
		Dev:     true,
		Logger:  s.log,
		Plugins: s.opts.Plugins,
		Cache:   s.cache,
	})
	if err == nil {
		s.builds.Add(1)
	}
	return site, err
}

// report prints a build's diagnostics and either swaps in the new site
// and reloads browsers, or keeps the last good site and shows the error
// overlay. The first build is always served.
func (s *Server) report(site *build.Site, first bool) {
	s.print(site.Diags)
	if site.HasErrors() {
		s.hub.broadcast(errorMessage(site.Diags))
		if !first {
			fmt.Fprintln(s.opts.Out, "Build failed; still serving the last good version")
			return
		}
	}
	snap := &snapshot{site: site, assets: map[string]string{}}
	dirs := site.Config.StaticDirs()
	for i := len(dirs) - 1; i >= 0; i-- {
		snap.staticDirs = append(snap.staticDirs, dirs[i])
	}
	for _, a := range site.Assets {
		snap.assets[a.OutPath] = a.Src
	}
	s.current.Store(snap)
	if !first {
		s.hub.broadcast(message{Type: "reload"})
		fmt.Fprintf(s.opts.Out, "Rebuilt %d pages\n", len(site.Pages))
	}
}

func (s *Server) print(ds []diag.Diagnostic) {
	for _, d := range ds {
		if d.Severity > diag.Info || s.opts.Verbose {
			fmt.Fprintln(s.opts.Out, diag.Format(d))
		}
	}
}

func errorMessage(ds []diag.Diagnostic) message {
	m := message{Type: "error"}
	for _, d := range ds {
		if d.Severity < diag.Error {
			continue
		}
		m.Diagnostics = append(m.Diagnostics, diagnosticJSON{
			File: d.File, Line: d.Line, Col: d.Col, Severity: d.Severity.String(),
			Code: d.Code, Message: d.Message, Text: diag.Format(d),
		})
	}
	return m
}

// Handler serves the current site.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == WSPath {
		s.hub.serveWS(s.ctx, w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap := s.current.Load()
	if snap == nil {
		http.Error(w, "building", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")

	p := path.Clean("/" + r.URL.Path)
	key := strings.TrimPrefix(p, "/")
	if strings.HasSuffix(r.URL.Path, "/") && key != "" {
		key += "/"
	}
	if key == "" || strings.HasSuffix(key, "/") {
		key += "index.html"
	}

	if data, ok := snap.site.Outputs[key]; ok {
		serveBytes(w, r, key, data, http.StatusOK)
		return
	}
	if _, ok := snap.site.Outputs[key+"/index.html"]; ok {
		http.Redirect(w, r, p+"/", http.StatusFound)
		return
	}
	if src, ok := snap.assets[key]; ok && serveFile(w, r, src) {
		return
	}
	for _, dir := range snap.staticDirs {
		if serveFile(w, r, filepath.Join(dir, filepath.FromSlash(key))) {
			return
		}
	}
	serveBytes(w, r, build.NotFoundPath, snap.site.Outputs[build.NotFoundPath], http.StatusNotFound)
}

func serveBytes(w http.ResponseWriter, r *http.Request, name string, data []byte, status int) {
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = http.DetectContentType(data)
	}
	w.Header().Set("Content-Type", ctype)
	if status != http.StatusOK {
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

// serveFile serves a regular file from disk, reporting false if there is
// none.
func serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := os.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	return true
}
