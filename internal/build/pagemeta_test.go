package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeadingAnchors(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{"content/a.md": "# Title\n\n## Install & go {#inst}\n\n### Sub\n"}))
	a := string(site.Outputs["a/index.html"])
	for _, want := range []string{
		`<h1 id="title">Title</h1>`,
		`<h2 id="inst">Install &amp; go<a class="heading-anchor" href="#inst" aria-label="Link to this section">#</a></h2>`,
		`<h3 id="sub">Sub<a class="heading-anchor" href="#sub" aria-label="Link to this section">#</a></h3>`,
	} {
		if !strings.Contains(a, want) {
			t.Errorf("missing %s in:\n%s", want, a)
		}
	}
	// The table of contents and the search index use the heading text only.
	if idx := string(site.Outputs["search-index.json"]); strings.Contains(idx, "#") {
		t.Errorf("anchors leaked into the search index: %s", idx)
	}

	off := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml": "site:\n  baseUrl: https://example.com/\ndefaults:\n  headingAnchors: false\n",
		"content/a.md":      "# Title\n\n## Install\n",
	}))
	if strings.Contains(string(off.Outputs["a/index.html"]), "heading-anchor") {
		t.Error("defaults.headingAnchors: false must turn anchors off")
	}
}

func TestEditURL(t *testing.T) {
	site := mustBuild(t, writeSite(t, map[string]string{
		"tryck.config.yaml":      "site:\n  baseUrl: https://example.com/\ntheme:\n  editUrl: https://github.com/you/repo/edit/main/docs\nversions:\n  current: v2\n  older:\n    - name: v1\n",
		"content/guide/a.md":     "# A\n",
		"versions/v1/guide/a.md": "# Old A\n",
	}))
	for path, want := range map[string]string{
		"guide/a/index.html":    `<a class="edit-link" href="https://github.com/you/repo/edit/main/docs/content/guide/a.md">Edit this page</a>`,
		"v1/guide/a/index.html": `<a class="edit-link" href="https://github.com/you/repo/edit/main/docs/versions/v1/guide/a.md">Edit this page</a>`,
	} {
		if !strings.Contains(string(site.Outputs[path]), want) {
			t.Errorf("%s: missing %s", path, want)
		}
	}
	plain := mustBuild(t, writeSite(t, map[string]string{"content/a.md": "# A\n"}))
	if strings.Contains(string(plain.Outputs["a/index.html"]), "page-meta") {
		t.Error("without editUrl or lastUpdated there is no page footer")
	}
}

func TestLastUpdatedFromGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	cfg := writeSite(t, map[string]string{
		"tryck.config.yaml":   "site:\n  baseUrl: https://example.com/\nbuild:\n  lastUpdated: true\n",
		"content/old.md":      "# Old\n",
		"content/new.md":      "# New\n",
		"content/Översikt.md": "# Översikt\n",
	})
	git := func(env []string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", cfg.Root}, args...)...)
		cmd.Env = append(cmd.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	id := []string{"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com"}
	git(nil, "init", "-q")
	git(append(id, "GIT_COMMITTER_DATE=2026-01-02T10:00:00Z", "GIT_AUTHOR_DATE=2026-01-02T10:00:00Z"), "add", "-A")
	git(append(id, "GIT_COMMITTER_DATE=2026-01-02T10:00:00Z", "GIT_AUTHOR_DATE=2026-01-02T10:00:00Z"), "commit", "-q", "-m", "first")
	if err := writeFile(cfg.Root, "content/new.md", "# New, changed\n"); err != nil {
		t.Fatal(err)
	}
	git(append(id, "GIT_COMMITTER_DATE=2026-03-04T10:00:00Z", "GIT_AUTHOR_DATE=2026-03-04T10:00:00Z"), "commit", "-q", "-am", "second")

	site := mustBuild(t, cfg)
	for path, want := range map[string]string{
		"old/index.html":      `Last updated <time datetime="2026-01-02">2026-01-02</time>`,
		"new/index.html":      `Last updated <time datetime="2026-03-04">2026-03-04</time>`,
		"Översikt/index.html": `Last updated <time datetime="2026-01-02">2026-01-02</time>`,
	} {
		if !strings.Contains(string(site.Outputs[path]), want) {
			t.Errorf("%s: missing %s", path, want)
		}
	}
	if sm := string(site.Outputs["sitemap.xml"]); !strings.Contains(sm, "<lastmod>2026-03-04</lastmod>") {
		t.Errorf("the sitemap should use the commit date:\n%s", sm)
	}
}

func writeFile(root, rel, body string) error {
	return os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(body), 0o644)
}
