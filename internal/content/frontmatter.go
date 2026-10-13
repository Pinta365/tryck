package content

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// FrontMatter is the YAML header of a page (spec §5.1).
type FrontMatter = plugin.FrontMatter

var utf8BOM = []byte("\xef\xbb\xbf")

// SplitFrontMatter separates a leading "---" delimited YAML block from the
// body. bodyLine is the 1-based file line on which the body starts, used
// to map body positions back to the file. If src has no front matter, fm
// is nil and body is src. An opening "---" without a closing one is an
// error.
func SplitFrontMatter(src []byte) (fm, body []byte, bodyLine int, err error) {
	src = bytes.TrimPrefix(src, utf8BOM)
	first, rest, _ := cutLine(src)
	if !isFence(first) {
		return nil, src, 1, nil
	}
	start := len(src) - len(rest)
	line := 2
	for len(rest) > 0 {
		l, next, _ := cutLine(rest)
		if isFence(l) {
			end := len(src) - len(rest)
			return src[start:end], next, line + 1, nil
		}
		rest = next
		line++
	}
	return nil, src, 1, errors.New("front matter starts with --- but has no closing ---")
}

// cutLine returns the first line of b without its line ending.
func cutLine(b []byte) (line, rest []byte, found bool) {
	line, rest, found = bytes.Cut(b, []byte("\n"))
	return bytes.TrimSuffix(line, []byte("\r")), rest, found
}

func isFence(line []byte) bool {
	return string(bytes.TrimRight(line, " \t")) == "---"
}

var yamlLineRE = regexp.MustCompile(`^(?:yaml: )?line (\d+): `)

// ParseFrontMatter decodes fm. Diagnostics point into file, with YAML
// line numbers shifted by the opening fence.
func ParseFrontMatter(file string, fm []byte) (FrontMatter, []diag.Diagnostic) {
	var front FrontMatter
	if len(bytes.TrimSpace(fm)) == 0 {
		return front, nil
	}
	err := yaml.Unmarshal(fm, &front)
	if err == nil {
		return front, nil
	}
	msgs := []string{err.Error()}
	var te *yaml.TypeError
	if errors.As(err, &te) {
		msgs = te.Errors
	}
	var ds []diag.Diagnostic
	for _, msg := range msgs {
		d := diag.Diagnostic{Severity: diag.Error, File: file, Code: "frontmatter.yaml"}
		if m := yamlLineRE.FindStringSubmatch(msg); m != nil {
			n, _ := strconv.Atoi(m[1])
			d.Line = n + 1 // the opening --- is line 1
			msg = msg[len(m[0]):]
		}
		d.Message = "invalid front matter: " + strings.TrimPrefix(msg, "yaml: ")
		ds = append(ds, d)
	}
	return front, ds
}
