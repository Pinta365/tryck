package markdown

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"

	"github.com/Pinta365/tryck/pkg/plugin"
)

type directiveRenderer struct {
	custom map[string]plugin.DirectiveRenderer // by directive name
}

func (r directiveRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindAdmonition, renderAdmonition)
	reg.Register(KindDetails, renderDetails)
	reg.Register(KindTabs, renderTabs)
	reg.Register(KindTab, renderTab)
	reg.Register(KindGenericDirective, r.renderGeneric)
}

func renderAdmonition(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Admonition)
	if !entering {
		w.WriteString("</aside>\n")
		return ast.WalkContinue, nil
	}
	w.WriteString(`<aside`)
	writeAttrs(w, n, "admonition admonition-"+n.Variant)
	w.WriteString(">\n")
	if n.Title != "" {
		w.WriteString(`<p class="admonition-title">`)
		w.Write(util.EscapeHTML([]byte(n.Title)))
		w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

func renderDetails(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Details)
	if !entering {
		w.WriteString("</details>\n")
		return ast.WalkContinue, nil
	}
	w.WriteString("<details")
	writeAttrs(w, n, "details")
	w.WriteString(">\n<summary>")
	w.Write(util.EscapeHTML([]byte(cmp.Or(n.Title, "Details"))))
	w.WriteString("</summary>\n")
	return ast.WalkContinue, nil
}

func (r directiveRenderer) renderGeneric(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*GenericDirective)
	if pr, ok := r.custom[n.Name]; ok {
		d := plugin.Directive{Name: n.Name, Title: n.Title, Attrs: map[string]string{}}
		for _, a := range n.Attributes() {
			d.Attrs[string(a.Name)] = string(attrBytes(a.Value))
		}
		if err := pr.RenderDirective(w, d, entering); err != nil {
			return ast.WalkStop, fmt.Errorf(":::%s: %w", n.Name, err)
		}
		return ast.WalkContinue, nil
	}
	if !entering {
		w.WriteString("</div>\n")
		return ast.WalkContinue, nil
	}
	w.WriteString("<div")
	writeAttrs(w, n, "directive directive-"+n.Name)
	w.WriteString(">\n")
	if n.Title != "" {
		w.WriteString(`<p class="directive-title">`)
		w.Write(util.EscapeHTML([]byte(n.Title)))
		w.WriteString("</p>\n")
	}
	return ast.WalkContinue, nil
}

// Tabs render readably without JavaScript: the tab list is hidden and
// every panel is shown with its label. The default layout's script
// unhides the list and shows one panel at a time.
func renderTabs(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Tabs)
	if !entering {
		w.WriteString("</div>\n")
		return ast.WalkContinue, nil
	}
	w.WriteString("<div")
	if _, ok := n.AttributeString("id"); !ok {
		fmt.Fprintf(w, ` id="%s"`, tabsBase(n))
	}
	writeAttrs(w, n, "tabs")
	w.WriteString(">\n")
	return ast.WalkContinue, nil
}

func renderTab(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*Tab)
	if !entering {
		w.WriteString("</div>\n")
		return ast.WalkContinue, nil
	}
	parent, ok := n.Parent().(*Tabs)
	if !ok {
		return ast.WalkContinue, nil
	}
	base := tabsBase(parent)
	i := tabIndex(n)
	if i == 0 {
		writeTabList(w, parent, base)
	}
	fmt.Fprintf(w, `<div class="tab-panel" role="tabpanel" id="%s-panel-%d" aria-labelledby="%s-tab-%d">`+"\n", base, i, base, i)
	w.WriteString(`<p class="tab-label" role="heading" aria-level="4">`)
	w.Write(util.EscapeHTML([]byte(n.Label)))
	w.WriteString("</p>\n")
	return ast.WalkContinue, nil
}

func writeTabList(w util.BufWriter, tabs *Tabs, base string) {
	w.WriteString(`<div class="tab-list" role="tablist" hidden>` + "\n")
	i := 0
	for c := tabs.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*Tab)
		if !ok {
			continue
		}
		selected, tabindex := "true", ""
		if i > 0 {
			selected, tabindex = "false", ` tabindex="-1"`
		}
		fmt.Fprintf(w, `<button type="button" role="tab" id="%s-tab-%d" aria-controls="%s-panel-%d" aria-selected="%s"%s>`,
			base, i, base, i, selected, tabindex)
		w.Write(util.EscapeHTML([]byte(t.Label)))
		w.WriteString("</button>\n")
		i++
	}
	w.WriteString("</div>\n")
}

// tabsBase is the prefix for element IDs in a tab set: its {#id} or
// tabs-N.
func tabsBase(n *Tabs) string {
	if v, ok := n.AttributeString("id"); ok {
		return string(util.EscapeHTML(attrBytes(v)))
	}
	return "tabs-" + strconv.Itoa(n.Index)
}

// tabIndex is the position of t among its Tab siblings.
func tabIndex(t *Tab) int {
	i := 0
	for c := t.PreviousSibling(); c != nil; c = c.PreviousSibling() {
		if _, ok := c.(*Tab); ok {
			i++
		}
	}
	return i
}

// writeAttrs writes the allowed {attrs} of a directive, merging class
// with the directive's own classes. Event handlers and other attributes
// that could run script are not written; "group" becomes data-group.
func writeAttrs(w util.BufWriter, n ast.Node, class string) {
	if v, ok := n.AttributeString("class"); ok {
		class += " " + string(attrBytes(v))
	}
	fmt.Fprintf(w, ` class="%s"`, util.EscapeHTML([]byte(class)))
	for _, a := range n.Attributes() {
		name := string(a.Name)
		switch {
		case name == "class":
			continue
		case name == "group":
			name = "data-group"
		case name == "open":
			w.WriteString(" open")
			continue
		case name == "id" || name == "title" || name == "lang" || name == "dir":
		case strings.HasPrefix(name, "data-") && validAttrName(name):
		default:
			continue
		}
		fmt.Fprintf(w, ` %s="%s"`, name, util.EscapeHTML(attrBytes(a.Value)))
	}
}

func validAttrName(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func attrBytes(v any) []byte {
	switch v := v.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		return []byte(fmt.Sprint(v))
	}
}
