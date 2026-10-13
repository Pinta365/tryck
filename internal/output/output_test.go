package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/content"
)

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newSite(t *testing.T) (*build.Site, string) {
	t.Helper()
	cfg := config.Defaults()
	cfg.Root = t.TempDir()
	return &build.Site{Config: cfg, Outputs: map[string][]byte{}}, cfg.Root
}

func TestWrite(t *testing.T) {
	site, root := newSite(t)
	write(t, filepath.Join(root, "static/robots.txt"), "robots")
	write(t, filepath.Join(root, "static/.well-known/security.txt"), "sec")
	write(t, filepath.Join(root, "static/index.html"), "static loses")
	write(t, filepath.Join(root, "content/docs/img.png"), "png")
	write(t, filepath.Join(root, "dist/stale.html"), "old")
	site.Assets = []content.Asset{{Src: filepath.Join(root, "content/docs/img.png"), OutPath: "docs/img.png"}}
	site.Outputs["index.html"] = []byte("home")
	site.Outputs["docs/setup/index.html"] = []byte("setup")

	st, err := Write(site)
	if err != nil {
		t.Fatal(err)
	}
	if st.Pages != 2 || st.Copied != 4 {
		t.Errorf("stats = %+v, want 2 pages and 4 copied", st)
	}
	dist := filepath.Join(root, "dist")
	for p, want := range map[string]string{
		"robots.txt":               "robots",
		".well-known/security.txt": "sec",
		"index.html":               "home",
		"docs/setup/index.html":    "setup",
		"docs/img.png":             "png",
	} {
		if got := read(t, filepath.Join(dist, p)); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dist, "stale.html")); !os.IsNotExist(err) {
		t.Error("cleanOutput did not remove stale file")
	}
}

func TestWriteWithoutClean(t *testing.T) {
	site, root := newSite(t)
	site.Config.Build.CleanOutput = false
	write(t, filepath.Join(root, "dist/keep.html"), "keep")
	if _, err := Write(site); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(root, "dist/keep.html")) != "keep" {
		t.Error("file removed with cleanOutput: false")
	}
}

func TestRefusesToCleanSources(t *testing.T) {
	for _, outDir := range []string{".", "./", "..", "content", "static/..", "./layouts"} {
		t.Run(outDir, func(t *testing.T) {
			site, root := newSite(t)
			write(t, filepath.Join(root, "content/index.md"), "# keep me")
			write(t, filepath.Join(root, "layouts/doc.html"), "keep")
			site.Config.Build.OutputDir = outDir
			_, err := Write(site)
			if err == nil || !strings.Contains(err.Error(), "refusing to clean") {
				t.Fatalf("err = %v, want refusal", err)
			}
			if read(t, filepath.Join(root, "content/index.md")) != "# keep me" {
				t.Error("source was deleted")
			}
		})
	}
}

func TestCleanAllowsNestedOutput(t *testing.T) {
	site, root := newSite(t)
	site.Config.Build.OutputDir = "build/site"
	site.Outputs["index.html"] = []byte("x")
	if _, err := Write(site); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(root, "build/site/index.html")) != "x" {
		t.Error("nested output not written")
	}
}

func TestAssetOverridesStaticFile(t *testing.T) {
	site, root := newSite(t)
	write(t, filepath.Join(root, "static/docs/img.png"), "static")
	write(t, filepath.Join(root, "content/docs/img.png"), "content")
	site.Assets = []content.Asset{{Src: filepath.Join(root, "content/docs/img.png"), OutPath: "docs/img.png"}}
	st, err := Write(site)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(root, "dist/docs/img.png")); got != "content" || st.Copied != 1 {
		t.Errorf("img = %q, copied = %d; want content asset copied once", got, st.Copied)
	}
}
