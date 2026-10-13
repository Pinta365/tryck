package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		config     string // written to tryck.config.yaml if non-empty
		args       []string
		wantCode   int
		wantStdout string // substring
		wantStderr string // substring
	}{
		{name: "no args", args: nil, wantCode: 2, wantStderr: "Usage:"},
		{name: "help", args: []string{"help"}, wantCode: 0, wantStdout: "Usage:"},
		{name: "version", args: []string{"version"}, wantCode: 0, wantStdout: "tryck dev\n"},
		{name: "unknown command", args: []string{"serve"}, wantCode: 2, wantStderr: `unknown command "serve"`},
		{name: "build without config", args: []string{"build"}, wantCode: 0, wantStdout: "Built 1 pages, copied 0 files, 0 warnings, 0 errors"},
		{name: "build bad flag", args: []string{"build", "--nope"}, wantCode: 2, wantStderr: "flag provided but not defined"},
		{name: "build extra arg", args: []string{"build", "site"}, wantCode: 2, wantStderr: `unexpected argument "site"`},
		{name: "build -h", args: []string{"build", "-h"}, wantCode: 0, wantStderr: "-base-url"},
		{name: "build missing explicit config", args: []string{"build", "--config", "other.yaml"}, wantCode: 2, wantStderr: "other.yaml: error[config.read]: config file not found"},
		{
			name:       "build unknown key warns",
			config:     "site:\n  title: x\nbuild:\n  assetsDir: ./a\n",
			args:       []string{"build"},
			wantCode:   0,
			wantStdout: "1 warnings, 0 errors",
			wantStderr: `tryck.config.yaml:4:3: warning[config.unknownKey]: unknown config key "build.assetsDir"`,
		},
		{
			name:       "build invalid yaml",
			config:     "site:\n  title: [\n",
			args:       []string{"build"},
			wantCode:   2,
			wantStderr: "tryck.config.yaml:2: error[config.yaml]",
		},
		{
			name:       "build invalid value",
			config:     "build:\n  workers: -2\n",
			args:       []string{"build"},
			wantCode:   2,
			wantStderr: "error[config.invalid]: build.workers",
		},
		{
			name:       "flag fixes invalid file value",
			config:     "build:\n  workers: -2\n",
			args:       []string{"build", "--workers", "2"},
			wantCode:   0,
			wantStdout: "Built 1 pages",
		},
		{name: "build invalid base-url flag", args: []string{"build", "--base-url", "ftp://x"}, wantCode: 2, wantStderr: "site.baseUrl"},
		{name: "build verbose logs config", args: []string{"build", "--verbose"}, wantCode: 0, wantStderr: "config loaded"},
		{
			name:       "info diagnostics need verbose",
			config:     "site:\n  title: x\n",
			args:       []string{"build", "--verbose"},
			wantCode:   0,
			wantStderr: "info[heading.duplicateId]",
		},
		{name: "dev", args: []string{"dev", "--port", "9000"}, wantCode: 0, wantStderr: "not implemented"},
		{name: "dev bad port", args: []string{"dev", "--port", "0"}, wantCode: 2, wantStderr: "invalid port"},
		{name: "init with dir", args: []string{"init", "mysite"}, wantCode: 0, wantStderr: "not implemented"},
		{name: "init too many args", args: []string{"init", "a", "b"}, wantCode: 2, wantStderr: `unexpected argument "b"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "content"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "content", "a.md"), []byte("## A\n\n## A\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tt.config != "" {
				if err := os.WriteFile(filepath.Join(dir, "tryck.config.yaml"), []byte(tt.config), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			code, stdout, stderr := runIn(t, dir, tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\nstderr: %s", code, tt.wantCode, stderr)
			}
			if !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want substring %q", stdout, tt.wantStdout)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stderr = %q, want substring %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestInfoHiddenWithoutVerbose(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "content", "a.md"), []byte("## A\n\n## A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runIn(t, dir, "build")
	if code != 0 || strings.Contains(stderr, "info[") {
		t.Errorf("code %d, stderr %q: info diagnostics should be hidden", code, stderr)
	}
}
