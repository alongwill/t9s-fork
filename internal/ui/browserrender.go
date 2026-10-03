package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	paneActiveBorder   = lipgloss.NewStyle().Foreground(colorCyan)
	paneInactiveBorder = lipgloss.NewStyle().Foreground(colorDimGray)
	hitStyle           = lipgloss.NewStyle().Background(colorYellow).Foreground(colorBg)
	hitCurStyle        = lipgloss.NewStyle().Background(colorOrange).Foreground(colorBg).Bold(true)
	yamlKeyStyle       = lipgloss.NewStyle().Foreground(colorCyan)
	pathSelStyle       = lipgloss.NewStyle().Foreground(colorCyan).Bold(true) // selected row in an inactive pane
)

// renderBrowser draws the Miller columns: the visible slice of the pane
// stack left to right, every line padded to the terminal width.
func (app App) renderBrowser(height int) string {
	layout := app.browserLayout()
	if len(layout) == 0 {
		return ""
	}
	stack := app.browser.stack
	extra := app.nextStepRows()
	paneH := height - extra
	cols := make([][]string, len(layout))
	for i, box := range layout {
		cols[i] = app.renderPane(stack[box.idx], box.w, paneH, box.idx == len(stack)-1)
	}
	var sb strings.Builder
	for r := 0; r < paneH; r++ {
		var line strings.Builder
		for _, c := range cols {
			line.WriteString(c[r])
		}
		sb.WriteString(app.fillLine(line.String()))
		sb.WriteByte('\n')
	}
	if extra > 0 {
		sb.WriteString(app.fillLine(app.nextStepLine()))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// renderPane returns exactly h lines, each exactly w cells wide.
func (app App) renderPane(p pane, w, h int, active bool) []string {
	border := paneInactiveBorder
	if active {
		border = paneActiveBorder
	}
	iw := max(0, w-2)
	inner := max(0, h-2)

	var body []string
	switch p.kind {
	case paneCategories:
		body = app.categoryLines(p, iw, inner, active)
	case paneTypes:
		body = app.typeLines(p, iw, inner, active)
	case paneInstances:
		body = app.instanceLines(p, iw, inner, active)
	case paneYAML:
		body = app.yamlLines(p, iw, inner)
	case paneDescribe:
		body = app.describeLines(p, iw, inner)
	case paneAliases:
		body = app.paletteLines(p, iw, inner)
	case paneCompare:
		body = app.compareLines(p, iw, inner, active)
	case paneDiff:
		body = app.diffPaneLines(p, iw, inner)
	}

	lines := make([]string, 0, h)
	title := cutWidth(" "+app.paneTitle(p)+" ", iw)
	titleStyled := titleStyle.Render(title)
	if !active {
		titleStyled = dimStyle.Render(title)
	}
	rule := strings.Repeat("─", max(0, iw-lipgloss.Width(title)))
	lines = append(lines, border.Render("┌")+titleStyled+border.Render(rule+"┐"))
	for i := 0; i < inner; i++ {
		row := strings.Repeat(" ", iw)
		if i < len(body) {
			row = padRight(body[i], iw)
		}
		lines = append(lines, border.Render("│")+row+border.Render("│"))
	}
	lines = append(lines, border.Render("└"+strings.Repeat("─", iw)+"┘"))
	return lines[:min(len(lines), h)]
}

func (app App) paneTitle(p pane) string {
	switch p.kind {
	case paneTypes:
		return fmt.Sprintf("%s (%d)", p.title, len(app.browser.typeEntries(p.category, p.filter)))
	case paneAliases:
		return app.palettePaneTitle(p)
	case paneCompare:
		return p.title + "  (* = browser node)"
	case paneDiff:
		return p.title + "  (- browser node, + other)"
	case paneInstances:
		if p.loading {
			return p.title
		}
		return fmt.Sprintf("%s (%d)", p.title, len(filterInstances(p.items, p.filter)))
	}
	return p.title
}

// rowStyle picks the style for a list row.
func rowStyle(selected, active, dim bool) lipgloss.Style {
	switch {
	case selected && active:
		return selectedStyle
	case selected:
		return pathSelStyle
	case dim:
		return dimStyle
	}
	return lipgloss.NewStyle()
}

func marker(selected bool) string {
	if selected {
		return "▶ "
	}
	return "  "
}

// rowLR lays out `marker left … right` in exactly iw cells (plain text).
func rowLR(selected bool, left, right string, iw int) string {
	avail := max(0, iw-2)
	rw := lipgloss.Width(right)
	left = cutWidth(left, max(1, avail-rw-1))
	gap := max(1, avail-lipgloss.Width(left)-rw)
	return fit(marker(selected)+left+strings.Repeat(" ", gap)+right, iw)
}

func messageLines(iw, inner int, style lipgloss.Style, text string) []string {
	var out []string
	for _, c := range wordWrap(text, max(1, iw-2)) {
		if len(out) >= inner {
			break
		}
		out = append(out, style.Render(fit("  "+c, iw)))
	}
	return out
}

// wordWrap breaks s at spaces to at most w cells per line; words longer than
// w are split hard.
func wordWrap(s string, w int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		for lipgloss.Width(word) > w {
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			chunks := wrapChunks(word, w)
			lines = append(lines, chunks[:len(chunks)-1]...)
			word = chunks[len(chunks)-1]
		}
		switch {
		case cur == "":
			cur = word
		case lipgloss.Width(cur)+1+lipgloss.Width(word) <= w:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" || len(lines) == 0 {
		lines = append(lines, cur)
	}
	return lines
}

func (app App) categoryLines(p pane, iw, inner int, active bool) []string {
	b := app.browser
	switch {
	case b.defsLoading:
		return messageLines(iw, inner, dimStyle, "loading resource definitions…")
	case b.defsErr != "":
		return messageLines(iw, inner, errStyle, b.defsErr)
	}
	rows := b.categoryRows(p.filter)
	if len(rows) == 0 {
		return messageLines(iw, inner, dimStyle, "(none)")
	}
	start := clampScrollStart(p.scroll, p.cur, len(rows), inner)
	var out []string
	for i := start; i < len(rows) && i < start+inner; i++ {
		r := rows[i]
		present := "?"
		if r.counted {
			present = fmt.Sprint(r.present)
		}
		dim := r.counted && r.present == 0
		text := rowLR(i == p.cur, r.label, fmt.Sprintf("%s/%d", present, r.known), iw)
		out = append(out, rowStyle(i == p.cur, active, dim).Render(text))
	}
	return out
}

func (app App) typeLines(p pane, iw, inner int, active bool) []string {
	b := app.browser
	entries := b.typeEntries(p.category, p.filter)
	vis := b.typeVisual(p.category, p.filter)
	if len(vis) == 0 {
		return messageLines(iw, inner, dimStyle, "(none)")
	}
	cur := 0
	if p.cur > 0 {
		cur = visualIndex(vis, p.cur)
	}
	start := clampScrollStart(p.scroll, cur, len(vis), inner)
	avail := max(0, iw-2)
	const countW, aliasW = 5, 12
	showAlias := avail >= 30
	var out []string
	for i := start; i < len(vis) && i < start+inner; i++ {
		v := vis[i]
		switch {
		case v.header != "":
			out = append(out, colHeaderStyle.Render(fit("  "+v.header, iw)))
			continue
		case v.note != "":
			out = append(out, dimStyle.Render(fit("    "+v.note, iw)))
			continue
		}
		e := entries[v.sel]
		selected := v.sel == p.cur
		var cnt string
		var dim bool
		alias := ""
		if e.config {
			cnt, dim = b.configCell(e.ck.Kind)
		} else {
			cnt, dim = b.typeCell(e.def)
			if len(e.def.Aliases) > 0 {
				alias = e.def.Aliases[0]
			}
		}
		var text string
		if showAlias && e.config { // config rows have no alias: the name gets that width
			text = fit(marker(selected)+fit(e.name(), avail-countW-1)+" "+padLeft(cnt, countW), iw)
		} else if showAlias {
			nameW := avail - aliasW - countW - 2
			text = marker(selected) + fit(e.name(), nameW) + " " + fit(alias, aliasW) + " " + padLeft(cnt, countW)
			text = fit(text, iw)
		} else {
			text = rowLR(selected, e.name(), cnt, iw)
		}
		out = append(out, rowStyle(selected, active, dim).Render(text))
	}
	return out
}

func padLeft(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return s
	}
	return strings.Repeat(" ", w-sw) + s
}

// instanceLines draws `ID NAMESPACE VERSION PHASE`. Columns are dropped
// (NAMESPACE, then PHASE, then VER) so the ID keeps at least 14 cells.
func (app App) instanceLines(p pane, iw, inner int, active bool) []string {
	avail := max(0, iw-2)
	type column struct {
		name string
		w    int
		get  func(i int) string
	}
	items := filterInstances(p.items, p.filter)
	cols := []column{
		{"NAMESPACE", 12, func(i int) string { return items[i].Namespace }},
		{"VER", 4, func(i int) string { return items[i].Version }},
		{"PHASE", 7, func(i int) string { return items[i].Phase }},
	}
	idName := "ID"
	if p.cfgKind != "" { // config documents have a name only
		cols, idName = nil, "NAME"
	}
	used := func() int {
		n := 0
		for _, c := range cols {
			n += c.w + 1
		}
		return n
	}
	for _, drop := range []string{"NAMESPACE", "PHASE", "VER"} {
		if avail-used() >= 14 {
			break
		}
		for i, c := range cols {
			if c.name == drop {
				cols = append(cols[:i:i], cols[i+1:]...)
				break
			}
		}
	}
	idW := max(1, avail-used())

	header := "  " + fit(idName, idW)
	for _, c := range cols {
		header += " " + fit(c.name, c.w)
	}
	out := []string{colHeaderStyle.Render(fit(header, iw))}
	rows := max(0, inner-1)

	switch {
	case p.loading:
		return append(out, messageLines(iw, rows, dimStyle, "loading…")...)
	case p.err != "":
		return append(out, messageLines(iw, rows, errStyle, p.err)...)
	case len(items) == 0:
		return append(out, messageLines(iw, rows, dimStyle, "(none)")...)
	}
	start := clampScrollStart(p.scroll, p.cur, len(items), rows)
	for i := start; i < len(items) && i < start+rows; i++ {
		text := marker(i == p.cur) + fit(items[i].ID, idW)
		for _, c := range cols {
			text += " " + fit(c.get(i), c.w)
		}
		st := rowStyle(i == p.cur, active, false)
		if app.browser.flashing(items[i].ID) {
			st = hitStyle
		}
		out = append(out, st.Render(fit(text, iw)))
	}
	return out
}

func (app App) yamlLines(p pane, iw, inner int) []string {
	switch {
	case p.loading:
		return messageLines(iw, inner, dimStyle, "loading…")
	case p.err != "":
		return messageLines(iw, inner, errStyle, p.err)
	}
	b := app.browser
	vl := yamlVisual(p.yaml, iw, b.wrap)
	if len(vl) == 0 {
		return messageLines(iw, inner, dimStyle, "(empty)")
	}
	start := clamp(p.scroll, 0, max(0, len(vl)-inner))

	var find *regexp.Regexp
	hits := map[int]bool{}
	curHit := -1
	if b.find != "" {
		find = newMatcher(b.find, false).re
		for _, h := range b.findHits {
			hits[h] = true
		}
		if b.findIdx < len(b.findHits) {
			curHit = b.findHits[b.findIdx]
		}
	}
	var out []string
	for i := start; i < len(vl) && i < start+inner; i++ {
		l := vl[i]
		hs := hitStyle
		if l.logical == curHit {
			hs = hitCurStyle
		}
		out = append(out, colorYAMLLine(fit(l.text, iw), find, hs))
	}
	return out
}
