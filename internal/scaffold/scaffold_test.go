package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Pinta365/tryck"
	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/config"
)

func TestInit(t *testing.T) {
	src := fstest.MapFS{
		"a.txt":         {Data: []byte("a")},
		"sub/.hidden/b": {Data: []byte("b")},
	}
	dir := filepath.Join(t.TempDir(), "new")
	files, err := Init(dir, src)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"a.txt", "sub/.hidden/b"}) {
		t.Errorf("files = %v", files)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "sub", ".hidden", "b")); string(b) != "b" {
		t.Error("nested dotfile not copied")
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(dir, src); err == nil || !strings.Contains(err.Error(), "a.txt") {
		t.Errorf("err = %v, want refusal naming a.txt", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "mine" {
		t.Error("existing file overwritten")
	}
}

// TestStarterBuildsClean checks the embedded starter builds without any
// warnings or errors and ships the GitHub Pages workflow.
func TestStarterBuildsClean(t *testing.T) {
	dir := t.TempDir()
	files, err := Init(dir, tryck.Starter())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tryck.config.yaml", ".github/workflows/pages.yml", ".gitignore", "content/index.md"} {
		if !slices.Contains(files, want) {
			t.Errorf("starter is missing %s (has %v)", want, files)
		}
	}
	cfg, ds := config.Load(filepath.Join(dir, config.DefaultPath), true)
	if len(ds) != 0 {
		t.Fatalf("config: %v", ds)
	}
	site, err := build.Build(context.Background(), cfg, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(site.Diags) != 0 {
		t.Errorf("starter has diagnostics: %v", site.Diags)
	}
	if len(site.Pages) != 4 {
		t.Errorf("starter has %d pages, want 4", len(site.Pages))
	}
}
