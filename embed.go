// Package tryck holds files embedded in the tryck binary. The command
// line lives in pkg/tryck and the public plugin API in pkg/plugin.
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
