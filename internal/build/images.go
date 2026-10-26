package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif" // register decoders for image.DecodeConfig
	"image/jpeg"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/yuin/goldmark/ast"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // and WebP

	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/diag"
)

// images reads image files for one build: their size for width and
// height, and, with images.resize, smaller copies for a srcset. Each file
// is read once per build however many pages show it. It is safe for
// concurrent use.
type images struct {
	cfg      config.ImagesConfig
	cacheDir string              // resized copies by content hash; "" = none
	rel      func(string) string // file path for diagnostics

	mu      sync.Mutex
	files   map[string]*imageFile // by file on disk
	outputs map[string][]byte     // resized copies by output path
	diags   []diag.Diagnostic
}

// imageFile is what is known about one image file.
type imageFile struct {
	once   sync.Once
	size   image.Point // as displayed, after EXIF orientation; zero if unknown
	format string      // "png", "jpeg", "gif" or "webp"
	orient int         // EXIF orientation of a JPEG, 1-8

	resizeOnce sync.Once
	copies     []imageCopy
}

// imageCopy is a resized copy of an image.
type imageCopy struct {
	width int
	out   string // output path
}

// ImageCacheDir is the default directory for resized images: tryck/images
// in the user's cache directory, or "" if there is none.
func ImageCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "tryck", "images")
}

func newImages(cfg config.ImagesConfig, cacheDir string, rel func(string) string) *images {
	return &images{cfg: cfg, cacheDir: cacheDir, rel: rel, files: map[string]*imageFile{}, outputs: map[string][]byte{}}
}

// file returns the image file's information, reading its header the
// first time.
func (im *images) file(name string) *imageFile {
	im.mu.Lock()
	f := im.files[name]
	if f == nil {
		f = &imageFile{}
		im.files[name] = f
	}
	im.mu.Unlock()
	f.once.Do(func() { f.readHeader(name) })
	return f
}

func (f *imageFile) readHeader(name string) {
	data, err := os.Open(name)
	if err != nil {
		return
	}
	defer data.Close()
	c, format, err := image.DecodeConfig(data)
	if err != nil || c.Width <= 0 || c.Height <= 0 {
		return
	}
	f.format, f.orient = format, 1
	f.size = image.Point{X: c.Width, Y: c.Height}
	if format == "jpeg" {
		if _, err := data.Seek(0, 0); err == nil {
			f.orient = jpegOrientation(data)
		}
		if f.orient >= 5 { // rotated a quarter turn
			f.size.X, f.size.Y = f.size.Y, f.size.X
		}
	}
}

// resized returns the copies of the image file name, served at out, for
// each configured width below its own, creating them or reading them from
// the cache the first time. taken reports output paths already in use.
func (im *images) resized(name, out string, f *imageFile, taken func(string) bool) []imageCopy {
	if f.format != "png" && f.format != "jpeg" {
		return nil // GIFs may be animated; WebP cannot be encoded
	}
	f.resizeOnce.Do(func() {
		var err error
		f.copies, err = im.resize(name, out, f, taken)
		if err != nil {
			im.report(diag.Warning, name, "image.resize", fmt.Sprintf("cannot resize %s: %v", im.rel(name), err))
		}
	})
	return f.copies
}

func (im *images) resize(name, out string, f *imageFile, taken func(string) bool) ([]imageCopy, error) {
	widths := slices.Clone(im.cfg.Widths)
	slices.Sort(widths)
	widths = slices.Compact(widths)
	if len(widths) == 0 || widths[0] >= f.size.X {
		return nil, nil
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	var src image.Image // decoded on the first cache miss
	var copies []imageCopy
	for _, w := range widths {
		if w >= f.size.X {
			break
		}
		cp := imageCopy{width: w, out: copyPath(out, w)}
		if taken(cp.out) {
			im.report(diag.Warning, name, "image.conflict", fmt.Sprintf("not writing a %dpx copy of %s: %s already exists", w, im.rel(name), cp.out))
			continue
		}
		key := fmt.Sprintf("%s-%dw-q%d%s", hex.EncodeToString(sum[:]), w, im.cfg.Quality, path.Ext(out))
		enc := im.cached(key)
		if enc == nil {
			if src == nil {
				if src, _, err = image.Decode(bytes.NewReader(data)); err != nil {
					return copies, err
				}
			}
			if enc, err = im.encode(src, f, w); err != nil {
				return copies, err
			}
			im.store(key, enc)
		}
		if len(enc) >= len(data) {
			continue // no smaller than the original
		}
		im.mu.Lock()
		im.outputs[cp.out] = enc
		im.mu.Unlock()
		copies = append(copies, cp)
	}
	return copies, nil
}

// encode scales src to width w as displayed and encodes it in the
// image's own format.
func (im *images) encode(src image.Image, f *imageFile, w int) ([]byte, error) {
	h := max(1, (f.size.Y*w+f.size.X/2)/f.size.X)
	sw, sh := w, h
	if f.orient >= 5 {
		sw, sh = h, w // scale as stored, then rotate
	}
	var dst draw.Image = image.NewRGBA(image.Rect(0, 0, sw, sh))
	if _, gray := src.(*image.Gray); gray && f.orient <= 1 {
		dst = image.NewGray(dst.Bounds()) // a third of the size of RGB
	}
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	if rgba, ok := dst.(*image.RGBA); ok {
		dst = orient(rgba, f.orient)
	}
	var buf bytes.Buffer
	var err error
	if f.format == "jpeg" {
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: im.cfg.Quality})
	} else {
		err = png.Encode(&buf, dst)
	}
	return buf.Bytes(), err
}

func (im *images) cached(key string) []byte {
	if im.cacheDir == "" {
		return nil
	}
	data, _ := os.ReadFile(filepath.Join(im.cacheDir, key))
	return data
}

// store writes a resized copy to the cache. Failing to is not an error:
// the next build resizes again.
func (im *images) store(key string, data []byte) {
	if im.cacheDir == "" || os.MkdirAll(im.cacheDir, 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(im.cacheDir, key+".*.tmp")
	if err != nil {
		return
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil && cerr == nil {
		err = os.Rename(tmp.Name(), filepath.Join(im.cacheDir, key))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
}

func (im *images) report(sev diag.Severity, name, code, msg string) {
	im.mu.Lock()
	im.diags = append(im.diags, diag.Diagnostic{Severity: sev, File: im.rel(name), Code: code, Message: msg})
	im.mu.Unlock()
}

// copyPath is the output path of an image's copy w pixels wide:
// "guide/shot.png" -> "guide/shot-800w.png".
func copyPath(out string, w int) string {
	ext := path.Ext(out)
	return fmt.Sprintf("%s-%dw%s", strings.TrimSuffix(out, ext), w, ext)
}

// imageAttrs are the attributes setImageAttrs manages.
var imageAttrs = []string{"width", "height", "loading", "srcset", "sizes"}

// setImageAttrs sets width and height on img from the file's size, when
// known and attrs is set, loading="lazy", and srcset and sizes when there
// is one. It first removes what an earlier build set on a cached AST,
// keeping other attributes.
func setImageAttrs(img *ast.Image, size image.Point, attrs bool, srcset, sizes string) {
	var keep []ast.Attribute
	for _, a := range img.Attributes() {
		if !slices.Contains(imageAttrs, string(a.Name)) {
			keep = append(keep, a)
		}
	}
	img.RemoveAttributes()
	for _, a := range keep {
		img.SetAttribute(a.Name, a.Value)
	}
	if !attrs && srcset == "" {
		return
	}
	if attrs && size.X > 0 {
		img.SetAttributeString("width", []byte(strconv.Itoa(size.X)))
		img.SetAttributeString("height", []byte(strconv.Itoa(size.Y)))
	}
	if srcset != "" {
		img.SetAttributeString("srcset", []byte(srcset))
		img.SetAttributeString("sizes", []byte(sizes))
	}
	if attrs {
		img.SetAttributeString("loading", []byte("lazy"))
	}
}
