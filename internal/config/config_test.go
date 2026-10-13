package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/diag"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tryck.config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func ptr[T any](v T) *T { return &v }

func TestDefaultsAreValid(t *testing.T) {
	if ds := Defaults().Validate(); len(ds) != 0 {
		t.Fatalf("Defaults().Validate() = %v", ds)
	}
}

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tryck.config.yaml")

	cfg, ds := Load(path, false)
	if len(ds) != 0 {
		t.Fatalf("optional missing file: diagnostics %v", ds)
	}
	if cfg.Path != "" || cfg.Build.OutputDir != "./dist" {
		t.Errorf("optional missing file: got path %q outputDir %q", cfg.Path, cfg.Build.OutputDir)
	}
	if cfg.Root != filepath.Dir(path) {
		t.Errorf("optional missing file: root = %q, want the config's directory %q", cfg.Root, filepath.Dir(path))
	}

	_, ds = Load(path, true)
	if !diag.HasErrors(ds) || ds[0].Code != "config.read" {
		t.Fatalf("required missing file: diagnostics %v, want config.read error", ds)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	cfg, ds := Load(writeConfig(t, ""), true)
	if len(ds) != 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	if cfg.Defaults.Layout != "doc" {
		t.Errorf("layout = %q, want default", cfg.Defaults.Layout)
	}
}

func TestLoadMergesOverDefaults(t *testing.T) {
	path := writeConfig(t, `
site:
  title: "OffGrid Engine Docs"
  baseUrl: "https://user.github.io/repo"
build:
  outputDir: "./public"
  workers: 4
defaults:
  tocDepth: [1, 4]
plugins:
  readingTime:
    wordsPerMinute: 250
  custom:
    enabled: true
`)
	cfg, ds := Load(path, true)
	if len(ds) != 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"site.title", cfg.Site.Title, "OffGrid Engine Docs"},
		{"site.baseUrl gets trailing slash", cfg.Site.BaseURL, "https://user.github.io/repo/"},
		{"site.language keeps default", cfg.Site.Language, "en"},
		{"build.outputDir", cfg.Build.OutputDir, "./public"},
		{"build.contentDir keeps default", cfg.Build.ContentDir, "./content"},
		{"build.cleanOutput keeps default", cfg.Build.CleanOutput, true},
		{"build.workers", cfg.Build.Workers, 4},
		{"defaults.tocDepth", cfg.Defaults.TOCDepth, [2]int{1, 4}},
		{"defaults.layout keeps default", cfg.Defaults.Layout, "doc"},
		{"readingTime.wordsPerMinute", cfg.Plugins["readingTime"]["wordsPerMinute"], 250},
		{"readingTime.enabled keeps default", cfg.Plugins["readingTime"]["enabled"], true},
		{"sitemap keeps default", cfg.Plugins["sitemap"]["enabled"], true},
		{"custom plugin", cfg.Plugins["custom"]["enabled"], true},
		{"root is config dir", cfg.Root, filepath.Dir(path)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestLoadDoesNotShareDefaultPluginMaps(t *testing.T) {
	a, _ := Load(writeConfig(t, "plugins:\n  sitemap:\n    enabled: false\n"), true)
	b := Defaults()
	if a.Plugins["sitemap"]["enabled"] != false || b.Plugins["sitemap"]["enabled"] != true {
		t.Error("plugin defaults were mutated by a loaded config")
	}
}

func TestLoadUnknownKeys(t *testing.T) {
	path := writeConfig(t, `site:
  title: x
  tittle: typo
build:
  assetsDir: ./assets
extra: 1
plugins:
  anything:
    goes: here
`)
	_, ds := Load(path, true)
	want := []struct {
		line, col int
		key       string
	}{
		{3, 3, `"site.tittle"`},
		{5, 3, `"build.assetsDir"`},
		{6, 1, `"extra"`},
	}
	if len(ds) != len(want) {
		t.Fatalf("got %d diagnostics %v, want %d", len(ds), ds, len(want))
	}
	for i, w := range want {
		d := ds[i]
		if d.Severity != diag.Warning || d.Code != "config.unknownKey" || d.Line != w.line || d.Col != w.col || !strings.Contains(d.Message, w.key) {
			t.Errorf("diagnostic %d = %s, want warning at %d:%d about %s", i, diag.Format(d), w.line, w.col, w.key)
		}
		if d.File != filepath.ToSlash(path) {
			t.Errorf("diagnostic %d file = %q, want %q", i, d.File, path)
		}
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantLine int
	}{
		{"syntax error", "site:\n  title: x\nbuild: [\n", 3},
		{"type error", "site:\n  title: x\nbuild:\n  workers: many\n", 4},
		{"wrong tocDepth length", "defaults:\n  tocDepth: [1, 2, 3]\n", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := Load(writeConfig(t, tt.body), true)
			if !diag.HasErrors(ds) {
				t.Fatalf("no error diagnostics: %v", ds)
			}
			d := ds[0]
			if d.Code != "config.yaml" || d.Line != tt.wantLine {
				t.Errorf("got %s, want config.yaml error on line %d", diag.Format(d), tt.wantLine)
			}
		})
	}
}

func TestPrecedence(t *testing.T) {
	path := writeConfig(t, `
site:
  baseUrl: "https://file.example/"
build:
  outputDir: "./file-dist"
  drafts: false
`)
	tests := []struct {
		name        string
		ov          Overrides
		wantBase    string
		wantOut     string
		wantDrafts  bool
		wantWorkers int
		wantContent string
	}{
		{
			name:        "file over default",
			wantBase:    "https://file.example/",
			wantOut:     "./file-dist",
			wantContent: "./content",
		},
		{
			name:        "flags over file",
			ov:          Overrides{OutputDir: ptr("./flag-dist"), BaseURL: ptr("https://preview.example/sub"), Drafts: ptr(true), Workers: ptr(8)},
			wantBase:    "https://preview.example/sub/",
			wantOut:     "./flag-dist",
			wantDrafts:  true,
			wantWorkers: 8,
			wantContent: "./content",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ds := Load(path, true)
			if len(ds) != 0 {
				t.Fatalf("diagnostics %v", ds)
			}
			cfg.Apply(tt.ov)
			if cfg.Site.BaseURL != tt.wantBase || cfg.Build.OutputDir != tt.wantOut || cfg.Build.Drafts != tt.wantDrafts ||
				cfg.Build.Workers != tt.wantWorkers || cfg.Build.ContentDir != tt.wantContent {
				t.Errorf("got baseUrl=%q outputDir=%q drafts=%v workers=%d contentDir=%q",
					cfg.Site.BaseURL, cfg.Build.OutputDir, cfg.Build.Drafts, cfg.Build.Workers, cfg.Build.ContentDir)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string // substring; "" means valid
	}{
		{"defaults", func(*Config) {}, ""},
		{"absolute baseUrl", func(c *Config) { c.Site.BaseURL = "https://offgrid.dev/" }, ""},
		{"path baseUrl", func(c *Config) { c.Site.BaseURL = "/docs/" }, ""},
		{"relative baseUrl", func(c *Config) { c.Site.BaseURL = "docs/" }, "site.baseUrl"},
		{"ftp baseUrl", func(c *Config) { c.Site.BaseURL = "ftp://x/" }, "site.baseUrl"},
		{"empty baseUrl", func(c *Config) { c.Site.BaseURL = "" }, "site.baseUrl"},
		{"negative workers", func(c *Config) { c.Build.Workers = -1 }, "build.workers"},
		{"empty contentDir", func(c *Config) { c.Build.ContentDir = " " }, "build.contentDir"},
		{"empty layout", func(c *Config) { c.Defaults.Layout = "" }, "defaults.layout"},
		{"tocDepth reversed", func(c *Config) { c.Defaults.TOCDepth = [2]int{3, 2} }, "defaults.tocDepth"},
		{"tocDepth too deep", func(c *Config) { c.Defaults.TOCDepth = [2]int{2, 7} }, "defaults.tocDepth"},
		{"tocDepth zero", func(c *Config) { c.Defaults.TOCDepth = [2]int{0, 3} }, "defaults.tocDepth"},
		{"tocDepth single level", func(c *Config) { c.Defaults.TOCDepth = [2]int{2, 2} }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Defaults()
			tt.mutate(c)
			ds := c.Validate()
			if tt.wantErr == "" {
				if len(ds) != 0 {
					t.Errorf("unexpected diagnostics %v", ds)
				}
				return
			}
			if len(ds) != 1 || ds[0].Severity != diag.Error || ds[0].Code != "config.invalid" || !strings.Contains(ds[0].Message, tt.wantErr) {
				t.Errorf("got %v, want one config.invalid error mentioning %q", ds, tt.wantErr)
			}
		})
	}
}

func TestWorkerCount(t *testing.T) {
	c := Defaults()
	if got := c.WorkerCount(); got != runtime.NumCPU() {
		t.Errorf("workers 0: WorkerCount() = %d, want %d", got, runtime.NumCPU())
	}
	c.Build.Workers = 3
	if got := c.WorkerCount(); got != 3 {
		t.Errorf("workers 3: WorkerCount() = %d", got)
	}
}

func TestDir(t *testing.T) {
	c := Defaults()
	c.Root = "/site"
	if got := c.Dir("./content"); got != filepath.Join("/site", "content") {
		t.Errorf("Dir(relative) = %q", got)
	}
	abs := filepath.Join(t.TempDir(), "out")
	if got := c.Dir(abs); got != abs {
		t.Errorf("Dir(absolute) = %q", got)
	}
}

func TestThemeDefaults(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("themes/acme/theme.yaml", "accent: red\nlogo: /logo.svg\ncolorScheme: dark\nnav:\n  - title: A\n    url: /a/\n    target: x\nfooter: theme\n")
	write("tryck.config.yaml", "theme:\n  path: themes/acme\n  accent: \"#00ff00\"\n  footer: site\n")

	cfg, ds := Load(filepath.Join(root, "tryck.config.yaml"), true)
	if len(ds) != 1 || ds[0].Code != "config.unknownKey" || ds[0].File != "themes/acme/theme.yaml" || ds[0].Line != 7 || !strings.Contains(ds[0].Message, `"nav[0].target"`) {
		t.Errorf("diagnostics = %v, want one unknownKey for nav[0].target in theme.yaml:7", ds)
	}
	th := cfg.Theme
	if th.Accent != "#00ff00" || th.Footer != "site" || th.Logo != "/logo.svg" || th.ColorScheme != "dark" || len(th.Nav) != 1 || th.Path != "themes/acme" {
		t.Errorf("merged theme = %+v", th)
	}
	if got := cfg.LayoutDirs(); len(got) != 2 || got[0] != filepath.Join(root, "themes/acme/layouts") || got[1] != filepath.Join(root, "layouts") {
		t.Errorf("LayoutDirs = %v", got)
	}
	if got := cfg.StaticDirs(); len(got) != 2 || got[0] != filepath.Join(root, "themes/acme/static") {
		t.Errorf("StaticDirs = %v", got)
	}
	if len(cfg.Validate()) != 0 {
		t.Errorf("validate: %v", cfg.Validate())
	}

	write("tryck.config.yaml", "theme:\n  path: themes/missing\n")
	if _, ds := Load(filepath.Join(root, "tryck.config.yaml"), true); !diag.HasErrors(ds) || !strings.Contains(ds[0].Message, "not a directory") {
		t.Errorf("missing theme dir: %v", ds)
	}
	if got := Defaults().LayoutDirs(); len(got) != 1 {
		t.Errorf("no theme: LayoutDirs = %v", got)
	}
}

func TestThemeValidate(t *testing.T) {
	tests := []struct {
		mutate  func(*ThemeConfig)
		wantErr string
	}{
		{func(c *ThemeConfig) { c.Accent = "#0b7a5a" }, ""},
		{func(c *ThemeConfig) { c.Accent = "rebeccapurple" }, ""},
		{func(c *ThemeConfig) { c.Accent = "oklch(60% 0.15 160)" }, ""},
		{func(c *ThemeConfig) { c.Accent = "red; } body { display: none" }, "theme.accent"},
		{func(c *ThemeConfig) { c.Accent = "red</style>" }, "theme.accent"},
		{func(c *ThemeConfig) { c.ColorScheme = "sepia" }, "theme.colorScheme"},
		{func(c *ThemeConfig) { c.Nav = []NavLink{{Title: "x"}} }, "theme.nav[0]"},
	}
	for _, tt := range tests {
		c := Defaults()
		tt.mutate(&c.Theme)
		ds := c.Validate()
		if tt.wantErr == "" && len(ds) != 0 || tt.wantErr != "" && (len(ds) != 1 || !strings.Contains(ds[0].Message, tt.wantErr)) {
			t.Errorf("%+v: got %v, want %q", c.Theme, ds, tt.wantErr)
		}
	}
}
