// Package nav builds the navigation tree that mirrors contentDir.
//
// The tree is built once per build (stage 3) and never modified after.
// Per-page views with Active/InPath flags are made by ForPage, which
// copies only the nodes on the path to the page and shares the rest.
package nav

import (
	"cmp"
	"math"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Pinta365/tryck/internal/content"
	"github.com/Pinta365/tryck/pkg/plugin"
)

// Node is one entry in the navigation tree.
type Node = plugin.NavNode

// sortKey holds the ordering inputs that are not part of Node.
type sortKey struct {
	weight int // 0 = unset
	prefix int // numeric filename prefix, or math.MaxInt for none
}

// Build creates the tree for pages. siteTitle labels the root when there
// is no root index page.
func Build(pages []*content.Page, siteTitle string) *Node {
	root := &Node{Title: siteTitle, Section: true}
	dirs := map[string]*Node{".": root}
	keys := map[*Node]sortKey{root: {prefix: math.MaxInt}}

	var dirNode func(dir string) *Node
	dirNode = func(dir string) *Node {
		if n, ok := dirs[dir]; ok {
			return n
		}
		parent := dirNode(path.Dir(dir))
		seg := path.Base(dir)
		n := &Node{Title: TitleCase(content.StripNumericPrefix(seg)), Section: true}
		keys[n] = sortKey{prefix: prefixOf(seg)}
		parent.Children = append(parent.Children, n)
		dirs[dir] = n
		return n
	}

	for _, p := range pages {
		dir := path.Dir(p.SourcePath)
		title := cmp.Or(p.Front.NavTitle, p.Title)
		if content.IsSectionIndex(p.SourcePath) {
			n := dirNode(dir)
			n.Title, n.URL, n.Draft = title, p.URL, p.Front.Draft
			keys[n] = sortKey{weight: p.Front.Weight, prefix: keys[n].prefix}
			continue
		}
		parent := dirNode(dir)
		base := strings.TrimSuffix(path.Base(p.SourcePath), path.Ext(p.SourcePath))
		n := &Node{Title: title, URL: p.URL, Draft: p.Front.Draft}
		keys[n] = sortKey{weight: p.Front.Weight, prefix: prefixOf(base)}
		parent.Children = append(parent.Children, n)
	}
	sortTree(root, keys)
	return root
}

func prefixOf(seg string) int {
	if n := content.NumericPrefix(seg); n >= 0 {
		return n
	}
	return math.MaxInt
}

// sortTree orders siblings by weight (unset last), then numeric prefix
// (none last), then title case-insensitively, then URL for stability.
func sortTree(n *Node, keys map[*Node]sortKey) {
	slices.SortStableFunc(n.Children, func(a, b *Node) int {
		ka, kb := keys[a], keys[b]
		return cmp.Or(
			cmp.Compare(weightKey(ka.weight), weightKey(kb.weight)),
			cmp.Compare(ka.prefix, kb.prefix),
			cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)),
			cmp.Compare(a.URL, b.URL),
		)
	})
	for _, c := range n.Children {
		sortTree(c, keys)
	}
}

func weightKey(w int) int {
	if w == 0 {
		return math.MaxInt
	}
	return w
}

// Walk returns the URLs of all pages in depth-first order: a section's
// own page comes before its children. Prev/next links follow it.
func Walk(root *Node) []string {
	var urls []string
	var walk func(*Node)
	walk = func(n *Node) {
		if n.URL != "" {
			urls = append(urls, n.URL)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return urls
}

// ForPage returns a view of root with Active set on the node for url and
// InPath on it and its ancestors. Nodes off that path are shared with
// root. If url is not in the tree, root is returned unchanged.
func ForPage(root *Node, url string) *Node {
	var path []int // child indexes from root to the page
	var find func(n *Node) bool
	find = func(n *Node) bool {
		if n.URL == url {
			return true
		}
		for i, c := range n.Children {
			path = append(path, i)
			if find(c) {
				return true
			}
			path = path[:len(path)-1]
		}
		return false
	}
	if url == "" || !find(root) {
		return root
	}

	view := *root
	view.InPath = true
	cur := &view
	for _, i := range path {
		child := *cur.Children[i]
		child.InPath = true
		cur.Children = slices.Clone(cur.Children)
		cur.Children[i] = &child
		cur = &child
	}
	cur.Active = true
	return &view
}

// TitleCase turns a folder name like "user-guide" into "User Guide".
func TitleCase(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		words[i] = string(unicode.ToUpper(r)) + w[size:]
	}
	return strings.Join(words, " ")
}
