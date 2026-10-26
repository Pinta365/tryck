package devserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/fsnotify/fsnotify"

	"github.com/Pinta365/tryck/internal/config"
)

type env struct {
	t    *testing.T
	root string
	srv  *Server
	http *httptest.Server
	ws   *websocket.Conn
	ctx  context.Context
}

func (e *env) write(rel, body string) {
	e.t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) get(path string) (int, string) {
	e.t.Helper()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(e.http.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusFound {
		return resp.StatusCode, resp.Header.Get("Location")
	}
	return resp.StatusCode, string(b)
}

// next waits up to 1 s for a WebSocket message.
func (e *env) next() message {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(e.ctx, time.Second)
	defer cancel()
	_, b, err := e.ws.Read(ctx)
	if err != nil {
		e.t.Fatalf("no WebSocket message within 1s: %v", err)
	}
	var m message
	if err := json.Unmarshal(b, &m); err != nil {
		e.t.Fatal(err)
	}
	return m
}

func (e *env) expectReload() {
	e.t.Helper()
	if m := e.next(); m.Type != "reload" {
		e.t.Fatalf("got %s message %v, want reload", m.Type, m.Diagnostics)
	}
}

func newEnv(t *testing.T, files map[string]string) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	e := &env{t: t, root: t.TempDir(), ctx: ctx}
	for name, body := range files {
		e.write(name, body)
	}
	cfgPath := filepath.Join(e.root, config.DefaultPath)
	ov := config.Overrides{BaseURL: ptr("http://127.0.0.1:1/"), Drafts: ptr(true)}
	cfg, ds := config.Load(cfgPath, false)
	if len(ds) != 0 {
		t.Fatalf("config: %v", ds)
	}
	cfg.Apply(ov)
	e.srv = New(cfg, Options{ConfigPath: cfgPath, Overrides: ov, Debounce: 20 * time.Millisecond})
	if err := e.srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	e.http = httptest.NewServer(e.srv.Handler())
	ws, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(e.http.URL, "http")+WSPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	e.ws = ws
	t.Cleanup(func() {
		ws.CloseNow()
		cancel()
		<-e.srv.Done()
		e.http.Close()
	})
	return e
}

func ptr[T any](v T) *T { return &v }

func TestReloadOnChange(t *testing.T) {
	e := newEnv(t, map[string]string{
		"tryck.config.yaml": "site:\n  title: Dev Site\n",
		"content/index.md":  "# Hello\n",
		"static/style.css":  "body{}",
	})
	if code, body := e.get("/"); code != 200 || !strings.Contains(body, "Hello") || !strings.Contains(body, WSPath) {
		t.Fatalf("GET / = %d, missing content or reload script:\n%s", code, body)
	}

	e.write("content/index.md", "# Changed\n")
	e.expectReload()
	if _, body := e.get("/"); !strings.Contains(body, "Changed") {
		t.Errorf("new content not served:\n%s", body)
	}

	// New directories are watched as they appear.
	e.write("content/guide/new.md", "# New page\n")
	e.expectReload()
	if code, body := e.get("/guide/new/"); code != 200 || !strings.Contains(body, "New page") {
		t.Errorf("GET /guide/new/ = %d", code)
	}

	// A new or changed script plugin rebuilds, even when plugins/ did
	// not exist at start.
	e.write("plugins/mark.js", "function onPageRendered(page) { page.html = page.html.replace('Changed', 'Scripted'); }\n")
	e.expectReload()
	if _, body := e.get("/"); !strings.Contains(body, "Scripted") {
		t.Errorf("script plugin not applied:\n%s", body)
	}

	// Static files are served from disk and reload without a rebuild.
	builds := e.srv.builds.Load()
	e.write("static/style.css", "body{color:red}")
	e.expectReload()
	if _, body := e.get("/style.css"); body != "body{color:red}" {
		t.Errorf("static file = %q", body)
	}
	if e.srv.builds.Load() != builds {
		t.Error("a static change rebuilt the site")
	}

	// Editor temp files are ignored.
	e.write("content/.index.md.swp", "x")
	e.write("content/index.md~", "x")
	e.write("content/4913", "x")
	ctx, cancel := context.WithTimeout(e.ctx, 200*time.Millisecond)
	defer cancel()
	if _, _, err := e.ws.Read(ctx); err == nil {
		t.Error("editor temp files triggered a message")
	}
}

func TestErrorOverlayKeepsLastGoodSite(t *testing.T) {
	e := newEnv(t, map[string]string{"content/index.md": "# Good\n"})

	e.write("content/index.md", "---\ntitle: [\n---\n# Broken\n")
	m := e.next()
	if m.Type != "error" || len(m.Diagnostics) == 0 || m.Diagnostics[0].Code != "frontmatter.yaml" || m.Diagnostics[0].Line != 2 {
		t.Fatalf("got %+v, want frontmatter.yaml error on line 2", m)
	}
	if _, body := e.get("/"); !strings.Contains(body, "Good") {
		t.Errorf("last good site not served:\n%s", body)
	}

	// A client connecting while the site is broken gets the error at once.
	ws2, _, err := websocket.Dial(e.ctx, "ws"+strings.TrimPrefix(e.http.URL, "http")+WSPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws2.CloseNow()
	ctx, cancel := context.WithTimeout(e.ctx, time.Second)
	defer cancel()
	if _, b, err := ws2.Read(ctx); err != nil || !strings.Contains(string(b), `"type":"error"`) {
		t.Errorf("new client: %s, %v", b, err)
	}

	e.write("content/index.md", "# Fixed\n")
	e.expectReload()
	if _, body := e.get("/"); !strings.Contains(body, "Fixed") {
		t.Errorf("fixed content not served:\n%s", body)
	}
}

func TestConfigReload(t *testing.T) {
	e := newEnv(t, map[string]string{
		"tryck.config.yaml": "site:\n  title: First\n",
		"content/index.md":  "Body\n",
	})
	e.write("tryck.config.yaml", "site:\n  title: Second\n")
	e.expectReload()
	if _, body := e.get("/"); !strings.Contains(body, "<title>Second</title>") {
		t.Errorf("config change not applied:\n%s", body)
	}
	e.write("tryck.config.yaml", "site:\n  title: [\n")
	if m := e.next(); m.Type != "error" || m.Diagnostics[0].Code != "config.yaml" {
		t.Errorf("got %+v, want config.yaml error", m)
	}
	if _, body := e.get("/"); !strings.Contains(body, "<title>Second</title>") {
		t.Error("bad config replaced the site")
	}
}

func TestRapidChangesEndWithLatestContent(t *testing.T) {
	e := newEnv(t, map[string]string{"content/index.md": "# v0\n"})
	for i := 1; i <= 20; i++ {
		e.write("content/index.md", "# v"+string(rune('0'+i%10))+"\n")
		time.Sleep(5 * time.Millisecond)
	}
	e.write("content/index.md", "# final\n")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, body := e.get("/"); strings.Contains(body, "final") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("latest content never served")
}

func TestServing(t *testing.T) {
	e := newEnv(t, map[string]string{
		"content/docs/setup.md": "# Setup\n",
		"content/docs/img.png":  "PNG",
		"static/robots.txt":     "robots",
	})
	tests := []struct {
		path, want string
		code       int
	}{
		{"/docs/setup/", "Setup", 200},
		{"/docs/setup", "/docs/setup/", http.StatusFound},
		{"/docs/img.png", "PNG", 200},
		{"/robots.txt", "robots", 200},
		{"/missing/", "Page not found", 404},
		{"/../../etc/passwd", "Page not found", 404},
		{"/sitemap.xml", "<loc>http://127.0.0.1:1/docs/setup/</loc>", 200},
	}
	for _, tt := range tests {
		code, body := e.get(tt.path)
		if code != tt.code || !strings.Contains(body, tt.want) {
			t.Errorf("GET %s = %d %q, want %d containing %q", tt.path, code, body, tt.code, tt.want)
		}
	}
	resp, err := http.Post(e.http.URL+"/", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d", resp.StatusCode)
	}
}

func TestThemeFolder(t *testing.T) {
	e := newEnv(t, map[string]string{
		"tryck.config.yaml":                "theme:\n  path: themes/t\n",
		"themes/t/theme.yaml":              "footer: v1\n",
		"themes/t/static/css/t.css":        "a{}",
		"themes/t/layouts/partials/x.html": "",
		"content/index.md":                 "# Home\n",
	})
	if _, body := e.get("/"); !strings.Contains(body, `<footer class="site-footer">v1</footer>`) {
		t.Fatalf("theme footer missing:\n%s", body)
	}
	if code, body := e.get("/css/t.css"); code != 200 || body != "a{}" {
		t.Errorf("theme static = %d %q", code, body)
	}
	if code, _ := e.get("/css/tryck.css"); code != 200 {
		t.Errorf("built-in css = %d", code)
	}

	e.write("themes/t/theme.yaml", "footer: v2\n")
	e.expectReload()
	if _, body := e.get("/"); !strings.Contains(body, ">v2</footer>") {
		t.Error("theme.yaml change not applied")
	}

	e.write("themes/t/layouts/partials/footer.html", "<footer>custom</footer>")
	e.expectReload()
	if _, body := e.get("/"); !strings.Contains(body, "<footer>custom</footer>") {
		t.Error("theme layout change not applied")
	}
}

func TestRelevant(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	if err := os.WriteFile(file, []byte("# A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		ev   fsnotify.Event
		want bool
	}{
		{fsnotify.Event{Name: file, Op: fsnotify.Write}, true},
		{fsnotify.Event{Name: file, Op: fsnotify.Create}, true},
		{fsnotify.Event{Name: file, Op: fsnotify.Chmod}, false},
		// Windows reports the directory of a changed file as written.
		{fsnotify.Event{Name: dir, Op: fsnotify.Write}, false},
		{fsnotify.Event{Name: dir, Op: fsnotify.Create}, true},
		{fsnotify.Event{Name: filepath.Join(dir, "gone"), Op: fsnotify.Remove}, true},
		{fsnotify.Event{Name: filepath.Join(dir, "gone"), Op: fsnotify.Write}, true}, // vanished: keep
		{fsnotify.Event{Name: filepath.Join(dir, ".a.md.swp"), Op: fsnotify.Write}, false},
	}
	for _, tt := range tests {
		if got := relevant(tt.ev); got != tt.want {
			t.Errorf("relevant(%s %s) = %v, want %v", tt.ev.Op, filepath.Base(tt.ev.Name), got, tt.want)
		}
	}
}
