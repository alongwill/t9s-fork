package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffAddStyle  = okStyle
	diffDelStyle  = errStyle
	diffHunkStyle = infoStyle
	diffHeadStyle = lipgloss.NewStyle().Bold(true)
)

// compareLines draws `NODE ROLE PRESENT VERSION SAME?`. ROLE is dropped on
// narrow panes so the node name keeps at least 14 cells.
func (app App) compareLines(p pane, iw, inner int, active bool) []string {
	avail := max(0, iw-2)
	const presentW, versionW, sameW, roleW = 7, 8, 5, 12
	showRole := avail-(presentW+versionW+sameW+roleW+4) >= 14
	used := presentW + versionW + sameW + 3
	if showRole {
		used += roleW + 1
	}
	nodeW := max(1, avail-used)

	header := "  " + fit("NODE", nodeW)
	if showRole {
		header += " " + fit("ROLE", roleW)
	}
	header += " " + fit("PRESENT", presentW) + " " + fit("VERSION", versionW) + " " + fit("SAME?", sameW)
	out := []string{colHeaderStyle.Render(fit(header, iw))}
	rows := max(0, inner-1)

	cv := p.cmp
	if len(cv.rows) == 0 {
		return append(out, messageLines(iw, rows, dimStyle, "(no nodes)")...)
	}
	start := clampScrollStart(p.scroll, p.cur, len(cv.rows), rows)
	for i := start; i < len(cv.rows) && i < start+rows; i++ {
		r := cv.rows[i]
		name := r.node.Hostname
		if name == "" {
			name = r.node.IP
		}
		if r.node.IP == cv.base {
			name += " *"
		}
		version := r.version
		if version == "" {
			version = "-"
		}
		same := cv.sameText(r)
		text := marker(i == p.cur) + fit(name, nodeW)
		if showRole {
			text += " " + fit(r.node.Role, roleW)
		}
		text += " " + fit(r.presentText(), presentW) + " " + fit(version, versionW) + " " + fit(same, sameW)
		st := rowStyle(i == p.cur, active, r.state == cmpAbsent || r.state == cmpLocked)
		if i != p.cur && same == "no" {
			st = warnStyle
		}
		out = append(out, st.Render(fit(text, iw)))
	}
	return out
}

// diffPaneLines draws the unified diff with coloured +/- lines.
func (app App) diffPaneLines(p pane, iw, inner int) []string {
	if len(p.diff) == 0 {
		return messageLines(iw, inner, dimStyle, "(empty)")
	}
	start := clamp(p.scroll, 0, max(0, len(p.diff)-inner))
	var out []string
	for i := start; i < len(p.diff) && i < start+inner; i++ {
		l := p.diff[i]
		var text string
		var st lipgloss.Style
		switch l.op {
		case 'h':
			text, st = l.text, diffHeadStyle
		case '@':
			text, st = l.text, diffHunkStyle
		case '+':
			text, st = "+"+strings.ReplaceAll(l.text, "\t", "  "), diffAddStyle
		case '-':
			text, st = "-"+strings.ReplaceAll(l.text, "\t", "  "), diffDelStyle
		default:
			text, st = " "+strings.ReplaceAll(l.text, "\t", "  "), lipgloss.NewStyle()
		}
		out = append(out, st.Render(fit(text, iw)))
	}
	return out
}
