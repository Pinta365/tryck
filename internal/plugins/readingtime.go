package plugins

import (
	"errors"
	"unicode"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/pkg/plugin"
)

// ReadingTime stores Data["wordCount"] and Data["readingTime"] (whole
// minutes, at least 1). Code blocks and raw HTML are not counted.
type ReadingTime struct {
	WordsPerMinute int `yaml:"wordsPerMinute"`
}

func (*ReadingTime) Name() string         { return "readingTime" }
func (*ReadingTime) configKeys() []string { return []string{"wordsPerMinute"} }

func (r *ReadingTime) Init(cfg map[string]any, _ plugin.SiteInfo) error {
	r.WordsPerMinute = 200
	if err := decode(cfg, r); err != nil {
		return err
	}
	if r.WordsPerMinute <= 0 {
		return errors.New("wordsPerMinute must be positive")
	}
	return nil
}

func (r *ReadingTime) OnPageParsed(p *plugin.Page) error {
	words := 0
	inWord := false
	_ = ast.Walk(p.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			for _, r := range string(t.Segment.Value(p.Source)) {
				if unicode.IsSpace(r) {
					inWord = false
				} else if !inWord {
					inWord = true
					words++
				}
			}
			if t.SoftLineBreak() || t.HardLineBreak() {
				inWord = false
			}
		default:
			if n.Type() == ast.TypeBlock {
				inWord = false
			}
		}
		return ast.WalkContinue, nil
	})
	p.Data["wordCount"] = words
	p.Data["readingTime"] = max(1, (words+r.WordsPerMinute-1)/r.WordsPerMinute)
	return nil
}
