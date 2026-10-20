// Package tryck embeds the starter site that tryck init copies. The
// command line is in pkg/tryck and the plugin API in pkg/plugin.
package tryck

import (
	"embed"
	"io/fs"
)

//go:embed all:examples/docs-starter
var starter embed.FS

// Starter returns the docs-starter example site that tryck init copies.
func Starter() fs.FS {
	sub, err := fs.Sub(starter, "examples/docs-starter")
	if err != nil {
		panic(err) // the path is fixed at compile time
	}
	return sub
}
