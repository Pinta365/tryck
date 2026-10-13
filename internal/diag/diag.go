// Package diag defines build diagnostics: a stable, file:line-addressed
// description of every problem found while loading or building a site.
package diag

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Severity orders diagnostics from least to most serious.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"
	default:
		return "severity(" + strconv.Itoa(int(s)) + ")"
	}
}

// Diagnostic is a single problem report.
type Diagnostic struct {
	Severity Severity
	File     string // relative to project root
	Line     int    // 1-based; 0 if unknown
	Col      int    // 1-based; 0 if unknown
	Code     string // stable id, e.g. "frontmatter.yaml", "link.broken"
	Message  string
}

// Format renders d as "file:line:col: severity[code]: message".
// The line and column are omitted when unknown, and the file part is
// omitted when the diagnostic is not tied to a file.
func Format(d Diagnostic) string {
	var b strings.Builder
	if d.File != "" {
		b.WriteString(d.File)
		if d.Line > 0 {
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(d.Line))
			if d.Col > 0 {
				b.WriteByte(':')
				b.WriteString(strconv.Itoa(d.Col))
			}
		}
		b.WriteString(": ")
	}
	b.WriteString(d.Severity.String())
	if d.Code != "" {
		b.WriteByte('[')
		b.WriteString(d.Code)
		b.WriteByte(']')
	}
	b.WriteString(": ")
	b.WriteString(d.Message)
	return b.String()
}

func (d Diagnostic) String() string { return Format(d) }

// Sort orders diagnostics by file, line and column. It is stable, so
// diagnostics at the same position keep their insertion order.
func Sort(ds []Diagnostic) {
	slices.SortStableFunc(ds, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
		)
	})
}

// HasErrors reports whether any diagnostic in ds is error-level.
func HasErrors(ds []Diagnostic) bool {
	return slices.ContainsFunc(ds, func(d Diagnostic) bool { return d.Severity >= Error })
}

// Collector gathers diagnostics. It is safe for concurrent use.
type Collector struct {
	mu    sync.Mutex
	diags []Diagnostic
}

// Add records diagnostics.
func (c *Collector) Add(ds ...Diagnostic) {
	c.mu.Lock()
	c.diags = append(c.diags, ds...)
	c.mu.Unlock()
}

// Errorf records an error-level diagnostic.
func (c *Collector) Errorf(file string, line, col int, code, format string, args ...any) {
	c.add(Error, file, line, col, code, format, args)
}

// Warnf records a warning-level diagnostic.
func (c *Collector) Warnf(file string, line, col int, code, format string, args ...any) {
	c.add(Warning, file, line, col, code, format, args)
}

// Infof records an info-level diagnostic.
func (c *Collector) Infof(file string, line, col int, code, format string, args ...any) {
	c.add(Info, file, line, col, code, format, args)
}

func (c *Collector) add(sev Severity, file string, line, col int, code, format string, args []any) {
	c.Add(Diagnostic{
		Severity: sev,
		File:     file,
		Line:     line,
		Col:      col,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
	})
}

// All returns a sorted copy of the collected diagnostics.
func (c *Collector) All() []Diagnostic {
	c.mu.Lock()
	out := slices.Clone(c.diags)
	c.mu.Unlock()
	Sort(out)
	return out
}

// Len returns the number of collected diagnostics.
func (c *Collector) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.diags)
}

// Count returns the number of diagnostics with the given severity.
func (c *Collector) Count(sev Severity) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, d := range c.diags {
		if d.Severity == sev {
			n++
		}
	}
	return n
}

// HasErrors reports whether any error-level diagnostic was collected.
func (c *Collector) HasErrors() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return HasErrors(c.diags)
}
