// Package config loads, defaults and validates tryck.config.yaml.
//
// Precedence is CLI flag > config file > built-in default: Load merges the
// file over Defaults, then Apply layers CLI overrides on top.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Pinta365/tryck/internal/diag"
)

// DefaultPath is the config file looked up when --config is not given.
const DefaultPath = "tryck.config.yaml"

// Config is the fully loaded site configuration.
type Config struct {
	Site     SiteConfig     `yaml:"site"`
	Build    BuildConfig    `yaml:"build"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Theme    ThemeConfig    `yaml:"theme"`
	Versions VersionsConfig `yaml:"versions"`
	// Plugins maps a plugin's config name to its raw section. Each plugin
	// decodes its own section.
	Plugins map[string]map[string]any `yaml:"plugins"`

	// Path is the config file that was loaded, or "" if none was found.
	Path string `yaml:"-"`
	// Root is the project root. Relative build directories resolve
	// against it. It is the directory holding the config file.
	Root string `yaml:"-"`
}

type SiteConfig struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	BaseURL     string `yaml:"baseUrl"`
	Language    string `yaml:"language"`
}

type BuildConfig struct {
	ContentDir  string `yaml:"contentDir"`
	OutputDir   string `yaml:"outputDir"`
	StaticDir   string `yaml:"staticDir"`
	LayoutsDir  string `yaml:"layoutsDir"`
	PluginsDir  string `yaml:"pluginsDir"`
	CleanOutput bool   `yaml:"cleanOutput"`
	Drafts      bool   `yaml:"drafts"`
	UnsafeHTML  bool   `yaml:"unsafeHTML"`
	LastUpdated bool   `yaml:"lastUpdated"` // read each page's last commit date from git
	Workers     int    `yaml:"workers"`     // 0 = runtime.NumCPU()
}

type DefaultsConfig struct {
	Layout     string `yaml:"layout"`
	TOC        bool   `yaml:"toc"`
	TOCDepth   [2]int `yaml:"tocDepth"` // min and max heading level
	DateFormat string `yaml:"dateFormat"`
	// HeadingAnchors adds a "#" link to every heading below h1.
	HeadingAnchors bool `yaml:"headingAnchors"`
}

// ThemeConfig selects an optional theme folder and sets the options of
// the built-in theme. A theme folder's theme.yaml supplies defaults for
// every key except path; the site config wins.
type ThemeConfig struct {
	Path        string         `yaml:"path"`        // theme folder with layouts/, static/ and theme.yaml
	Accent      string         `yaml:"accent"`      // CSS color for links, active nav and tabs
	AccentDark  string         `yaml:"accentDark"`  // accent in dark mode; default: accent mixed with white
	Logo        string         `yaml:"logo"`        // site path of an image shown in the header
	ColorScheme string         `yaml:"colorScheme"` // auto (with a toggle), light or dark
	Nav         []NavLink      `yaml:"nav"`         // header links
	Footer      string         `yaml:"footer"`      // inline Markdown
	EditURL     string         `yaml:"editUrl"`     // prefix for "Edit this page" links; the file's path from the project root follows
	CSS         []string       `yaml:"css"`         // extra stylesheets, linked after the theme's
	Params      map[string]any `yaml:"params"`      // free-form values for custom themes
}

// VersionsConfig names the current version (contentDir) and lists older
// versions, newest first. Without older versions the site is unversioned.
type VersionsConfig struct {
	Current string          `yaml:"current"`
	Older   []VersionConfig `yaml:"older"`
}

// VersionConfig is one older version: a snapshot of the content served
// under /<name>/.
type VersionConfig struct {
	Name string `yaml:"name"`
	Dir  string `yaml:"dir"` // default: versions/<name>
}

// Version is a content tree of the site and where it is served.
type Version struct {
	Name    string // "" for an unversioned site
	Dir     string // the content directory
	Prefix  string // URL prefix: "/" for the current version, "/<name>/" for older ones
	Current bool
}

// ContentVersions lists the site's content trees, the current one first.
// An unversioned site has one, with an empty name.
func (c *Config) ContentVersions() []Version {
	vs := []Version{{Name: c.Versions.Current, Dir: c.Dir(c.Build.ContentDir), Prefix: "/", Current: true}}
	for _, v := range c.Versions.Older {
		dir := v.Dir
		if dir == "" {
			dir = filepath.Join("versions", v.Name)
		}
		vs = append(vs, Version{Name: v.Name, Dir: c.Dir(dir), Prefix: "/" + v.Name + "/"})
	}
	return vs
}

// versionNameRE matches names usable as one URL segment.
var versionNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidVersionName reports whether name can name a version: one URL
// segment of letters, digits, '.', '-' and '_'.
func ValidVersionName(name string) bool { return versionNameRE.MatchString(name) }

// NavLink is a header link.
type NavLink struct {
	Title string `yaml:"title"`
	URL   string `yaml:"url"`
}

// Defaults returns the built-in configuration.
func Defaults() *Config {
	return &Config{
		Site: SiteConfig{
			BaseURL:  "/",
			Language: "en",
		},
		Build: BuildConfig{
			ContentDir:  "./content",
			OutputDir:   "./dist",
			StaticDir:   "./static",
			LayoutsDir:  "./layouts",
			PluginsDir:  "./plugins",
			CleanOutput: true,
		},
		Defaults: DefaultsConfig{
			Layout:         "doc",
			TOC:            true,
			TOCDepth:       [2]int{2, 3},
			DateFormat:     "2006-01-02",
			HeadingAnchors: true,
		},
		Theme: ThemeConfig{ColorScheme: "auto"},
		Plugins: map[string]map[string]any{
			"readingTime":        {"enabled": true, "wordsPerMinute": 200},
			"syntaxHighlighting": {"enabled": true, "theme": "github", "darkTheme": "github-dark", "lineNumbers": true},
			"linkCheck":          {"enabled": true, "failOnBroken": true},
			"sitemap":            {"enabled": true},
			"search":             {"enabled": true},
			"mermaid":            {"enabled": true},
			"feed":               {"enabled": true},
		},
		Root: ".",
	}
}

// Load reads the config file at path and merges it over Defaults.
//
// A missing file is fine unless required is set (the user passed
// --config explicitly). The returned Config is always usable; callers
// must check the diagnostics for errors.
func Load(path string, required bool) (*Config, []diag.Diagnostic) {
	cfg := Defaults()
	cfg.Root = filepath.Dir(path) // the project root, even if the file does not exist yet
	file := filepath.ToSlash(filepath.Clean(path))

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && !required {
			return cfg, nil
		}
		return cfg, []diag.Diagnostic{{
			Severity: diag.Error,
			File:     file,
			Code:     "config.read",
			Message:  readErrMessage(err),
		}}
	}
	cfg.Path = file

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return cfg, []diag.Diagnostic{yamlDiag(file, err.Error())}
	}
	if doc.Kind == 0 { // empty file
		return cfg, nil
	}

	var dc diag.Collector
	checkKeys(&dc, file, &doc, reflect.TypeOf(Config{}), "")
	if dc.HasErrors() {
		return cfg, dc.All()
	}

	defPlugins := cfg.Plugins
	cfg.Plugins = nil
	if err := doc.Decode(cfg); err != nil {
		var te *yaml.TypeError
		if errors.As(err, &te) {
			for _, msg := range te.Errors {
				dc.Add(yamlDiag(file, msg))
			}
		} else {
			dc.Add(yamlDiag(file, err.Error()))
		}
	}
	cfg.Plugins = mergePlugins(defPlugins, cfg.Plugins)
	if cfg.Theme.Path != "" && !dc.HasErrors() {
		dc.Add(cfg.applyThemeDefaults(&doc)...)
	}
	cfg.normalize()
	return cfg, dc.All()
}

// ThemeFile is a theme folder's settings file.
const ThemeFile = "theme.yaml"

// applyThemeDefaults reads the theme folder's theme.yaml and lays the
// site's theme section over it again, so site values win key by key.
// css is the exception: the site's stylesheets are added after the
// theme's, so a site can add one without dropping the theme's.
func (c *Config) applyThemeDefaults(doc *yaml.Node) []diag.Diagnostic {
	dir := c.ThemeDir()
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return []diag.Diagnostic{{Severity: diag.Error, File: c.Path, Code: "config.invalid",
			Message: fmt.Sprintf("theme.path %q is not a directory", c.Theme.Path)}}
	}
	data, err := os.ReadFile(filepath.Join(dir, ThemeFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	file := filepath.ToSlash(filepath.Join(filepath.Clean(c.Theme.Path), ThemeFile))
	if err != nil {
		return []diag.Diagnostic{{Severity: diag.Error, File: file, Code: "config.read", Message: readErrMessage(err)}}
	}
	var tdoc yaml.Node
	if err := yaml.Unmarshal(data, &tdoc); err != nil {
		return []diag.Diagnostic{yamlDiag(file, err.Error())}
	}
	if tdoc.Kind == 0 {
		return nil
	}
	var dc diag.Collector
	checkKeys(&dc, file, &tdoc, reflect.TypeOf(ThemeConfig{}), "")
	base := ThemeConfig{ColorScheme: "auto"}
	if err := tdoc.Decode(&base); err != nil {
		dc.Add(yamlDiag(file, err.Error()))
		return dc.All()
	}
	base.Path = c.Theme.Path
	themeCSS := base.CSS
	if site := mapValue(doc, "theme"); site != nil {
		_ = site.Decode(&base) // already decoded once without errors
	}
	var css []string
	for _, s := range slices.Concat(themeCSS, base.CSS) {
		if !slices.Contains(css, s) {
			css = append(css, s)
		}
	}
	base.CSS = css
	c.Theme = base
	return dc.All()
}

// mapValue returns the value node of key in a document's top mapping.
func mapValue(doc *yaml.Node, key string) *yaml.Node {
	n := doc
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// LayoutDirs lists the layout directories, lowest priority first: the
// theme's layouts/, then the site's layoutsDir.
func (c *Config) LayoutDirs() []string {
	return c.layered("layouts", c.Build.LayoutsDir)
}

// StaticDirs lists the static directories, lowest priority first: the
// theme's static/, then the site's staticDir.
func (c *Config) StaticDirs() []string {
	return c.layered("static", c.Build.StaticDir)
}

// PluginDirs lists the script plugin directories, lowest priority first:
// the theme's plugins/, then the site's pluginsDir.
func (c *Config) PluginDirs() []string {
	return c.layered("plugins", c.Build.PluginsDir)
}

func (c *Config) layered(themeSub, siteDir string) []string {
	var dirs []string
	if t := c.ThemeDir(); t != "" {
		dirs = append(dirs, filepath.Join(t, themeSub))
	}
	return append(dirs, c.Dir(siteDir))
}

// ThemeDir is the theme folder, or "" when no theme is set.
func (c *Config) ThemeDir() string {
	if c.Theme.Path == "" {
		return ""
	}
	return c.Dir(c.Theme.Path)
}

func readErrMessage(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		if errors.Is(err, fs.ErrNotExist) {
			return "config file not found"
		}
		return "cannot read config file: " + pe.Err.Error()
	}
	return "cannot read config file: " + err.Error()
}

// cssColorRE accepts hex colors, names and functions like rgb() or
// oklch(), and nothing that could end the declaration it is put in.
var cssColorRE = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|[a-zA-Z]+|[a-z-]+\([0-9a-zA-Z.,%/ +-]*\))$`)

var yamlLineRE = regexp.MustCompile(`^(?:yaml: )?line (\d+): `)

// yamlDiag turns a yaml.v3 error message into a config.yaml diagnostic,
// lifting the "line N:" prefix into the Line field.
func yamlDiag(file, msg string) diag.Diagnostic {
	d := diag.Diagnostic{Severity: diag.Error, File: file, Code: "config.yaml"}
	if m := yamlLineRE.FindStringSubmatch(msg); m != nil {
		d.Line, _ = strconv.Atoi(m[1])
		msg = msg[len(m[0]):]
	}
	d.Message = "invalid YAML: " + strings.TrimPrefix(msg, "yaml: ")
	return d
}

// checkKeys walks n against the yaml-tagged fields of t and warns about
// every mapping key that has no matching field. Map-typed fields (the
// plugins section) are free-form and not checked. It also reports
// fixed-length lists of the wrong length, which yaml.v3 rejects without
// a line number.
func checkKeys(dc *diag.Collector, file string, n *yaml.Node, t reflect.Type, prefix string) {
	if n.Kind == yaml.DocumentNode {
		for _, c := range n.Content {
			checkKeys(dc, file, c, t, prefix)
		}
		return
	}
	if t.Kind() == reflect.Array && n.Kind == yaml.SequenceNode && len(n.Content) != t.Len() {
		dc.Errorf(file, n.Line, n.Column, "config.yaml", "invalid YAML: %s must be a list of %d values, got %d",
			strings.TrimSuffix(prefix, "."), t.Len(), len(n.Content))
		return
	}
	if t.Kind() == reflect.Slice && n.Kind == yaml.SequenceNode {
		for i, c := range n.Content {
			checkKeys(dc, file, c, t.Elem(), fmt.Sprintf("%s[%d].", strings.TrimSuffix(prefix, "."), i))
		}
		return
	}
	if n.Kind != yaml.MappingNode || t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		f, ok := fieldByTag(t, key.Value)
		if !ok {
			dc.Warnf(file, key.Line, key.Column, "config.unknownKey", "unknown config key %q", prefix+key.Value)
			continue
		}
		checkKeys(dc, file, val, f.Type, prefix+key.Value+".")
	}
}

func fieldByTag(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := range t.NumField() {
		f := t.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if tag == name && tag != "-" {
			return f, true
		}
	}
	return reflect.StructField{}, false
}

// mergePlugins overlays user plugin sections on the defaults key by key,
// so setting one option does not drop the defaults of the others.
func mergePlugins(def, user map[string]map[string]any) map[string]map[string]any {
	out := make(map[string]map[string]any, len(def)+len(user))
	for name, sec := range def {
		out[name] = maps.Clone(sec)
	}
	for name, sec := range user {
		if out[name] == nil {
			out[name] = map[string]any{}
		}
		maps.Copy(out[name], sec)
	}
	return out
}

// Overrides holds values from CLI flags. Nil fields are not set.
type Overrides struct {
	OutputDir *string
	BaseURL   *string
	Drafts    *bool
	Workers   *int
}

// Apply layers CLI overrides on top of the loaded config.
func (c *Config) Apply(o Overrides) {
	if o.OutputDir != nil {
		c.Build.OutputDir = *o.OutputDir
	}
	if o.BaseURL != nil {
		c.Site.BaseURL = *o.BaseURL
	}
	if o.Drafts != nil {
		c.Build.Drafts = *o.Drafts
	}
	if o.Workers != nil {
		c.Build.Workers = *o.Workers
	}
	c.normalize()
}

// normalize ensures baseUrl ends in a slash so path joins are uniform.
func (c *Config) normalize() {
	if c.Site.BaseURL != "" && !strings.HasSuffix(c.Site.BaseURL, "/") {
		c.Site.BaseURL += "/"
	}
}

// Validate checks values that decode fine but make no sense.
func (c *Config) Validate() []diag.Diagnostic {
	var dc diag.Collector
	bad := func(format string, args ...any) {
		dc.Errorf(c.Path, 0, 0, "config.invalid", format, args...)
	}

	if u, err := url.Parse(c.Site.BaseURL); err != nil || c.Site.BaseURL == "" {
		bad("site.baseUrl %q is not a valid URL", c.Site.BaseURL)
	} else if u.Scheme != "" {
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			bad("site.baseUrl %q must be an http(s) URL or start with /", c.Site.BaseURL)
		}
	} else if !strings.HasPrefix(c.Site.BaseURL, "/") {
		bad("site.baseUrl %q must be an http(s) URL or start with /", c.Site.BaseURL)
	}

	for name, dir := range map[string]string{
		"build.contentDir": c.Build.ContentDir,
		"build.outputDir":  c.Build.OutputDir,
		"build.staticDir":  c.Build.StaticDir,
		"build.layoutsDir": c.Build.LayoutsDir,
		"build.pluginsDir": c.Build.PluginsDir,
	} {
		if strings.TrimSpace(dir) == "" {
			bad("%s must not be empty", name)
		}
	}
	if c.Build.Workers < 0 {
		bad("build.workers must be 0 (all CPUs) or positive, got %d", c.Build.Workers)
	}
	if c.Defaults.Layout == "" {
		bad("defaults.layout must not be empty")
	}
	switch c.Theme.ColorScheme {
	case "auto", "light", "dark":
	default:
		bad("theme.colorScheme must be auto, light or dark, got %q", c.Theme.ColorScheme)
	}
	for name, v := range map[string]string{"theme.accent": c.Theme.Accent, "theme.accentDark": c.Theme.AccentDark} {
		if v != "" && !cssColorRE.MatchString(v) {
			bad("%s %q is not a CSS color", name, v)
		}
	}
	if u, err := url.Parse(c.Theme.EditURL); c.Theme.EditURL != "" && (err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "") {
		bad("theme.editUrl %q must be an http(s) URL", c.Theme.EditURL)
	}
	for i, l := range c.Theme.Nav {
		if l.Title == "" || l.URL == "" {
			bad("theme.nav[%d] needs both title and url", i)
		}
	}
	if v := c.Versions; v.Current != "" || len(v.Older) > 0 {
		seen := map[string]bool{}
		check := func(key, name string) {
			switch {
			case name == "":
				bad("%s must not be empty", key)
			case !versionNameRE.MatchString(name):
				bad("%s %q must be one URL segment: letters, digits, '.', '-' and '_'", key, name)
			case seen[name]:
				bad("%s %q is used twice", key, name)
			}
			seen[name] = true
		}
		check("versions.current", v.Current)
		for i, o := range v.Older {
			check(fmt.Sprintf("versions.older[%d].name", i), o.Name)
		}
	}
	if lo, hi := c.Defaults.TOCDepth[0], c.Defaults.TOCDepth[1]; lo < 1 || hi > 6 || lo > hi {
		bad("defaults.tocDepth must be [min, max] with 1 <= min <= max <= 6, got [%d, %d]", lo, hi)
	}
	return dc.All()
}

// WorkerCount resolves build.workers, where 0 means one per CPU.
func (c *Config) WorkerCount() int {
	if c.Build.Workers > 0 {
		return c.Build.Workers
	}
	return runtime.NumCPU()
}

// Dir resolves a configured directory against the project root.
func (c *Config) Dir(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.Root, p)
}
