// Command plugin-hello is a custom tryck binary with one extra plugin. It
// shows how little a plugin build needs: define the plugin, then call
// tryck.Main with it. Build it like any Go program:
//
//	go build ./examples/plugin-hello
//
// See pkg/plugin for the full plugin API.
package main

import (
	"fmt"

	"github.com/Pinta365/tryck/pkg/plugin"
	"github.com/Pinta365/tryck/pkg/tryck"
)

// Hello is a plugin configured under "plugins.hello" in tryck.config.yaml:
//
//	plugins:
//	  hello:
//	    greeting: Hej
type Hello struct {
	greeting string
}

// Name is the plugin's config key under "plugins:".
func (*Hello) Name() string { return "hello" }

// ConfigKeys declares the keys of the "plugins.hello" section. Any other
// key warns, so a typo like "greetng" is reported instead of ignored.
// ("enabled" is always allowed and need not be listed.)
func (*Hello) ConfigKeys() []string { return []string{"greeting"} }

// Init receives the plugin's config section (nil if absent).
func (h *Hello) Init(cfg map[string]any, _ plugin.SiteInfo) error {
	h.greeting = "Hello" // the default
	if v, ok := cfg["greeting"].(string); ok {
		h.greeting = v
	}
	return nil
}

// OnPageParsed runs for every page, in parallel, after its Markdown is
// parsed. It sets a value the layouts can use as .Page.Data.greeting.
func (h *Hello) OnPageParsed(p *plugin.Page) error {
	p.Data["greeting"] = h.greeting
	return nil
}

// OnBuildComplete runs once after every page is rendered. It adds a file
// to the output (dist/hello.txt).
func (h *Hello) OnBuildComplete(s plugin.SiteView, out plugin.Outputs) error {
	text := fmt.Sprintf("%s from %d pages\n", h.greeting, len(s.Pages()))
	return out.Add("hello.txt", []byte(text))
}

func main() { tryck.Main(&Hello{}) }
