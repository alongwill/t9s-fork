package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (app App) handleHelpKey(msg tea.KeyMsg) (App, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc", "q", "?":
		app = app.goBack()
		return app, nil
	default:
		var cmd tea.Cmd
		app.helpVP, cmd = app.helpVP.Update(msg)
		return app, cmd
	}
}

// buildHelpContent renders the help of a default (read-only) app.
func buildHelpContent() string { return buildHelpContentFor(App{}) }

// buildHelpContentFor renders the help overlay. Keys that change a cluster are
// listed only in write mode (k9s ClearDanger).
func buildHelpContentFor(app App) string {
	k := keyStyle.Render
	d := dimStyle.Render
	h := titleStyle.Render

	section := func(title string, rows [][2]string) string {
		var sb strings.Builder
		sb.WriteString(h(title) + "\n")
		for _, r := range rows {
			sb.WriteString(fmt.Sprintf("  %-12s %s\n", k(r[0]), d(r[1])))
		}
		return sb.String()
	}

	var sb strings.Builder

	sb.WriteString(app.helpStatusSection(section))
	sb.WriteByte('\n')

	sb.WriteString(section("Global", [][2]string{
		{"?", "Toggle this help"},
		{"x", "Switch context"},
		{"/", "Search / filter list"},
		{"Esc", "Back / cancel"},
		{"q / ctrl+c", "Quit"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Node List", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"↵ / c", "Containers"},
		{"s", "Services"},
		{"l", "Log streams"},
		{"e", "Extensions (installed)"},
		{"C", "Extension catalog"},
		{"m", "Machine config"},
		{"d", "Dmesg stream"},
		{"t", "Metrics (CPU/RAM)"},
		{"p", "Processes"},
		{"a", "Resource browser"},
		{"ctrl+a", "All types palette (browser for the selected node)"},
		{":", "Command mode (:nodes :net :netview :addr :aliases :q :help)"},
		{"A", "Network addresses"},
		{"N", "Network view (tree from the NIC up, HTML diagram)"},
		{"i", "Disks: partition bars per disk"},
		{"H", "Cluster health"},
		{"r", "Refresh nodes"},
	}))
	if app.writeMode {
		sb.WriteString(section("Node List: write mode (--write)", [][2]string{
			{"R", "Reboot node"},
			{"S", "Shutdown node"},
			{"U", "Upgrade Talos"},
			{"K", "Upgrade Kubernetes"},
		}))
	}
	sb.WriteByte('\n')

	sb.WriteString(browserHelp(section))
	sb.WriteByte('\n')

	sb.WriteString(section("Learning (resource browser)", [][2]string{
		{"d", "Describe the selected type: what it is, its Ubuntu equivalent, what feeds it"},
		{"next-step line", "Dim line under the panes: the selected row and the keys worth pressing next"},
		{":tips off / on", "Silence or restore the tips in the status line (this session)"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Services", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"↵ / l", "Stream logs"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Log Streams", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"PgUp / PgDn", "Page up / down"},
		{"Home / End / g / G", "Top / bottom"},
		{"↵", "Stream selected logs"},
		{"/", "Filter streams"},
		{"r", "Reload streams"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Logs", [][2]string{
		{"↑↓ / j k", "Select line (scrolling up turns Autoscroll off)"},
		{"PgUp / PgDn", "Half page"},
		{"g / Home", "Top (Autoscroll off)"},
		{"G / End", "Bottom (Autoscroll on)"},
		{"s", "Toggle Autoscroll: off freezes the view, +N new counts what arrives"},
		{"f", "Toggle FullScreen (hide header, hints and footer)"},
		{"t", "Toggle Timestamps (~ marks arrival time, UTC)"},
		{"w", "Toggle Wrap"},
		{"/  n  N", "Find, next, previous"},
		{"Esc / q", "Clear find, leave FullScreen, then back"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Dmesg", [][2]string{
		{"↑↓", "Scroll"},
		{"g", "Go to top"},
		{"G", "Go to bottom"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	mcRows := [][2]string{
		{"↑↓", "Scroll"},
		{"g", "Go to top"},
		{"G", "Go to bottom"},
		{"Esc / q", "Back"},
	}
	if app.writeMode {
		mcRows = append(mcRows, [2]string{"e", "Edit and apply the machine config (write mode)"})
	}
	sb.WriteString(section("Machine Config / Health", mcRows))
	sb.WriteByte('\n')

	sb.WriteString(section("Containers", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"↵", "Container detail"},
		{"w", "Wrap"},
		{"r", "Refresh"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Container detail", [][2]string{
		{"l", "Live logs of this container (Esc returns here)"},
		{"r", "Reload stats, processes and logs"},
		{"Esc / q", "Back to the containers list"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Extensions", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"↵", "Schematic: the Image Factory YAML this node was built from (Esc returns)"},
		{"C", "Extension catalog"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Metrics / Processes / Disks / Addresses", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"r", "Refresh"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(section("Extension Catalog", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	if app.writeMode {
		sb.WriteString(section("Upgrade (write mode)", [][2]string{
			{"type", "Enter image / version"},
			{"tab", "Toggle --drain (--preserve on old CLI)"},
			{"↵", "Confirm"},
			{"y / n", "Yes / No on confirm step"},
			{"Esc", "Abort / back"},
		}))
		sb.WriteByte('\n')
	}

	sb.WriteString(section("Context Switcher", [][2]string{
		{"↑↓ / j k", "Navigate"},
		{"↵", "Switch to context"},
		{"Esc / q", "Back"},
	}))
	sb.WriteByte('\n')

	sb.WriteString(dimStyle.Render("  Press Esc, q or ? to close"))

	return sb.String()
}

func (app App) renderHelpView(height int) string {
	app.helpVP.Height = height
	app.helpVP.Width = app.width
	if app.helpVP.TotalLineCount() == 0 {
		app.helpVP.SetContent(buildHelpContentFor(app))
	}
	return app.helpVP.View()
}

// browserHelp lists every browser key table, generated from the same
// keyAction slices that drive dispatch and the hint bar.
func browserHelp(section func(string, [][2]string) string) string {
	titles := []struct {
		kind  paneKind
		title string
	}{
		{paneCategories, "Resource Browser: lists (categories / types / instances)"},
		{paneTypes, "Resource Browser: types pane (extra keys)"},
		{paneInstances, "Resource Browser: instances (extra keys)"},
		{paneYAML, "Resource Browser: YAML pane"},
		{paneDescribe, "Resource Browser: describe pane"},
		{paneRelated, "Resource Browser: related view (p)"},
		{paneNetwork, "Resource Browser: network view (N, n)"},
		{paneDisks, "Resource Browser: disk view (i)"},
		{paneCompare, "Resource Browser: compare nodes (c)"},
		{paneDiff, "Resource Browser: diff pane"},
	}
	var sb strings.Builder
	seen := map[string]bool{}
	for _, t := range titles {
		var rows [][2]string
		for _, a := range browserActionsFor(t.kind) {
			id := strings.Join(a.keys, "/") + "|" + a.desc
			if (t.kind == paneTypes || t.kind == paneInstances) && seen[id] {
				continue
			}
			if t.kind == paneCategories {
				seen[id] = true
			}
			rows = append(rows, [2]string{strings.Join(a.keys, " / "), a.desc})
		}
		if len(rows) > 0 {
			sb.WriteString(section(t.title, rows))
			sb.WriteByte('\n')
		}
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}
