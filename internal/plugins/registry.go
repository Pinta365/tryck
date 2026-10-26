// Package plugins holds tryck's built-in plugins and the registry that
// enables, configures and orders them for a build.
package plugins

import (
	"cmp"
	"fmt"
	"slices"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/markdown"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// builtins returns fresh instances of every built-in plugin. Custom
// binaries add their own plugins with tryck.Main instead.
func builtins(cfg *config.Config) []plugin.Plugin {
	return []plugin.Plugin{
		&TOC{Default: cfg.Defaults.TOC, MinLevel: cfg.Defaults.TOCDepth[0], MaxLevel: cfg.Defaults.TOCDepth[1]},
		&ReadingTime{},
		&Highlight{},
		&LinkCheck{},
		&Sitemap{},
		&Search{},
		&Redirects{},
		&Mermaid{},
		&Math{},
		&Social{StaticDirs: cfg.StaticDirs()},
		&Feed{},
		&LLMsTxt{UnsafeHTML: cfg.Build.UnsafeHTML},
	}
}

// Set is the enabled, initialized plugins of one build, in phase order.
type Set struct {
	plugins []plugin.Plugin
}

// Load enables the built-ins plus extra, initializes them and orders
// them by Priority, then Name. A plugin is enabled unless its section in
// cfg.Plugins sets enabled: false. Two plugins with the same name are an
// error and the later one is skipped.
func Load(cfg *config.Config, site plugin.SiteInfo, extra ...plugin.Plugin) (*Set, []diag.Diagnostic) {
	var dc diag.Collector
	all := append(builtins(cfg), extra...)
	known := map[string]bool{}
	set := &Set{}
	for _, p := range all {
		name := p.Name()
		if known[name] {
			dc.Errorf(cfg.Path, 0, 0, "plugin.config", "plugin %s: duplicate plugin name", name)
			continue
		}
		known[name] = true
		section := cfg.Plugins[name]
		if v, ok := section["enabled"]; ok {
			enabled, isBool := v.(bool)
			if !isBool {
				dc.Errorf(cfg.Path, 0, 0, "config.invalid", "plugins.%s.enabled must be true or false, got %v", name, v)
				continue
			}
			if !enabled {
				continue
			}
		}
		if k, ok := p.(plugin.ConfigKeys); ok {
			allowed := map[string]bool{"enabled": true}
			for _, key := range k.ConfigKeys() {
				allowed[key] = true
			}
			for _, key := range sortedKeys(section) {
				if !allowed[key] {
					dc.Warnf(cfg.Path, 0, 0, "config.unknownKey", "unknown config key %q", "plugins."+name+"."+key)
				}
			}
		}
		if err := p.Init(section, site); err != nil {
			dc.Errorf(cfg.Path, 0, 0, "plugin.config", "plugin %s: %v", name, err)
			continue
		}
		set.plugins = append(set.plugins, p)
	}
	for _, name := range sortedKeys(cfg.Plugins) {
		if !known[name] {
			dc.Warnf(cfg.Path, 0, 0, "config.unknownKey", "unknown plugin %q in plugins", name)
		}
	}
	slices.SortStableFunc(set.plugins, func(a, b plugin.Plugin) int {
		return cmp.Or(cmp.Compare(priority(a), priority(b)), cmp.Compare(a.Name(), b.Name()))
	})
	return set, dc.All()
}

func priority(p plugin.Plugin) int {
	if o, ok := p.(plugin.Ordered); ok {
		return o.Priority()
	}
	return plugin.DefaultPriority
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Plugins returns the enabled plugins in phase order.
func (s *Set) Plugins() []plugin.Plugin { return s.plugins }

// Each returns the enabled plugins implementing T, in phase order.
func Each[T any](s *Set) []T {
	var out []T
	for _, p := range s.plugins {
		if t, ok := p.(T); ok {
			out = append(out, t)
		}
	}
	return out
}

// Directives maps directive names to the plugins rendering them. A name
// claimed twice, or a built-in name, is an error.
func (s *Set) Directives() (map[string]plugin.DirectiveRenderer, []diag.Diagnostic) {
	var ds []diag.Diagnostic
	m := map[string]plugin.DirectiveRenderer{}
	owner := map[string]string{}
	for _, p := range s.plugins {
		dr, ok := p.(plugin.DirectiveRenderer)
		if !ok {
			continue
		}
		for _, name := range dr.DirectiveNames() {
			switch {
			case markdown.IsBuiltin(name):
				ds = append(ds, diag.Diagnostic{Severity: diag.Error, Code: "plugin.config",
					Message: fmt.Sprintf("plugin %s: directive %q is built in", p.Name(), name)})
			case owner[name] != "":
				ds = append(ds, diag.Diagnostic{Severity: diag.Error, Code: "plugin.config",
					Message: fmt.Sprintf("plugin %s: directive %q is already rendered by %s", p.Name(), name, owner[name])})
			default:
				m[name], owner[name] = dr, p.Name()
			}
		}
	}
	return m, ds
}

// Stylesheets lists the CSS files enabled plugins need, in phase order.
func (s *Set) Stylesheets() []string {
	var out []string
	for _, p := range Each[plugin.Stylesheets](s) {
		out = append(out, p.Stylesheets()...)
	}
	return out
}

// decode copies a plugin config section into the yaml-tagged struct out.
func decode(section map[string]any, out any) error {
	if len(section) == 0 {
		return nil
	}
	b, err := yaml.Marshal(section)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(b, out)
}
