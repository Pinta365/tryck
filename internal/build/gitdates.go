package build

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gitDates holds the date of the last commit that touched each file, for
// build.lastUpdated. Files outside a git repository, or never committed,
// have no date.
type gitDates struct {
	top   string               // the repository's top level, symlinks resolved
	dates map[string]time.Time // path relative to top -> commit date
}

// readGitDates runs one git log over dirs. It returns nil, without an
// error, when git is not installed or root is not in a repository.
func readGitDates(root string, dirs []string) *gitDates {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil
	}
	top, err := filepath.EvalSymlinks(strings.TrimSpace(string(out)))
	if err != nil {
		return nil
	}
	args := []string{"-C", top, "-c", "core.quotepath=off", "log", "--format=%x00%cI", "--name-only", "--no-renames", "--"}
	for _, d := range dirs {
		if r, err := relTo(top, d); err == nil {
			args = append(args, r)
		}
	}
	out, err = exec.Command("git", args...).Output()
	if err != nil {
		return nil
	}
	g := &gitDates{top: top, dates: map[string]time.Time{}}
	// Commits come newest first, so a file's first appearance is its last change.
	for _, commit := range bytes.Split(out, []byte{0}) {
		lines := strings.Split(strings.TrimSpace(string(commit)), "\n")
		date, err := time.Parse(time.RFC3339, lines[0])
		if err != nil {
			continue
		}
		for _, f := range lines[1:] {
			if f = strings.TrimSpace(f); f != "" {
				if _, seen := g.dates[f]; !seen {
					g.dates[f] = date
				}
			}
		}
	}
	return g
}

// of returns the last commit date of the file at path, or the zero time.
func (g *gitDates) of(path string) time.Time {
	if g == nil {
		return time.Time{}
	}
	r, err := relTo(g.top, path)
	if err != nil {
		return time.Time{}
	}
	return g.dates[r]
}

// relTo returns path relative to top, with symlinks resolved and forward
// slashes, as git prints it.
func relTo(top, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	r, err := filepath.Rel(top, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(r), nil
}
