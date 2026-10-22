package tryck_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Pinta365/tryck/pkg/plugin"
	"github.com/Pinta365/tryck/pkg/tryck"
)

// stamp adds a build stamp to the output.
type stamp struct{}

func (stamp) Name() string                               { return "stamp" }
func (stamp) Init(map[string]any, plugin.SiteInfo) error { return nil }
func (stamp) OnBuildComplete(s plugin.SiteView, out plugin.Outputs) error {
	return out.Add("stamp.txt", []byte(fmt.Sprintf("%d pages\n", len(s.Pages()))))
}

// A custom tryck binary is one call. It has every tryck command, with
// the plugins running next to the built-ins.
func ExampleMain() {
	tryck.Main(stamp{})
}

// Run runs the command line in the current process and returns the exit
// code, which makes a plugin easy to test.
func ExampleRun() {
	dir, _ := os.MkdirTemp("", "site")
	defer os.RemoveAll(dir)
	config := filepath.Join(dir, "tryck.config.yaml")
	_ = os.WriteFile(config, []byte("site:\n  baseUrl: https://example.com/\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "content"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "content", "index.md"), []byte("# Home\n"), 0o644)

	code := tryck.Run([]string{"build", "--config", config}, io.Discard, os.Stderr, stamp{})
	out, _ := os.ReadFile(filepath.Join(dir, "dist", "stamp.txt"))
	fmt.Printf("exit code %d, %s", code, out)
	// Output: exit code 0, 1 pages
}
