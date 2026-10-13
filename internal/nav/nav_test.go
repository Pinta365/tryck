package nav

import (
	"slices"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/internal/content"
)

func page(rel, title string, weight int) *content.Page {
	p := &content.Page{Title: title}
	p.SourcePath = rel
	p.URL = content.URLFor(rel, "")
	p.Front.Weight = weight
	return p
}

// dump renders the tree as indented "Title url" lines.
func dump(n *Node) string {
	var b strings.Builder
	var walk func(n *Node, depth int)
	walk = func(n *Node, depth int) {
		b.WriteString(strings.Repeat("  ", depth) + n.Title)
		if n.URL != "" {
			b.WriteString(" " + n.URL)
		}
		if n.Active {
			b.WriteString(" [active]")
		} else if n.InPath {
			b.WriteString(" [path]")
		}
		b.WriteString("\n")
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(n, 0)
	return b.String()
}

func TestBuildOrdering(t *testing.T) {
	pages := []*content.Page{
		page("index.md", "Home", 0),
		page("zeta.md", "Zeta", 0),
		page("alpha.md", "alpha", 0),
		page("Beta.md", "Beta", 0),
		page("02-second.md", "Second", 0),
		page("01-first.md", "First", 0),
		page("heavy.md", "Heavy", 20),
		page("light.md", "Light", 10),
		page("03-guide/_index.md", "The Guide", 5),
		page("03-guide/02-b.md", "B", 0),
		page("03-guide/01-a.md", "A", 0),
		page("ref/api.md", "API", 0),
		page("ref/deep/cli.md", "CLI", 0),
	}
	got := dump(Build(pages, "Site"))
	want := `Home /
  The Guide /guide/
    A /guide/a/
    B /guide/b/
  Light /light/
  Heavy /heavy/
  First /first/
  Second /second/
  alpha /alpha/
  Beta /Beta/
  Ref
    API /ref/api/
    Deep
      CLI /ref/deep/cli/
  Zeta /zeta/
`
	if got != want {
		t.Errorf("tree:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildLabels(t *testing.T) {
	p := page("user-guide/install.md", "Installing the engine", 0)
	p.Front.NavTitle = "Install"
	root := Build([]*content.Page{p}, "Site")
	if root.Title != "Site" || root.URL != "" || !root.Section {
		t.Errorf("root = %+v", root)
	}
	folder := root.Children[0]
	if folder.Title != "User Guide" || folder.URL != "" || !folder.Section {
		t.Errorf("folder without _index = %+v", folder)
	}
	if leaf := folder.Children[0]; leaf.Title != "Install" || leaf.Section {
		t.Errorf("navTitle not used: %+v", leaf)
	}
}

func TestWalk(t *testing.T) {
	root := Build([]*content.Page{
		page("index.md", "Home", 0),
		page("02-b.md", "B", 0),
		page("01-guide/_index.md", "Guide", 0),
		page("01-guide/01-x.md", "X", 0),
		page("03-ref/y.md", "Y", 0),
	}, "Site")
	want := []string{"/", "/guide/", "/guide/x/", "/b/", "/ref/y/"}
	if got := Walk(root); !slices.Equal(got, want) {
		t.Errorf("Walk = %v, want %v", got, want)
	}
}

func TestForPage(t *testing.T) {
	root := Build([]*content.Page{
		page("index.md", "Home", 0),
		page("01-guide/_index.md", "Guide", 0),
		page("01-guide/01-x.md", "X", 0),
		page("02-other.md", "Other", 0),
	}, "Site")
	before := dump(root)

	view := ForPage(root, "/guide/x/")
	want := `Home / [path]
  Guide /guide/ [path]
    X /guide/x/ [active]
  Other /other/
`
	if got := dump(view); got != want {
		t.Errorf("view:\n%s\nwant:\n%s", got, want)
	}
	if dump(root) != before {
		t.Error("ForPage modified the shared tree")
	}
	if view.Children[1] != root.Children[1] {
		t.Error("nodes off the active path should be shared")
	}
	if ForPage(root, "/missing/") != root {
		t.Error("unknown URL should return the shared root")
	}
	if got := dump(ForPage(root, "/")); !strings.HasPrefix(got, "Home / [active]\n  Guide /guide/\n") {
		t.Errorf("root page view:\n%s", got)
	}
}

func TestTitleCase(t *testing.T) {
	for in, want := range map[string]string{"user-guide": "User Guide", "api_ref": "Api Ref", "ärende": "Ärende", "": ""} {
		if got := TitleCase(in); got != want {
			t.Errorf("TitleCase(%q) = %q, want %q", in, got, want)
		}
	}
}
