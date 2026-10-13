// Package output writes a built Site to disk.
package output

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Pinta365/tryck/internal/build"
)

// Stats summarizes what Write produced.
type Stats struct {
	Pages  int // generated files (pages, 404, site-wide outputs)
	Copied int // files copied from staticDir and contentDir
}

// Write emits site into its configured outputDir: it optionally cleans
// the directory, copies staticDir and content assets, then writes the
// generated files, which win over copied files with the same path.
func Write(site *build.Site) (Stats, error) {
	cfg := site.Config
	out := cfg.Dir(cfg.Build.OutputDir)
	var st Stats

	if cfg.Build.CleanOutput {
		if err := checkCleanable(out, cfg.Root, cfg.Dir(cfg.Build.ContentDir), cfg.Dir(cfg.Build.StaticDir), cfg.Dir(cfg.Build.LayoutsDir)); err != nil {
			return st, err
		}
		if err := clean(out); err != nil {
			return st, err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return st, err
	}

	n, err := copyTree(cfg.Dir(cfg.Build.StaticDir), out)
	st.Copied += n
	if err != nil {
		return st, fmt.Errorf("copy static files: %w", err)
	}
	for _, a := range site.Assets {
		if err := copyFile(a.Src, filepath.Join(out, filepath.FromSlash(a.OutPath))); err != nil {
			return st, fmt.Errorf("copy %s: %w", a.Src, err)
		}
		st.Copied++
	}
	for p, data := range site.Outputs {
		if err := writeFile(filepath.Join(out, filepath.FromSlash(p)), data); err != nil {
			return st, err
		}
		st.Pages++
	}
	return st, nil
}

// checkCleanable refuses to clean an outputDir that is, or contains, the
// project root or one of the source directories.
func checkCleanable(out string, sources ...string) error {
	absOut, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if absOut == filepath.VolumeName(absOut)+string(filepath.Separator) {
		return fmt.Errorf("refusing to clean output directory %s: it is the filesystem root", out)
	}
	for _, src := range sources {
		absSrc, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		if absSrc == absOut || strings.HasPrefix(absSrc, absOut+string(filepath.Separator)) {
			return fmt.Errorf("refusing to clean output directory %s: it contains source directory %s", out, src)
		}
	}
	return nil
}

// clean removes the contents of dir but keeps dir itself, so a server
// or editor watching it is not disturbed.
func clean(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies every regular file under src into dst. A missing src
// copies nothing.
func copyTree(src, dst string) (int, error) {
	n := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == src && os.IsNotExist(err) {
				return fs.SkipAll
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if err := copyFile(p, filepath.Join(dst, rel)); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writeFile(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
