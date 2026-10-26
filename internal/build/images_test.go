package build_test

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
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

// noisePNG is a w×h PNG that does not compress well, so its copies are
// smaller than it.
func noisePNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = byte(rnd.Uint32())
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func decodeSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	c, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return c.Width, c.Height
}

func TestImageResize(t *testing.T) {
	cfg := writeSite(t, map[string]string{
		"tryck.config.yaml":           "site:\n  baseUrl: https://example.com/docs/\nimages:\n  resize: true\n  widths: [800, 480, 1600]\n  sizes: 50vw\n",
		"content/guide/a.md":          "# A\n\n![shot](img/shot.png)\n\n![small](img/small.png)\n",
		"content/guide/img/shot.png":  noisePNG(t, 1000, 500),
		"content/guide/img/small.png": noisePNG(t, 300, 100),
	})
	cache := t.TempDir()
	site, err := build.Build(context.Background(), cfg, build.Options{ImageCache: cache})
	if err != nil {
		t.Fatal(err)
	}
	html := string(site.Outputs["guide/a/index.html"])
	want := `<img src="/docs/guide/img/shot.png" alt="shot" width="1000" height="500" srcset="/docs/guide/img/shot-480w.png 480w, /docs/guide/img/shot-800w.png 800w, /docs/guide/img/shot.png 1000w" sizes="50vw" loading="lazy">`
	if !strings.Contains(html, want) {
		t.Errorf("missing %s in:\n%s", want, html)
	}
	if !strings.Contains(html, `<img src="/docs/guide/img/small.png" alt="small" width="300" height="100" loading="lazy">`) {
		t.Errorf("an image narrower than every width has a srcset:\n%s", html)
	}
	for out, w := range map[string]int{"guide/img/shot-480w.png": 480, "guide/img/shot-800w.png": 800} {
		data, ok := site.Outputs[out]
		if !ok {
			t.Fatalf("%s not written", out)
		}
		if gw, gh := decodeSize(t, data); gw != w || gh != w/2 {
			t.Errorf("%s is %dx%d, want %dx%d", out, gw, gh, w, w/2)
		}
	}
	if _, ok := site.Outputs["guide/img/shot-1600w.png"]; ok {
		t.Error("a copy wider than the original is written")
	}

	// The next build reads the copies from the cache.
	entries, err := os.ReadDir(cache)
	if err != nil || len(entries) != 2 {
		t.Fatalf("cache has %d entries (%v), want 2", len(entries), err)
	}
	for _, e := range entries {
		if err := os.WriteFile(filepath.Join(cache, e.Name()), []byte("cached"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	site, err = build.Build(context.Background(), cfg, build.Options{ImageCache: cache})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(site.Outputs["guide/img/shot-800w.png"]); got != "cached" {
		t.Errorf("copy not read from the cache: %d bytes", len(got))
	}
}

func TestImageResizeConflict(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml":    "site:\n  baseUrl: https://example.com/\nimages:\n  resize: true\n  widths: [480, 800]\n",
		"content/a.md":         "# A\n\n![shot](/shot.png)\n",
		"static/shot.png":      noisePNG(t, 1000, 500),
		"static/shot-480w.png": "mine",
	}))
	if d, ok := findDiag(site.Diags, "image.conflict"); !ok || !strings.Contains(d.Message, "shot-480w.png already exists") {
		t.Errorf("no image.conflict warning: %v", site.Diags)
	}
	if _, ok := site.Outputs["shot-480w.png"]; ok {
		t.Error("the existing file is overwritten")
	}
	if html := string(site.Outputs["a/index.html"]); !strings.Contains(html, `srcset="/shot-800w.png 800w, /shot.png 1000w"`) {
		t.Errorf("srcset:\n%s", html)
	}
}

// withOrientation inserts an EXIF APP1 segment with orientation o after
// a JPEG's start marker.
func withOrientation(jpg string, o byte) string {
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, o, 0, 0, 0, 0, 0, 0, 0, 0}
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := append([]byte{0xFF, 0xE1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)}, seg...)
	return jpg[:2] + string(app1) + jpg[2:]
}

func TestImageResizeOrientation(t *testing.T) {
	// Stored 400×200 with a black top-left corner; orientation 6 shows
	// it turned a quarter right, 200×400 with the corner top right.
	img := image.NewGray(image.Rect(0, 0, 400, 200))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := range 50 {
		for x := range 50 {
			img.Pix[img.PixOffset(x, y)] = 0
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\nimages:\n  resize: true\n  widths: [100]\n",
		"content/a.md":      "# A\n\n![photo](photo.jpg)\n",
		"content/photo.jpg": withOrientation(buf.String(), 6),
	}))
	if html := string(site.Outputs["a/index.html"]); !strings.Contains(html, `width="200" height="400" srcset="/photo-100w.jpg 100w, /photo.jpg 200w"`) {
		t.Errorf("size of a turned photo:\n%s", html)
	}
	cp, err := jpeg.Decode(bytes.NewReader(site.Outputs["photo-100w.jpg"]))
	if err != nil {
		t.Fatal(err)
	}
	if b := cp.Bounds(); b.Dx() != 100 || b.Dy() != 200 {
		t.Fatalf("copy is %v, want 100x200", b)
	}
	dark := func(x, y int) bool { r, _, _, _ := cp.At(x, y).RGBA(); return r < 0x4000 }
	if !dark(95, 5) || dark(5, 5) {
		t.Error("copy is not turned a quarter right")
	}
}
