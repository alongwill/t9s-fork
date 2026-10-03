package ui

import (
	"fmt"
	"strings"
)

// minNextStepHeight is the terminal height below which the next-step line is
// hidden: the panes need the rows more.
const minNextStepHeight = 20

// nextStepRows is 1 when the next-step line is on screen, else 0. paneInnerRows
// and renderBrowser both read it so the height budget stays in one place.
func (app App) nextStepRows() int {
	if !isBrowserState(app.state) || len(app.browser.stack) == 0 || app.height < minNextStepHeight {
		return 0
	}
	return 1
}

// nextStep is the dim line under the panes: what the selected row is and which
// keys are worth pressing next.
func (app App) nextStep() string {
	p, ok := app.browser.top()
	if !ok {
		return ""
	}
	b := app.browser
	var parts []string
	switch p.kind {
	case paneCategories:
		rows := b.categoryRows(p.filter)
		if p.cur < len(rows) {
			parts = append(parts, "↵ open "+rows[p.cur].label)
		}
		parts = append(parts, "/ filter", "ctrl+a every type", ": commands")
	case paneTypes:
		parts = app.typeNextStep(p)
	case paneInstances:
		parts = []string{"↵ view YAML", "d what is this", "p related", "c compare on all nodes"}
		if app.hasWatch() && p.cfgKind == "" {
			parts = append(parts, "W watch")
		}
	case paneYAML:
		parts = []string{"/ find", "w wrap", "f full screen", "c compare", "esc back"}
	case paneDescribe:
		parts = []string{"↑↓ scroll", "y YAML", "d back"}
		if len(selectableLines(describeVisual(app.describeRows(p), app.yamlInnerWidth()))) > 0 {
			parts = []string{"↑↓ select a type", "↵ jump to it", "y YAML", "d back"}
		}
	case paneAliases:
		parts = []string{"↵ jump to the type", "esc clear, then close"}
	case paneCompare:
		parts = []string{"↵ diff against the browser's node", "esc back"}
	case paneDiff:
		parts = []string{"↑↓ scroll", "esc back"}
	case paneRelated:
		parts = app.relatedNextStep(p)
	case paneNetwork:
		parts = app.networkNextStep(p)
	case paneDisks:
		parts = app.disksNextStep(p)
	}
	return strings.Join(parts, " · ")
}

func (app App) typeNextStep(p pane) []string {
	b := app.browser
	rows := b.typeEntries(p.category, p.filter)
	if p.cur >= len(rows) {
		return []string{"/ filter", "ctrl+a every type"}
	}
	e := rows[p.cur]
	what := "d what is this"
	if e.config {
		switch n := len(b.docsOfKind(e.ck.Kind)); {
		case b.cfgState == cfgLoading || b.cfgState == cfgNone:
			return []string{"loading the machine config…", what}
		case n == 0:
			return []string{"not in this node's config", what}
		case n == 1:
			return []string{"↵ open the document", what, "c compare"}
		default:
			return []string{fmt.Sprintf("↵ %d documents", n), what, "c compare (on a document)"}
		}
	}
	n, counted := b.counts[e.def.Type]
	switch {
	case !counted:
		return []string{"counting…", what}
	case n == countLocked:
		return []string{"requires os:admin", what}
	case n == countError:
		return []string{"could not list it · ctrl+r retries", what}
	case n == 0:
		return []string{"not on this node", what}
	case n == 1:
		parts := []string{"↵ open it", what, "c compare"}
		if app.hasWatch() {
			parts = append(parts, "W watch")
		}
		return parts
	}
	parts := []string{fmt.Sprintf("↵ %d instances", n), what, "c compare (on an instance)"}
	if app.hasWatch() {
		parts = append(parts, "W watch (on an instance)")
	}
	return parts
}

// nextStepLine renders the line to exactly the terminal width.
func (app App) nextStepLine() string {
	return dimStyle.Render(fit("  "+app.nextStep(), app.width))
}
