package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/catalog"
	"github.com/florianspk/t9s/internal/talos"
)

// Describe pane (k9s `d`): static facts about a config kind or resource type.
// Talos v1.14 has no `explain` subcommand, so resource types show their
// ResourceDefinition fields only; config kinds show the catalogue text.

// descLine is one row of the describe pane. An empty label makes it a free paragraph.
type descLine struct{ label, text string }

const descLabelW = 18

// dline is one rendered line of the describe pane.
type dline struct{ label, text string }

// findConfigKind looks a kind up in the catalogue (plus the legacy document).
func findConfigKind(kind string) (catalog.ConfigKind, bool) {
	if kind == legacyKind.Kind {
		return legacyKind, true
	}
	for _, k := range catalog.ConfigKinds() {
		if k.Kind == kind {
			return k, true
		}
	}
	return catalog.ConfigKind{Kind: kind}, false
}

func (b browser) describeConfigKind(ck catalog.ConfigKind) []descLine {
	cat := catalog.ConfigCategoryFor(ck.Group)
	lines := []descLine{
		{"Kind", ck.Kind},
		{"Type", "machine config document"},
		{"Group", fmt.Sprintf("%s (%s)", catalog.Label(cat), ck.Group)},
		{"Since", ck.Since},
	}
	switch b.cfgState {
	case cfgLoaded:
		lines = append(lines, descLine{"In this node", fmt.Sprintf("%d document(s)", len(b.docsOfKind(ck.Kind)))})
	case cfgDenied:
		lines = append(lines, descLine{"In this node", "requires os:admin"})
	}
	if ck.Desc != "" {
		lines = append(lines, descLine{"", ""}, descLine{"", ck.Desc})
	}
	return lines
}

func (b browser) describeResource(d talos.ResourceDef) []descLine {
	sens := "no"
	if d.Sensitive {
		sens = "yes (requires os:admin)"
	}
	aliases := "-"
	if len(d.Aliases) > 0 {
		aliases = strings.Join(d.Aliases, ", ")
	}
	lines := []descLine{
		{"Type", d.Type},
		{"Display type", d.DisplayType},
		{"Aliases", aliases},
		{"Default namespace", d.DefaultNamespace},
		{"Sensitive", sens},
		{"Category", catalog.Label(catalog.CategoryFor(d))},
	}
	if n, ok := b.counts[d.Type]; ok && n >= 0 {
		lines = append(lines, descLine{"Instances", fmt.Sprint(n)})
	}
	return append(lines, descLine{"", ""},
		descLine{"", "Field documentation is not available: this Talos version has no explain subcommand."})
}

// describeVisual wraps the pane's lines to w cells.
func describeVisual(lines []descLine, w int) []dline {
	var out []dline
	for _, l := range lines {
		if l.label == "" {
			if l.text == "" {
				out = append(out, dline{})
				continue
			}
			for _, c := range wordWrap(l.text, max(1, w)) {
				out = append(out, dline{text: c})
			}
			continue
		}
		tw := max(1, w-descLabelW)
		for i, c := range wordWrap(l.text, tw) {
			if i == 0 {
				out = append(out, dline{label: l.label, text: c})
			} else {
				out = append(out, dline{text: c, label: "\x00"}) // continuation: indented, no label
			}
		}
	}
	return out
}

func (app App) describeLines(p pane, iw, inner int) []string {
	vl := describeVisual(p.desc, iw)
	start := clamp(p.scroll, 0, max(0, len(vl)-inner))
	var out []string
	for i := start; i < len(vl) && i < start+inner; i++ {
		l := vl[i]
		switch {
		case l.label == "":
			out = append(out, fit(l.text, iw))
		case l.label == "\x00":
			out = append(out, fit(strings.Repeat(" ", descLabelW)+l.text, iw))
		default:
			out = append(out, yamlKeyStyle.Render(fit(l.label, descLabelW))+fit(l.text, max(0, iw-descLabelW)))
		}
	}
	if len(out) == 0 {
		return []string{dimStyle.Render(fit("  (nothing to describe)", iw))}
	}
	return out
}

func (app App) paneDescribeLen(p pane) int {
	return len(describeVisual(p.desc, app.yamlInnerWidth()))
}

// openDescribe implements `d` on a types or instances pane.
func (app App) openDescribe() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok {
		return app, nil
	}
	b := app.browser
	var title string
	var lines []descLine
	switch p.kind {
	case paneTypes:
		rows := b.typeEntries(p.category, p.filter)
		if p.cur >= len(rows) {
			return app, nil
		}
		e := rows[p.cur]
		title = e.name()
		if e.config {
			lines = b.describeConfigKind(e.ck)
		} else {
			lines = b.describeResource(e.def)
		}
	case paneInstances:
		title = p.title
		if p.cfgKind != "" {
			ck, _ := findConfigKind(p.cfgKind)
			lines = b.describeConfigKind(ck)
		} else {
			lines = b.describeResource(p.def)
		}
	default:
		return app, nil
	}
	app.browser = b.push(pane{kind: paneDescribe, title: "Describe " + title, desc: lines})
	return app.syncBrowserState(), nil
}

// describeToYAML implements `y` in the describe pane: close it and open the
// row underneath, as Enter would.
func (app App) describeToYAML() (App, tea.Cmd) {
	app = app.popPane()
	return app.browserEnter()
}
