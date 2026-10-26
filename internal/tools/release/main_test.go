package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func read(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func repo(t *testing.T) string {
	return writeTree(t, map[string]string{
		releasesDir + "/_index.md":                           "---\ntitle: Release notes\n---\n\nNewest first.\n\n- [v0.10.0](v0.10.0.md): ten\n- [v0.9.0](v0.9.0.md): nine\n",
		releasesDir + "/v0.9.0.md":                           "---\ntitle: v0.9.0\nweight: 1\ndescription: Nine.\n---\n",
		releasesDir + "/v0.10.0.md":                          "---\ntitle: v0.10.0\nweight: 7\ndescription: Ten.\n---\n",
		"docs/content/01-getting-started/01-installation.md": "This prints `tryck <version>`, for example `tryck v0.10.0`. More.\n",
		"docs/content/06-deploying.md":                       "env:\n  TRYCK_VERSION: v0.10.0\n",
		"examples/docs-starter/.github/workflows/pages.yml":  "env:\n  TRYCK_VERSION: v0.10.0\n",
	})
}

func TestPrepare(t *testing.T) {
	root := repo(t)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	// The first run writes the notes to fill in.
	if _, err := prepare(root, "v0.11.0", "eleven things", now); !errors.Is(err, errNotesCreated) {
		t.Fatalf("first run: %v, want errNotesCreated", err)
	}
	notes := read(t, root, releasesDir+"/v0.11.0.md")
	if !strings.Contains(notes, "title: v0.11.0\ndate: 2026-11-02\nlayout: post\nweight: 1\n") {
		t.Errorf("skeleton:\n%s", notes)
	}
	if _, err := prepare(root, "v0.11.0", "eleven things", now); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("unwritten notes: %v", err)
	}
	notes = strings.Replace(notes, placeholder, "- New things.", 1)
	if err := os.WriteFile(filepath.Join(root, releasesDir, "v0.11.0.md"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(root, "v0.11.0", "eleven things", now); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("empty description: %v", err)
	}
	notes = strings.Replace(notes, `description: ""`, "description: Eleven.", 1)
	if err := os.WriteFile(filepath.Join(root, releasesDir, "v0.11.0.md"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := prepare(root, "v0.11.0", "eleven things", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 6 { // the new notes already have weight 1
		t.Errorf("changed %d files, want 6: %v", len(changed), changed)
	}
	// Versions order numerically: v0.10.0 is newer than v0.9.0.
	for name, weight := range map[string]string{"v0.11.0": "weight: 1\n", "v0.10.0": "weight: 2\n", "v0.9.0": "weight: 3\n"} {
		if got := read(t, root, releasesDir+"/"+name+".md"); !strings.Contains(got, weight) {
			t.Errorf("%s: want %q in\n%s", name, weight, got)
		}
	}
	if got := read(t, root, releasesDir+"/_index.md"); !strings.Contains(got, "Newest first.\n\n- [v0.11.0](v0.11.0.md): eleven things\n- [v0.10.0]") {
		t.Errorf("index:\n%s", got)
	}
	if got := read(t, root, "docs/content/01-getting-started/01-installation.md"); got != "This prints `tryck <version>`, for example `tryck v0.11.0`. More.\n" {
		t.Errorf("installation: %q", got)
	}
	for _, f := range []string{"docs/content/06-deploying.md", "examples/docs-starter/.github/workflows/pages.yml"} {
		if got := read(t, root, f); got != "env:\n  TRYCK_VERSION: v0.11.0\n" {
			t.Errorf("%s: %q", f, got)
		}
	}

	// Running again changes nothing.
	if changed, err := prepare(root, "v0.11.0", "eleven things", now); err != nil || len(changed) != 0 {
		t.Errorf("second run changed %v, %v", changed, err)
	}
}

func TestPrepareRefuses(t *testing.T) {
	root := repo(t)
	now := time.Now()
	for _, tt := range []struct{ version, summary, want string }{
		{"0.11.0", "x", "must look like v1.2.3"},
		{"v0.11", "x", "must look like v1.2.3"},
		{"v0.11.0", " ", "summary"},
		{"v0.9.0", "x", "older than v0.10.0"},
	} {
		if _, err := prepare(root, tt.version, tt.summary, now); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("prepare(%q, %q) = %v, want an error with %q", tt.version, tt.summary, err, tt.want)
		}
	}

	// A file that no longer names a version is an error, not a silent skip.
	if err := os.WriteFile(filepath.Join(root, "docs/content/06-deploying.md"), []byte("moved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, releasesDir, "v0.11.0.md"), []byte("---\ntitle: v0.11.0\nweight: 1\ndescription: Eleven.\n---\nNotes.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(root, "v0.11.0", "eleven", now); err == nil || !strings.Contains(err.Error(), "06-deploying.md no longer names a version") {
		t.Errorf("moved version: %v", err)
	}
}

func TestPrepareFromUnreleased(t *testing.T) {
	root := repo(t)
	unreleased := filepath.Join(root, filepath.FromSlash(unreleasedPath))
	entries := "- **Math.** Formulas render at build time.\n- **Fix.** Something broken works."
	if err := os.WriteFile(unreleased, []byte(strings.Replace(unreleasedTemplate, unreleasedGuide, unreleasedGuide+"\n\n"+entries, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	if _, err := prepare(root, "v0.11.0", "math", now); !errors.Is(err, errNotesGathered) {
		t.Fatalf("first run: %v, want errNotesGathered", err)
	}
	notes := read(t, root, releasesDir+"/v0.11.0.md")
	if !strings.HasSuffix(notes, "---\n\n"+entries+"\n") || strings.Contains(notes, placeholder) || strings.Contains(notes, "draft") {
		t.Errorf("notes from unreleased.md:\n%s", notes)
	}
	if got := read(t, root, unreleasedPath); got != unreleasedTemplate {
		t.Errorf("unreleased.md not emptied:\n%s", got)
	}
	// The description is still to be written.
	if _, err := prepare(root, "v0.11.0", "math", now); err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("second run without a description: %v", err)
	}

	// An empty unreleased.md gives the usual empty notes.
	root = repo(t)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(unreleasedPath)), []byte(unreleasedTemplate), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(root, "v0.11.0", "math", now); !errors.Is(err, errNotesCreated) {
		t.Fatalf("empty unreleased.md: %v, want errNotesCreated", err)
	}
	if !strings.Contains(read(t, root, releasesDir+"/v0.11.0.md"), placeholder) {
		t.Error("empty unreleased.md: notes have no placeholder")
	}
}
