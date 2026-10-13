package content

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"

	"github.com/Pinta365/tryck/internal/diag"
)

func TestSplitFrontMatter(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		wantFM       string
		wantBody     string
		wantBodyLine int
		wantErr      bool
	}{
		{name: "none", src: "# Hi\n", wantBody: "# Hi\n", wantBodyLine: 1},
		{name: "empty file", src: "", wantBody: "", wantBodyLine: 1},
		{name: "basic", src: "---\ntitle: A\n---\n# Hi\n", wantFM: "title: A\n", wantBody: "# Hi\n", wantBodyLine: 4},
		{name: "multi-line", src: "---\na: 1\nb: 2\n---\nbody", wantFM: "a: 1\nb: 2\n", wantBody: "body", wantBodyLine: 5},
		{name: "empty block", src: "---\n---\nbody\n", wantFM: "", wantBody: "body\n", wantBodyLine: 3},
		{name: "crlf", src: "---\r\ntitle: A\r\n---\r\nbody\r\n", wantFM: "title: A\r\n", wantBody: "body\r\n", wantBodyLine: 4},
		{name: "bom", src: "\xef\xbb\xbf---\ntitle: A\n---\nbody", wantFM: "title: A\n", wantBody: "body", wantBodyLine: 4},
		{name: "trailing spaces on fence", src: "--- \ntitle: A\n---\t\nbody", wantFM: "title: A\n", wantBody: "body", wantBodyLine: 4},
		{name: "closing fence at EOF", src: "---\ntitle: A\n---", wantFM: "title: A\n", wantBody: "", wantBodyLine: 4},
		{name: "not at top", src: "\n---\ntitle: A\n---\n", wantBody: "\n---\ntitle: A\n---\n", wantBodyLine: 1},
		{name: "longer rule is not a fence", src: "----\ntitle: A\n", wantBody: "----\ntitle: A\n", wantBodyLine: 1},
		{name: "unclosed", src: "---\ntitle: A\n# Body\n", wantErr: true, wantBody: "---\ntitle: A\n# Body\n", wantBodyLine: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm, body, line, err := SplitFrontMatter([]byte(tt.src))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if string(fm) != tt.wantFM || string(body) != tt.wantBody || line != tt.wantBodyLine {
				t.Errorf("got fm=%q body=%q line=%d, want fm=%q body=%q line=%d", fm, body, line, tt.wantFM, tt.wantBody, tt.wantBodyLine)
			}
		})
	}
}

func TestParseFrontMatter(t *testing.T) {
	fm := "title: Setup\nweight: 3\ndate: 2026-10-13\ntoc: false\naliases: [/old/]\nauthor: Anders\ncustom:\n  n: 1\n"
	front, ds := ParseFrontMatter("content/a.md", []byte(fm))
	if len(ds) != 0 {
		t.Fatalf("diagnostics %v", ds)
	}
	if front.Title != "Setup" || front.Weight != 3 || front.TOC == nil || *front.TOC || !slices.Equal(front.Aliases, []string{"/old/"}) {
		t.Errorf("typed fields: %+v", front)
	}
	if !front.Date.Equal(time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date = %v", front.Date)
	}
	if front.Params["author"] != "Anders" || front.Params["custom"] == nil {
		t.Errorf("params = %v", front.Params)
	}
	if _, ok := front.Params["title"]; ok {
		t.Error("known key leaked into Params")
	}
}

func TestParseFrontMatterErrorLines(t *testing.T) {
	tests := []struct {
		name     string
		fm       string
		wantLine int // file line: front matter line + 1 for the opening ---
	}{
		{"syntax", "title: A\nweight: [\n", 3},
		{"type", "title: A\nweight: heavy\n", 3},
		{"type on first line", "draft: maybe\n", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ds := ParseFrontMatter("content/a.md", []byte(tt.fm))
			if len(ds) == 0 {
				t.Fatal("no diagnostics")
			}
			d := ds[0]
			if d.Severity != diag.Error || d.Code != "frontmatter.yaml" || d.File != "content/a.md" || d.Line != tt.wantLine {
				t.Errorf("got %s, want frontmatter.yaml error at line %d", diag.Format(d), tt.wantLine)
			}
		})
	}
}

func TestURLFor(t *testing.T) {
	tests := []struct{ rel, slug, want string }{
		{"index.md", "", "/"},
		{"_index.md", "", "/"},
		{"index.md", "ignored", "/"},
		{"docs/setup.md", "", "/docs/setup/"},
		{"docs/_index.md", "", "/docs/"},
		{"docs/index.md", "", "/docs/"},
		{"01-intro.md", "", "/intro/"},
		{"02-guide/01-install.md", "", "/guide/install/"},
		{"02-guide/_index.md", "", "/guide/"},
		{"docs/setup.md", "getting-started", "/docs/getting-started/"},
		{"docs/setup.md", "/padded/", "/docs/padded/"},
		{"docs/_index.md", "documentation", "/documentation/"},
		{"404.md", "", "/404/"},
		{"2026-10-13-post.md", "", "/2026-10-13-post/"},
		{"01-.md", "", "/01-/"},
	}
	for _, tt := range tests {
		if got := URLFor(tt.rel, tt.slug); got != tt.want {
			t.Errorf("URLFor(%q, %q) = %q, want %q", tt.rel, tt.slug, got, tt.want)
		}
	}
}

func TestStripNumericPrefix(t *testing.T) {
	for in, want := range map[string]string{"01-intro": "intro", "intro": "intro", "10-": "10-", "1a-x": "1a-x", "001-a-b": "a-b"} {
		if got := StripNumericPrefix(in); got != want {
			t.Errorf("StripNumericPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNumericPrefix(t *testing.T) {
	for in, want := range map[string]int{"01-intro": 1, "120-x": 120, "intro": -1, "10-": -1, "2026-10-13-post": -1, "99999999999999999999-x": math.MaxInt32} {
		if got := NumericPrefix(in); got != want {
			t.Errorf("NumericPrefix(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestOutputAndAssetPaths(t *testing.T) {
	if got := OutputPath("/"); got != "index.html" {
		t.Errorf("OutputPath(/) = %q", got)
	}
	if got := OutputPath("/docs/setup/"); got != "docs/setup/index.html" {
		t.Errorf("OutputPath = %q", got)
	}
	if got := AssetPath("01-docs/02-img/03-shot.png"); got != "docs/img/03-shot.png" {
		t.Errorf("AssetPath = %q", got)
	}
}

func TestTitles(t *testing.T) {
	for rel, want := range map[string]string{
		"docs/01-getting-started.md": "Getting started",
		"docs/_index.md":             "Docs",
		"02-user_guide/index.md":     "User guide",
		"index.md":                   "",
	} {
		if got := TitleFromPath(rel); got != want {
			t.Errorf("TitleFromPath(%q) = %q, want %q", rel, got, want)
		}
	}

	src := []byte("Intro text\n\n## Sub\n\n# The `tryck` *engine*\n\n# Second\n")
	doc := goldmark.New().Parser().Parse(text.NewReader(src))
	if got := FirstH1(doc, src); got != "The tryck engine" {
		t.Errorf("FirstH1 = %q", got)
	}
	src = []byte("## Only h2\n")
	if got := FirstH1(goldmark.New().Parser().Parse(text.NewReader(src)), src); got != "" {
		t.Errorf("FirstH1 without h1 = %q", got)
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"index.md", "docs/01-setup.md", "docs/img/shot.png", "docs/README.MD", ".hidden.md", ".git/config", "docs/.draft/x.md"} {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"docs/01-setup.md", "docs/README.MD", "index.md"}; !slices.Equal(src.Pages, want) {
		t.Errorf("pages = %v, want %v", src.Pages, want)
	}
	if len(src.Assets) != 1 || src.Assets[0].OutPath != "docs/img/shot.png" {
		t.Errorf("assets = %+v", src.Assets)
	}

	src, err = Discover(filepath.Join(dir, "missing"))
	if err != nil || len(src.Pages) != 0 {
		t.Errorf("missing dir: %v, %v", src, err)
	}
}
