package build_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Pinta365/tryck/internal/build"
	"github.com/Pinta365/tryck/internal/config"
	"github.com/Pinta365/tryck/internal/diag"
	"github.com/Pinta365/tryck/internal/output"
)

// genSite writes a project with n pages spread over 10 sections, each
// page with headings, a code block, a table and links.
func genSite(tb testing.TB, n int) *config.Config {
	tb.Helper()
	root := tb.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			tb.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	write("tryck.config.yaml", "site:\n  title: Generated\n  baseUrl: https://example.com/gen/\n")
	write("content/index.md", "# Generated site\n")
	for s := range 10 {
		write(fmt.Sprintf("content/%02d-section-%d/_index.md", s, s), fmt.Sprintf("---\ntitle: Section %d\n---\nSection intro.\n", s))
	}
	for i := range n {
		write(fmt.Sprintf("content/%02d-section-%d/%03d-page-%d.md", i%10, i%10, i, i), fmt.Sprintf(`---
title: Page %d
weight: %d
description: Generated page number %d
---

Intro paragraph with **bold**, _italic_ and `+"`code`"+`.

## Install

| Key | Value |
| --- | ----- |
| n   | %d    |

`+"```go\nfunc main() { fmt.Println(%d) }\n```"+`

## Install

- [Next section](/section-%d/)
- [Home](/)

### Details

Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor.
`, i, i%7, i, i, i, (i+1)%10))
	}
	cfg, ds := config.Load(filepath.Join(root, config.DefaultPath), true)
	if len(ds) != 0 {
		tb.Fatalf("config: %v", ds)
	}
	return cfg
}

// TestParallelBuild builds a 500-page site with 8 workers (run with
// -race) and checks the result is identical to a single-worker build.
func TestParallelBuild(t *testing.T) {
	cfg := genSite(t, 500)

	cfg.Build.Workers = 8
	par, err := build.Build(context.Background(), cfg, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if par.HasErrors() {
		t.Fatalf("errors: %v", par.Diags[:min(5, len(par.Diags))])
	}
	if len(par.Pages) != 511 {
		t.Errorf("pages = %d, want 511", len(par.Pages))
	}

	cfg.Build.Workers = 1
	seq, err := build.Build(context.Background(), cfg, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(par.Diags, seq.Diags, func(a, b diag.Diagnostic) bool { return a == b }) {
		t.Error("diagnostics differ between 8 workers and 1 worker")
	}
	if !maps.EqualFunc(par.Outputs, seq.Outputs, func(a, b []byte) bool { return string(a) == string(b) }) {
		t.Error("outputs differ between 8 workers and 1 worker")
	}

	cfg.Build.Workers = 8
	cfg.Build.OutputDir = t.TempDir()
	st, err := output.Write(par)
	if err != nil {
		t.Fatal(err)
	}
	if st.Pages != len(par.Outputs) {
		t.Errorf("wrote %d files, want %d", st.Pages, len(par.Outputs))
	}
}

// BenchmarkBuild1000Pages tracks the build-time target: under 1 s for
// 1,000 pages (build plus writing to disk).
func BenchmarkBuild1000Pages(b *testing.B) {
	cfg := genSite(b, 1000)
	cfg.Build.OutputDir = b.TempDir()
	b.ResetTimer()
	for range b.N {
		site, err := build.Build(context.Background(), cfg, build.Options{})
		if err != nil {
			b.Fatal(err)
		}
		if site.HasErrors() {
			b.Fatal(site.Diags[0])
		}
		if _, err := output.Write(site); err != nil {
			b.Fatal(err)
		}
	}
}
