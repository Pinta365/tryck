// Package scaffold creates a new site from a template file tree.
package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Init copies every file in src into dir, creating dir if needed. It
// writes nothing if any of the files already exists, and returns the
// written paths relative to dir.
func Init(dir string, src fs.FS) ([]string, error) {
	var files []string
	if err := fs.WalkDir(src, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files = append(files, p)
		return nil
	}); err != nil {
		return nil, err
	}

	var exists []string
	for _, f := range files {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(f))); err == nil {
			exists = append(exists, f)
		}
	}
	if len(exists) > 0 {
		return nil, fmt.Errorf("%s already contains %s; choose an empty directory", dir, strings.Join(exists, ", "))
	}

	for _, f := range files {
		data, err := fs.ReadFile(src, f)
		if err != nil {
			return nil, err
		}
		dst := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, err
		}
	}
	return files, nil
}
