// Package output writes a built Site to disk.
package output

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/internal/workpool"
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

	// Copies run first and in parallel; generated files are written
	// after, so they win over a copied file with the same path.
	var copies []copyJob
	static := cfg.Dir(cfg.Build.StaticDir)
	if err := content.WalkFiles(static, func(src, rel string) {
		copies = append(copies, copyJob{src, filepath.Join(out, rel)})
	}); err != nil {
		return st, fmt.Errorf("copy static files: %w", err)
	}
	for _, a := range site.Assets {
		copies = append(copies, copyJob{a.Src, filepath.Join(out, filepath.FromSlash(a.OutPath))})
	}
	// Two sources for one destination must not be copied concurrently;
	// the later one (a content asset over a static file) wins.
	last := map[string]int{}
	for i, c := range copies {
		last[c.dst] = i
	}
	kept := copies[:0]
	for i, c := range copies {
		if last[c.dst] == i {
			kept = append(kept, c)
		}
	}
	copies = kept
	workers := cfg.WorkerCount()
	if err := runAll(len(copies), workers, func(i int) error {
		if err := copyFile(copies[i].src, copies[i].dst); err != nil {
			return fmt.Errorf("copy %s: %w", copies[i].src, err)
		}
		return nil
	}); err != nil {
		return st, err
	}
	st.Copied = len(copies)

	paths := slices.Sorted(maps.Keys(site.Outputs))
	if err := runAll(len(paths), workers, func(i int) error {
		return writeFile(filepath.Join(out, filepath.FromSlash(paths[i])), site.Outputs[paths[i]])
	}); err != nil {
		return st, err
	}
	st.Pages = len(paths)
	return st, nil
}

type copyJob struct{ src, dst string }

// runAll runs fn for [0, n) on a worker pool and returns the first error.
// Remaining items are skipped after an error.
func runAll(n, workers int, fn func(i int) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	var first error
	_ = workpool.Run(ctx, n, workers, func(i int) {
		if err := fn(i); err != nil {
			once.Do(func() { first = err; cancel() })
		}
	})
	return first
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
