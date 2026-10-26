package devserver

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// watcher watches directories recursively and adds new subdirectories
// as they appear.
type watcher struct {
	fs *fsnotify.Watcher
}

func newWatcher() (*watcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &watcher{fs: w}, nil
}

// addTree watches dir and every directory below it. A missing dir is
// skipped; its parent is watched instead so its creation is noticed.
func (w *watcher) addTree(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		return w.add(parent)
	}
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // a directory vanished while walking
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		return w.add(p)
	})
}

func (w *watcher) add(dir string) error {
	err := w.fs.Add(dir)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// handleCreate starts watching a newly created directory.
func (w *watcher) handleCreate(ev fsnotify.Event) {
	if ev.Has(fsnotify.Create) {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			_ = w.addTree(ev.Name)
		}
	}
}

func (w *watcher) close() error { return w.fs.Close() }

// relevant reports whether an event can change the site. It drops
// editor temporary files, permission changes, and writes to a directory:
// Windows reports a directory as written whenever a file in it changes,
// and that file has its own event. A directory that is created, removed
// or renamed is still relevant.
func relevant(ev fsnotify.Event) bool {
	if ignored(ev.Name) || ev.Op == fsnotify.Chmod {
		return false
	}
	if ev.Op == fsnotify.Write {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			return false
		}
	}
	return true
}

// ignored reports editor temporary files and other noise.
func ignored(path string) bool {
	name := filepath.Base(path)
	return strings.HasSuffix(name, ".swp") || strings.HasSuffix(name, ".swx") ||
		strings.HasSuffix(name, "~") || strings.HasPrefix(name, ".#") ||
		name == "4913" || name == ".DS_Store"
}
