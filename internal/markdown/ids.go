package markdown

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
)

// Slugify turns text into a URL- and anchor-safe identifier: lowercase,
// letters and digits kept (including non-ASCII letters), runs of spaces,
// dashes and underscores become one dash, other punctuation is dropped.
//
//	"Setup & Install" -> "setup-install"
//	"Översikt 2.0"    -> "översikt-20"
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r) || r == '-' || r == '_':
			dash = true
		}
	}
	return b.String()
}

// ids generates unique heading IDs for one document. Duplicates get -1,
// -2 suffixes. It implements goldmark's parser.IDs.
type ids struct {
	used     map[string]bool
	suffixed []string // generated IDs that needed a suffix
}

func newIDs() *ids { return &ids{used: map[string]bool{}} }

func (s *ids) Generate(value []byte, kind ast.NodeKind) []byte {
	base := Slugify(string(value))
	if base == "" {
		base = "id"
		if kind == ast.KindHeading {
			base = "heading"
		}
	}
	id := base
	for i := 1; s.used[id]; i++ {
		id = base + "-" + strconv.Itoa(i)
	}
	if id != base {
		s.suffixed = append(s.suffixed, id)
	}
	s.used[id] = true
	return []byte(id)
}

func (s *ids) Put(value []byte) { s.used[string(value)] = true }
