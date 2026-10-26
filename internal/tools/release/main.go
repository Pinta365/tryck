// Command release prepares a tryck release in the working tree: the
// release notes page, the release index, and the version that the docs
// and the starter workflow download.
//
//	go run ./internal/tools/release v0.9.0 "a short summary for the index"
//
// Between releases, changes are noted in docs/content/09-releases/
// unreleased.md, a draft page. The first run creates
// docs/content/09-releases/v0.9.0.md from those notes (or empty, if there
// are none) and empties unreleased.md, for you to edit and give a
// description. The next run checks the notes, orders every release page by
// version, adds the release to the index and points the docs and the
// starter at the new version. Running it again changes nothing. It never
// commits, tags or pushes; it prints the commands for that.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	releasesDir = "docs/content/09-releases"
	placeholder = "<!-- Write the release notes here. -->"
)

// bumps are the files that name the latest release, and the pattern
// around the version in each.
var bumps = []struct {
	file string
	re   *regexp.Regexp
}{
	{"docs/content/01-getting-started/01-installation.md", regexp.MustCompile("(for example `tryck )(v[0-9]+\\.[0-9]+\\.[0-9]+)(`)")},
	{"docs/content/06-deploying.md", regexp.MustCompile(`(TRYCK_VERSION: )(v[0-9]+\.[0-9]+\.[0-9]+)()`)},
	{"examples/docs-starter/.github/workflows/pages.yml", regexp.MustCompile(`(TRYCK_VERSION: )(v[0-9]+\.[0-9]+\.[0-9]+)()`)},
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, `usage: go run ./internal/tools/release <version> "<summary for the release index>"`)
		os.Exit(2)
	}
	version, summary := os.Args[1], os.Args[2]
	changed, err := prepare(".", version, summary, time.Now())
	switch {
	case errors.Is(err, errNotesGathered):
		fmt.Printf("Created %s from %s, which is empty again. Edit the notes, write the description, then run this again.\n", notesPath(version), unreleasedPath)
		return
	case errors.Is(err, errNotesCreated):
		fmt.Printf("Created %s. Write the notes and the description, then run this again.\n", notesPath(version))
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
	if len(changed) == 0 {
		fmt.Printf("Everything already points at %s.\n", version)
	} else {
		fmt.Printf("Updated for %s:\n", version)
		for _, f := range changed {
			fmt.Println("  " + f)
		}
	}
	fmt.Printf(`
Review the changes, then:

  git add -A
  git commit -m "Release notes for %[1]s, and point the starter and docs at it"
  git push origin main
  git tag -a %[1]s -m %[1]s
  git push origin %[1]s
`, version)
}

var (
	errNotesCreated  = errors.New("release notes created")
	errNotesGathered = errors.New("release notes created from unreleased.md")
)

// unreleasedPath gathers the changes on main between releases. It is a
// draft, so only tryck dev shows it.
const unreleasedPath = releasesDir + "/unreleased.md"

const unreleasedGuide = "<!-- Add a line for each change that lands on main, as in the release notes. The release tool moves them into the next release's notes. -->"

const unreleasedTemplate = "---\ntitle: Unreleased\ndraft: true\ndescription: Changes on main since the last release.\n---\n\n" + unreleasedGuide + "\n"

// unreleasedEntries returns the body of unreleased.md without its front
// matter and guide comment, or "" if nothing was added.
func unreleasedEntries(s string) string {
	if strings.HasPrefix(s, "---\n") {
		if i := strings.Index(s[4:], "\n---\n"); i >= 0 {
			s = s[4+i+5:]
		}
	}
	return strings.TrimSpace(strings.Replace(s, unreleasedGuide, "", 1))
}

func notesPath(version string) string {
	return filepath.ToSlash(filepath.Join(releasesDir, version+".md"))
}

// prepare updates the tree at root for version and returns the files it
// changed, relative to root. If the notes do not exist it creates them
// and returns errNotesCreated.
func prepare(root, version, summary string, now time.Time) ([]string, error) {
	v, ok := parseVersion(version)
	if !ok {
		return nil, fmt.Errorf("version %q must look like v1.2.3", version)
	}
	if strings.TrimSpace(summary) == "" {
		return nil, errors.New("the summary for the release index must not be empty")
	}
	notes := filepath.Join(root, notesPath(version))
	if _, err := os.Stat(notes); errors.Is(err, os.ErrNotExist) {
		// The notes start from what was gathered in unreleased.md, which
		// is emptied for the next release.
		text := placeholder + "\n\nUpgrading: nothing to change."
		unreleased := filepath.Join(root, filepath.FromSlash(unreleasedPath))
		gathered := false
		if data, err := os.ReadFile(unreleased); err == nil {
			if entries := unreleasedEntries(string(data)); entries != "" {
				text, gathered = entries, true
			}
		}
		skeleton := fmt.Sprintf("---\ntitle: %s\ndate: %s\nlayout: post\nweight: 1\ndescription: \"\"\n---\n\n%s\n",
			version, now.Format("2006-01-02"), text)
		if err := os.WriteFile(notes, []byte(skeleton), 0o644); err != nil {
			return nil, err
		}
		if gathered {
			if err := os.WriteFile(unreleased, []byte(unreleasedTemplate), 0o644); err != nil {
				return nil, err
			}
			return nil, errNotesGathered
		}
		return nil, errNotesCreated
	}
	body, err := os.ReadFile(notes)
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(body), placeholder) {
		return nil, fmt.Errorf("%s still has the placeholder; write the notes first", notesPath(version))
	}
	if m := regexp.MustCompile(`(?m)^description:\s*(""|'')?\s*$`).FindIndex(body); m != nil {
		return nil, fmt.Errorf("%s needs a description", notesPath(version))
	}

	// Every release page, newest first, gets weight 1, 2, ...
	entries, err := os.ReadDir(filepath.Join(root, releasesDir))
	if err != nil {
		return nil, err
	}
	type release struct {
		name string
		v    [3]int
	}
	var releases []release
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if rv, ok := parseVersion(name); ok && strings.HasSuffix(e.Name(), ".md") {
			releases = append(releases, release{name, rv})
		}
	}
	slices.SortFunc(releases, func(a, b release) int { return slices.Compare(b.v[:], a.v[:]) })
	if newest := releases[0]; newest.v != v {
		return nil, fmt.Errorf("%s is older than %s, the newest release notes", version, newest.name)
	}
	var changed []string
	weightRE := regexp.MustCompile(`(?m)^weight: *[0-9]+ *$`)
	for i, r := range releases {
		rel := notesPath(r.name)
		ok, err := edit(root, rel, func(s string) (string, error) {
			if !weightRE.MatchString(s) {
				return "", fmt.Errorf("%s has no weight in its front matter", rel)
			}
			return weightRE.ReplaceAllString(s, "weight: "+strconv.Itoa(i+1)), nil
		})
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, rel)
		}
	}

	// The index lists releases newest first.
	index := filepath.ToSlash(filepath.Join(releasesDir, "_index.md"))
	ok, err = edit(root, index, func(s string) (string, error) {
		if strings.Contains(s, "- ["+version+"]("+version+".md)") {
			return s, nil
		}
		line := fmt.Sprintf("- [%s](%s.md): %s\n", version, version, strings.TrimSpace(summary))
		i := strings.Index(s, "\n- [v")
		if i < 0 {
			return "", fmt.Errorf("%s has no list of releases to add to", index)
		}
		return s[:i+1] + line + s[i+1:], nil
	})
	if err != nil {
		return nil, err
	}
	if ok {
		changed = append(changed, index)
	}

	for _, b := range bumps {
		ok, err := edit(root, b.file, func(s string) (string, error) {
			if !b.re.MatchString(s) {
				return "", fmt.Errorf("%s no longer names a version where expected; update the release tool", b.file)
			}
			return b.re.ReplaceAllString(s, "${1}"+version+"${3}"), nil
		})
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, b.file)
		}
	}
	return changed, nil
}

// edit applies fn to the file rel under root and writes it back if it
// changed, reporting whether it did.
func edit(root, rel string, fn func(string) (string, error)) (bool, error) {
	p := filepath.Join(root, filepath.FromSlash(rel))
	data, err := os.ReadFile(p)
	if err != nil {
		return false, err
	}
	out, err := fn(string(data))
	if err != nil || out == string(data) {
		return false, err
	}
	return true, os.WriteFile(p, []byte(out), 0o644)
}

func parseVersion(s string) ([3]int, bool) {
	m := regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`).FindStringSubmatch(s)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}
