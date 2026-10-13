package build

import (
	"crypto/sha256"
	"sync"

	"github.com/yuin/goldmark/ast"

	"github.com/Pinta365/tryck/internal/markdown"
)

// Cache keeps parsed Markdown between builds, keyed by source path and
// the SHA-256 of the file. tryck dev uses one Cache for all rebuilds so
// unchanged pages skip goldmark parsing. A Cache is tied to one config:
// create a new one when the config changes.
//
// Cached ASTs are only modified in stage 2 of a build, and only by link
// rewriting, which re-applies from the original destinations each time.
// One build at a time may use a Cache.
type Cache struct {
	mu      sync.Mutex
	entries map[string]*parsed
	used    map[string]bool // source paths seen by the current build
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{entries: map[string]*parsed{}, used: map[string]bool{}}
}

// parsed is the cacheable result of parsing one page body.
type parsed struct {
	sum    [sha256.Size]byte
	doc    ast.Node
	issues []markdown.Issue
	h1     string
	links  []linkRef
}

// linkRef is a link or image with its destination as written.
type linkRef struct {
	node ast.Node // *ast.Link or *ast.Image
	dest string
}

func (c *Cache) get(path string, sum [sha256.Size]byte) *parsed {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.used[path] = true
	if e := c.entries[path]; e != nil && e.sum == sum {
		return e
	}
	return nil
}

func (c *Cache) put(path string, e *parsed) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries[path] = e
	c.mu.Unlock()
}

// prune drops entries for files the last build did not see.
func (c *Cache) prune() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for path := range c.entries {
		if !c.used[path] {
			delete(c.entries, path)
		}
	}
	c.used = map[string]bool{}
}

// Len returns the number of cached pages.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
