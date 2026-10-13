package build

import "testing"

func TestLinkResolver(t *testing.T) {
	r := &linkResolver{
		baseURL: "https://u.github.io/repo/",
		pages: map[string]string{
			"index.md":             "/",
			"01-guide/_index.md":   "/guide/",
			"01-guide/02-setup.md": "/guide/setup/",
			"01-guide/install.md":  "/guide/get-started/", // slug
			"ref/api.md":           "/ref/api/",
		},
		assets: map[string]string{"01-guide/img/shot.png": "guide/img/shot.png"},
	}
	tests := []struct {
		source, dest, want string
	}{
		// unchanged
		{"index.md", "https://example.org/x", "https://example.org/x"},
		{"index.md", "mailto:a@b.c", "mailto:a@b.c"},
		{"index.md", "#top", "#top"},
		{"index.md", "?q=1", "?q=1"},
		{"01-guide/02-setup.md", "../../outside.md", "../../outside.md"},
		// .md links, relative to the source file, honoring slugs
		{"index.md", "01-guide/02-setup.md", "/repo/guide/setup/"},
		{"01-guide/02-setup.md", "install.md#step-2", "/repo/guide/get-started/#step-2"},
		{"01-guide/02-setup.md", "../ref/api.md", "/repo/ref/api/"},
		{"01-guide/02-setup.md", "../index.md", "/repo/"},
		{"01-guide/02-setup.md", "./_index.md", "/repo/guide/"},
		{"ref/api.md", "/01-guide/install.md", "/repo/guide/get-started/"},
		{"ref/api.md", "missing.md", "/repo/ref/missing/"},
		// names without .md, directories, assets
		{"01-guide/02-setup.md", "install", "/repo/guide/get-started/"},
		{"index.md", "01-guide/", "/repo/guide/"},
		{"index.md", "01-guide", "/repo/guide/"},
		{"index.md", "02-other/", "/repo/other/"},
		{"01-guide/02-setup.md", "img/shot.png", "/repo/guide/img/shot.png"},
		{"01-guide/02-setup.md", "img/missing.png", "/repo/guide/img/missing.png"},
		// root-absolute site paths get the base path
		{"index.md", "/guide/setup/", "/repo/guide/setup/"},
		{"index.md", "/img/logo.png?v=2", "/repo/img/logo.png?v=2"},
		{"index.md", "/a%20b/#x%20y", "/repo/a%20b/#x%20y"},
	}
	for _, tt := range tests {
		got, _ := r.resolve(tt.source, tt.dest)
		if got != tt.want {
			t.Errorf("resolve(%q, %q) = %q, want %q", tt.source, tt.dest, got, tt.want)
		}
	}
}
