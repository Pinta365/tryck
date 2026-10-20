package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Pinta365/tryck/pkg/tryck"
)

// runIn runs the CLI with dir as the working directory.
func runIn(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	var out, errOut bytes.Buffer
	code = tryck.Run(args, &out, &errOut, &Hello{})
	return code, out.String(), errOut.String()
}

func TestHelloPlugin(t *testing.T) {
	tests := []struct {
		name   string
		config string // replaces tryck.config.yaml if non-empty
		want   string // substring of dist/hello.txt
	}{
		{name: "default greeting"},
		{
			name:   "configured greeting",
			config: "site:\n  title: Hello\n  baseUrl: https://example.com/\nplugins:\n  hello:\n    greeting: Hej\n",
			want:   "Hej from",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if code, _, stderr := runIn(t, dir, "init", "."); code != 0 {
				t.Fatalf("init: exit code %d\nstderr: %s", code, stderr)
			}
			if tt.config != "" {
				if err := os.WriteFile(filepath.Join(dir, "tryck.config.yaml"), []byte(tt.config), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			code, _, stderr := runIn(t, dir, "build")
			if code != 0 {
				t.Fatalf("build: exit code %d\nstderr: %s", code, stderr)
			}
			// The starter config writes to ./dist.
			b, err := os.ReadFile(filepath.Join(dir, "dist", "hello.txt"))
			if err != nil {
				t.Fatal(err)
			}
			want := "Hello from"
			if tt.want != "" {
				want = tt.want
			}
			if !strings.Contains(string(b), want) || !strings.Contains(string(b), "pages") {
				t.Errorf("hello.txt = %q, want substring %q and a page count", b, want)
			}
		})
	}
}
