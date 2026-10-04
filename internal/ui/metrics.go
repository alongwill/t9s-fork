package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/talos"
)

func (app App) handleMetricsKey(msg tea.KeyMsg) (App, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc", "q":
		app = app.goBack()
	case "r":
		if app.selNode != nil {
			app.statsLoading = true
			return app, app.loadStats()
		}
	}
	return app, nil
}

func (app App) renderMetrics(height int) string {
	node := ""
	if app.selNode != nil {
		node = app.selNode.Hostname
	}
	title := fmt.Sprintf("  Container Metrics: %s\n", titleStyle.Render(node))

	if app.statsLoading && len(app.stats) == 0 {
		return title + lipgloss.Place(app.width, height-2, lipgloss.Center, lipgloss.Center,
			infoStyle.Render("Loading metrics…"))
	}
	if len(app.stats) == 0 {
		return title + lipgloss.Place(app.width, height-2, lipgloss.Center, lipgloss.Center,
			warnStyle.Render("No stats available."))
	}

	colID, colNS, colCPU, _ := metricsWidths(app.width)

	hdr := colHeaderStyle.Render(
		"  " + col("CONTAINER", colID) + "  " + col("NAMESPACE", colNS) + "  " + col("CPU%", colCPU) + "  " + "MEMORY",
	)

	var sb strings.Builder
	sb.WriteString(title)
	sb.WriteString(hdr)
	sb.WriteByte('\n')

	// elapsed = interval between the two stat samples (fixed, not render-time).
	elapsed := app.statsAt.Sub(app.prevStatsAt).Nanoseconds()

	prevMap := make(map[string]int64, len(app.prevStats))
	for _, p := range app.prevStats {
		prevMap[p.ID] = p.CPUNanos
	}

	for i, s := range app.stats {
		if i >= height-3 {
			break
		}

		cpuStr := "–"
		if elapsed > 0 {
			if prev, ok := prevMap[s.ID]; ok && s.CPUNanos >= prev {
				pct := float64(s.CPUNanos-prev) / float64(elapsed) * 100.0
				cpuStr = fmt.Sprintf("%.1f%%", pct)
				switch {
				case pct >= 80:
					cpuStr = errStyle.Render(cpuStr)
				case pct >= 50:
					cpuStr = warnStyle.Render(cpuStr)
				default:
					cpuStr = okStyle.Render(cpuStr)
				}
			}
		}

		memStr := formatMem(s.MemoryMB)

		ns, name := splitMetricsID(s.ID)
		row := "  " +
			col(truncate(name, colID), colID) + "  " +
			col(truncate(ns, colNS), colNS) + "  " +
			padRight(cpuStr, colCPU) + "  " +
			memStr

		sb.WriteString(row)
		sb.WriteByte('\n')
	}

	if len(app.prevStats) == 0 {
		sb.WriteString("\n  " + dimStyle.Render("CPU% available after first refresh (5s)…"))
	}

	return sb.String()
}

// metricsWidths gives the column widths for a terminal width: the namespace,
// CPU and memory columns are fixed, the container column takes the rest
// (at least 20, at most 60). At 80 columns the container name gets 40.
func metricsWidths(width int) (id, ns, cpu, mem int) {
	ns, cpu, mem = 16, 8, 10
	id = min(60, max(20, width-2-(2+ns)-(2+cpu)-(2+mem)))
	return id, ns, cpu, mem
}

// splitMetricsID splits a stats ID into the pod namespace and the name shown
// in the container column. A CRI ID "ns/pod:name:id12" gives ("ns",
// "pod:name:id12"): the namespace has its own column, so it is not repeated.
// Talos system containers ("apid") have no pod namespace and give ("-", id).
func splitMetricsID(id string) (ns, name string) {
	c := talos.ParseCRIID(id)
	if !c.CRI {
		return "-", id
	}
	return c.Namespace, strings.TrimPrefix(id, c.Namespace+"/")
}

func formatMem(mb float64) string {
	if mb >= 1024 {
		gb := mb / 1024
		s := fmt.Sprintf("%.2f GB", gb)
		if gb >= 4 {
			return warnStyle.Render(s)
		}
		return infoStyle.Render(s)
	}
	s := fmt.Sprintf("%.0f MB", mb)
	return dimStyle.Render(s)
}

// col for colored strings: pad based on visual width, not byte length.
// Used when the string may already contain ANSI codes.
func colStyled(s string, w int) string {
	_ = talos.StatsResult{} // ensure import used
	return padRight(s, w)
}
