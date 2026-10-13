// Package content discovers source files and models pages: front matter,
// URL mapping and titles.
package content

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
)

// Page is one Markdown source file and everything derived from it.
type Page struct {
	SourcePath string // relative to contentDir, forward slashes
	File       string // relative to project root, for diagnostics
	URL        string // "/docs/setup/"
	OutPath    string // relative to outputDir: "docs/setup/index.html"
	Front      FrontMatter
	Title      string // front matter title, first h1, or file name
	HasH1      bool   // the body contains a level-1 heading
	Source     []byte // body without front matter
	BodyLine   int    // file line of the first body line
	AST        ast.Node
	HTML       []byte // rendered body fragment
}

// LineAt converts a byte offset in Source to a 1-based file line.
func (p *Page) LineAt(offset int) int {
	offset = min(offset, len(p.Source))
	return p.BodyLine + bytes.Count(p.Source[:offset], []byte("\n"))
}

// Asset is a non-Markdown file inside contentDir, copied next to pages.
type Asset struct {
	Src     string // absolute or root-relative path on disk
	OutPath string // relative to outputDir
}

// Sources lists the files found in contentDir.
type Sources struct {
	Pages  []string // Markdown files, relative to contentDir, forward slashes
	Assets []Asset
}

// Discover walks dir for Markdown files and other assets, skipping
// dotfiles and dot-directories. A missing dir yields no sources.
func Discover(dir string) (Sources, error) {
	var src Sources
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if p != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.EqualFold(path.Ext(rel), ".md") {
			src.Pages = append(src.Pages, rel)
		} else {
			src.Assets = append(src.Assets, Asset{Src: p, OutPath: AssetPath(rel)})
		}
		return nil
	})
	return src, err
}

// FirstH1 returns the text of the first level-1 heading in doc, or "".
func FirstH1(doc ast.Node, source []byte) string {
	var title string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if h, ok := n.(*ast.Heading); ok && entering && h.Level == 1 {
			title = strings.TrimSpace(PlainText(h, source))
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return title
}

// PlainText concatenates the text content of n's inline descendants.
func PlainText(n ast.Node, source []byte) string {
	var b bytes.Buffer
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *ast.CodeSpan:
			for cc := t.FirstChild(); cc != nil; cc = cc.NextSibling() {
				if tx, ok := cc.(*ast.Text); ok {
					b.Write(tx.Segment.Value(source))
				}
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// TitleFromPath derives a title from a content path: the file name, or
// the directory name for section indexes, without numeric prefix, with
// dashes and underscores as spaces and the first letter capitalized.
// The root index yields "".
func TitleFromPath(rel string) string {
	name := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	if IsSectionIndex(rel) {
		dir := path.Dir(rel)
		if dir == "." {
			return ""
		}
		name = path.Base(dir)
	}
	return Humanize(StripNumericPrefix(name))
}

// Humanize turns "getting-started" into "Getting started".
func Humanize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
