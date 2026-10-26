package build_test

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
)

func encodePNG(t *testing.T, w, h int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func encodeJPEG(t *testing.T, w, h int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestImageAttributes(t *testing.T) {
	cfg := writeSite(t, map[string]string{
		"content/guide/a.md":         "# A\n\n![shot](img/shot.png)\n\n![logo](/logo.jpg)\n\n![broken](img/fake.png)\n\n![remote](https://example.org/x.png)\n",
		"content/guide/img/shot.png": encodePNG(t, 40, 30),
		"content/guide/img/fake.png": "not a png",
		"static/logo.jpg":            encodeJPEG(t, 16, 8),
	})
	cache := build.NewCache()
	site, err := build.Build(context.Background(), cfg, build.Options{Cache: cache})
	if err != nil {
		t.Fatal(err)
	}
	html := string(site.Outputs["guide/a/index.html"])
	for _, want := range []string{
		`<img src="/guide/img/shot.png" alt="shot" width="40" height="30" loading="lazy">`,
		`<img src="/logo.jpg" alt="logo" width="16" height="8" loading="lazy">`,
		`<img src="/guide/img/fake.png" alt="broken" loading="lazy">`,
		`<img src="https://example.org/x.png" alt="remote" loading="lazy">`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}

	// A changed image gets its new size, though the page is cached.
	shot := filepath.Join(cfg.Root, "content", "guide", "img", "shot.png")
	if err := os.WriteFile(shot, []byte(encodePNG(t, 20, 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	site, err = build.Build(context.Background(), cfg, build.Options{Cache: cache})
	if err != nil {
		t.Fatal(err)
	}
	if html := string(site.Outputs["guide/a/index.html"]); !strings.Contains(html, `alt="shot" width="20" height="10" loading="lazy">`) {
		t.Errorf("size not updated:\n%s", html)
	}

	off := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\ndefaults:\n  imageAttributes: false\n",
		"content/a.md":      "# A\n\n![shot](shot.png)\n",
		"content/shot.png":  encodePNG(t, 40, 30),
	}))
	if html := string(off.Outputs["a/index.html"]); !strings.Contains(html, `<img src="/shot.png" alt="shot">`) {
		t.Errorf("imageAttributes: false still sets attributes:\n%s", html)
	}
}
