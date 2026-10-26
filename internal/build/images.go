package build

import (
	"image"
	_ "image/gif" // register decoders for image.DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"os"
	"slices"
	"strconv"
	"sync"

	"github.com/yuin/goldmark/ast"
	_ "golang.org/x/image/webp" // and WebP
)

// imageSizes reads and remembers the pixel size of image files for one
// build. Only the header is decoded. A file that cannot be read or is
// not a PNG, JPEG, GIF or WebP image has no size. It is safe for
// concurrent use.
type imageSizes struct {
	mu    sync.Mutex
	sizes map[string]image.Point // file -> size, zero if unknown
}

func newImageSizes() *imageSizes {
	return &imageSizes{sizes: map[string]image.Point{}}
}

func (s *imageSizes) size(file string) (image.Point, bool) {
	s.mu.Lock()
	pt, ok := s.sizes[file]
	s.mu.Unlock()
	if !ok {
		pt = decodeSize(file) // outside the lock; a race only decodes twice
		s.mu.Lock()
		s.sizes[file] = pt
		s.mu.Unlock()
	}
	return pt, pt.X > 0 && pt.Y > 0
}

func decodeSize(file string) image.Point {
	f, err := os.Open(file)
	if err != nil {
		return image.Point{}
	}
	defer f.Close()
	c, _, err := image.DecodeConfig(f)
	if err != nil {
		return image.Point{}
	}
	return image.Point{X: c.Width, Y: c.Height}
}

// imageAttrs are the attributes setImageAttrs manages.
var imageAttrs = []string{"width", "height", "loading"}

// setImageAttrs sets width and height on img from the file's size, when
// known, and loading="lazy". It first removes what an earlier build set
// on a cached AST, keeping other attributes.
func setImageAttrs(img *ast.Image, size image.Point, known bool) {
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
	if known {
		img.SetAttributeString("width", []byte(strconv.Itoa(size.X)))
		img.SetAttributeString("height", []byte(strconv.Itoa(size.Y)))
	}
	img.SetAttributeString("loading", []byte("lazy"))
}
