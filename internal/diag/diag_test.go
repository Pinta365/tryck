package diag

import (
	"sync"
	"testing"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			name: "full position",
			d:    Diagnostic{Severity: Error, File: "content/docs/setup.md", Line: 12, Col: 5, Code: "link.broken", Message: "/docs/instal/ does not exist (did you mean /docs/install/?)"},
			want: "content/docs/setup.md:12:5: error[link.broken]: /docs/instal/ does not exist (did you mean /docs/install/?)",
		},
		{
			name: "line without column",
			d:    Diagnostic{Severity: Warning, File: "tryck.config.yaml", Line: 3, Code: "config.unknownKey", Message: "unknown key"},
			want: "tryck.config.yaml:3: warning[config.unknownKey]: unknown key",
		},
		{
			name: "file only",
			d:    Diagnostic{Severity: Info, File: "a.md", Code: "heading.duplicateId", Message: "m"},
			want: "a.md: info[heading.duplicateId]: m",
		},
		{
			name: "column ignored without line",
			d:    Diagnostic{Severity: Error, File: "a.md", Col: 4, Code: "x", Message: "m"},
			want: "a.md: error[x]: m",
		},
		{
			name: "no file",
			d:    Diagnostic{Severity: Error, Code: "config.read", Message: "m"},
			want: "error[config.read]: m",
		},
		{
			name: "no code",
			d:    Diagnostic{Severity: Error, File: "a.md", Line: 1, Col: 1, Message: "m"},
			want: "a.md:1:1: error: m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.d); got != tt.want {
				t.Errorf("Format() = %q, want %q", got, tt.want)
			}
			if got := tt.d.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSeverityString(t *testing.T) {
	for sev, want := range map[Severity]string{Info: "info", Warning: "warning", Error: "error", Severity(9): "severity(9)"} {
		if got := sev.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(sev), got, want)
		}
	}
}

func TestCollectorSortsAndCounts(t *testing.T) {
	var c Collector
	c.Warnf("b.md", 2, 1, "w", "second in b")
	c.Errorf("a.md", 10, 1, "e", "late in a")
	c.Infof("a.md", 2, 3, "i", "early in a")
	c.Warnf("a.md", 2, 3, "w", "same position, added later")

	got := c.All()
	wantMsgs := []string{"early in a", "same position, added later", "late in a", "second in b"}
	if len(got) != len(wantMsgs) {
		t.Fatalf("All() returned %d diagnostics, want %d", len(got), len(wantMsgs))
	}
	for i, m := range wantMsgs {
		if got[i].Message != m {
			t.Errorf("All()[%d].Message = %q, want %q", i, got[i].Message, m)
		}
	}
	if c.Len() != 4 || c.Count(Warning) != 2 || c.Count(Error) != 1 || c.Count(Info) != 1 {
		t.Errorf("counts: len=%d warn=%d err=%d info=%d", c.Len(), c.Count(Warning), c.Count(Error), c.Count(Info))
	}
	if !c.HasErrors() {
		t.Error("HasErrors() = false, want true")
	}
}

func TestCollectorNoErrors(t *testing.T) {
	var c Collector
	if c.HasErrors() {
		t.Error("empty collector reports errors")
	}
	c.Warnf("a.md", 1, 1, "w", "warn")
	if c.HasErrors() {
		t.Error("warning-only collector reports errors")
	}
	if HasErrors(nil) {
		t.Error("HasErrors(nil) = true")
	}
}

func TestCollectorAllReturnsCopy(t *testing.T) {
	var c Collector
	c.Errorf("a.md", 1, 1, "e", "m")
	got := c.All()
	got[0].Message = "changed"
	if c.All()[0].Message != "m" {
		t.Error("mutating All() result changed the collector")
	}
}

func TestCollectorConcurrent(t *testing.T) {
	var c Collector
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Warnf("p.md", i+1, 1, "w", "warning %d", i)
			_ = c.HasErrors()
		}()
	}
	wg.Wait()
	if c.Len() != 50 {
		t.Errorf("Len() = %d, want 50", c.Len())
	}
}
