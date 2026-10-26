package plugins

import (
	"bytes"
	"slices"
	"strings"
)

// addToHead adds links, one tag per line, to a page's <head>: after the
// title, so the head-end partial stays last, or before </head> in a
// layout without a title.
func addToHead(html []byte, links string) []byte {
	links = strings.TrimSuffix(links, "\n")
	if links == "" {
		return html
	}
	if i := bytes.Index(html, []byte("</title>")); i >= 0 {
		at := i + len("</title>")
		return slices.Concat(html[:at], []byte("\n"+links), html[at:])
	}
	return bytes.Replace(html, []byte("</head>"), []byte(links+"\n</head>"), 1)
}
