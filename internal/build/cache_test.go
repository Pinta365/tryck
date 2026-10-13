package build_test

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
)

func TestCacheReuseAndRelink(t *testing.T) {
	cfg := writeSite(t, map[string]string{
		"content/a.md": "# A\n\nSee [b](b.md).\n",
		"content/b.md": "# B\n",
		"content/c.md": "# C\n",
	})
	cache := build.NewCache()
	opts := build.Options{Cache: cache}
	run := func() *build.Site {
		t.Helper()
		s, err := build.Build(context.Background(), cfg, opts)
		if err != nil {
			t.Fatal(err)
		}
		if s.HasErrors() {
			t.Fatalf("errors: %v", s.Diags)
		}
		return s
	}

	first := run()
	if cache.Len() != 3 {
		t.Fatalf("cache has %d pages, want 3", cache.Len())
	}
	second := run()
	if !maps.EqualFunc(first.Outputs, second.Outputs, func(x, y []byte) bool { return string(x) == string(y) }) {
		t.Error("cached rebuild differs from first build")
	}
	if first.Pages[0].AST != second.Pages[0].AST {
		t.Error("unchanged page was parsed again")
	}

	// a.md is unchanged and cached, but its link must follow b's new slug.
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(cfg.Root, "content", rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.md", "---\nslug: bee\n---\n# B\n")
	third := run()
	if a := string(third.Outputs["a/index.html"]); !strings.Contains(a, `<a href="/bee/">b</a>`) {
		t.Errorf("cached page kept a stale link:\n%s", a)
	}

	if err := os.Remove(filepath.Join(cfg.Root, "content", "c.md")); err != nil {
		t.Fatal(err)
	}
	run()
	if cache.Len() != 2 {
		t.Errorf("cache has %d pages after deleting one, want 2", cache.Len())
	}

	uncached, err := build.Build(context.Background(), cfg, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cached := run()
	if !maps.EqualFunc(uncached.Outputs, cached.Outputs, func(x, y []byte) bool { return string(x) == string(y) }) {
		t.Error("cached build differs from an uncached one")
	}
}
