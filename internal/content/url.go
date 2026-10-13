package content

import (
	"path"
	"regexp"
	"strings"
)

var (
	numericPrefixRE = regexp.MustCompile(`^(\d+)-`)
	datePrefixRE    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(-|$)`)
)

// StripNumericPrefix removes an ordering prefix like "01-" from a path
// segment. A segment that would become empty, or that starts with a
// YYYY-MM-DD date (a dated blog post), is returned unchanged.
func StripNumericPrefix(seg string) string {
	if datePrefixRE.MatchString(seg) {
		return seg
	}
	if m := numericPrefixRE.FindString(seg); m != "" && len(m) < len(seg) {
		return seg[len(m):]
	}
	return seg
}

// IsSectionIndex reports whether a content file is a section index page
// (_index.md or index.md).
func IsSectionIndex(rel string) bool {
	base := path.Base(rel)
	return base == "_index.md" || base == "index.md"
}

// URLFor maps a Markdown file path, relative to contentDir and using
// forward slashes, to its clean URL (spec §5.2):
//
//	docs/setup.md      -> /docs/setup/
//	docs/_index.md     -> /docs/
//	index.md           -> /
//	01-intro.md        -> /intro/
//
// A non-empty slug replaces the last URL segment. The root index page
// ignores the slug.
func URLFor(rel, slug string) string {
	rel = strings.TrimSuffix(rel, path.Ext(rel))
	segs := strings.Split(rel, "/")
	if last := segs[len(segs)-1]; last == "_index" || last == "index" {
		segs = segs[:len(segs)-1]
	}
	for i, s := range segs {
		segs[i] = StripNumericPrefix(s)
	}
	if slug = strings.Trim(slug, "/"); slug != "" && len(segs) > 0 {
		segs[len(segs)-1] = slug
	}
	if len(segs) == 0 {
		return "/"
	}
	return "/" + strings.Join(segs, "/") + "/"
}

// AssetPath maps a non-Markdown file in contentDir to its output path,
// stripping numeric prefixes from its directories like URLFor does.
func AssetPath(rel string) string {
	segs := strings.Split(rel, "/")
	for i := range segs[:len(segs)-1] {
		segs[i] = StripNumericPrefix(segs[i])
	}
	return strings.Join(segs, "/")
}

// OutputPath maps a page URL to the file it is written to, relative to
// outputDir: "/docs/setup/" -> "docs/setup/index.html".
func OutputPath(url string) string {
	return strings.TrimPrefix(url, "/") + "index.html"
}
