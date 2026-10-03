package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/talos"
)

// detailLogTail is how many log lines the detail view shows.
const detailLogTail = 30

// section is one independently loaded part of the container detail view.
type section struct {
	loading bool
	err     string
}

// containerDetail is the state of the container detail view.
type containerDetail struct {
	c    talos.ContainerInfo
	id   talos.CRIID
	seq  uint64 // bumped on every (re)load; stale messages are dropped
	stat *talos.StatsResult
	// memTotalMB is the node's total memory, 0 when unknown.
	memTotalMB float64
	procs      []talos.ProcessInfo
	logs       []string

	statsSec, procsSec, logsSec section
	memErr                      bool // memory total unavailable: show the plain figure
}

type detailStatsMsg struct {
	seq  uint64
	stat *talos.StatsResult // nil when the container is not in the stats
	err  error
}

type detailMemMsg struct {
	seq     uint64
	totalMB float64
	err     error
}

type detailProcsMsg struct {
	seq   uint64
	procs []talos.ProcessInfo
	err   error
}

type detailLogsMsg struct {
	seq   uint64
	lines []string
	err   error
}

// isSandbox reports a CRI pod sandbox row, which has no logs of its own.
func (d containerDetail) isSandbox() bool { return d.id.CRI && d.id.Name == "" }

// openContainerDetail opens the detail view for c and starts its loads.
func (app App) openContainerDetail(c talos.ContainerInfo) (App, tea.Cmd) {
	seq := app.detail.seq + 1
	app.detail = containerDetail{c: c, id: talos.ParseCRIID(c.ID), seq: seq}.markLoading()
	app = app.goTo(StateContainerDetail)
	return app, app.loadContainerDetail()
}

// loadContainerDetail marks every section loading and returns the commands
// that fill them, each with its own 10 s timeout.
func (app App) loadContainerDetail() tea.Cmd {
	d := app.detail
	client := app.client
	node := app.selNode.IP
	ns := d.c.Namespace
	cid := d.c.ID
	seq := d.seq
	withTimeout := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 10*time.Second)
	}

	cmds := []tea.Cmd{
		func() tea.Msg {
			ctx, cancel := withTimeout()
			defer cancel()
			stats, err := client.GetContainerStats(ctx, node, ns)
			if err != nil {
				return detailStatsMsg{seq: seq, err: err}
			}
			for i := range stats {
				if stats[i].ID == cid {
					return detailStatsMsg{seq: seq, stat: &stats[i]}
				}
			}
			return detailStatsMsg{seq: seq}
		},
		func() tea.Msg {
			ctx, cancel := withTimeout()
			defer cancel()
			total, err := client.GetNodeMemoryTotalMB(ctx, node)
			return detailMemMsg{seq: seq, totalMB: total, err: err}
		},
		func() tea.Msg {
			ctx, cancel := withTimeout()
			defer cancel()
			procs, err := client.GetProcesses(ctx, node)
			return detailProcsMsg{seq: seq, procs: procs, err: err}
		},
	}
	if !d.isSandbox() {
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := withTimeout()
			defer cancel()
			lines, err := client.GetContainerLogs(ctx, node, ns, cid, detailLogTail)
			return detailLogsMsg{seq: seq, lines: lines, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// markLoading resets the sections to their loading state.
func (d containerDetail) markLoading() containerDetail {
	d.stat, d.procs, d.logs, d.memTotalMB, d.memErr = nil, nil, nil, 0, false
	d.statsSec = section{loading: true}
	d.procsSec = section{loading: true}
	d.logsSec = section{loading: !d.isSandbox()}
	return d
}

// applyDetailMsg stores one finished load, dropping it when it belongs to an
// earlier load.
func (app App) applyDetailMsg(msg tea.Msg) App {
	d := app.detail
	switch m := msg.(type) {
	case detailStatsMsg:
		if m.seq != d.seq {
			return app
		}
		d.statsSec.loading = false
		if m.err != nil {
			d.statsSec.err = m.err.Error()
		} else {
			d.stat = m.stat
		}
	case detailMemMsg:
		if m.seq != d.seq {
			return app
		}
		if m.err != nil {
			d.memErr = true
		} else {
			d.memTotalMB = m.totalMB
		}
	case detailProcsMsg:
		if m.seq != d.seq {
			return app
		}
		d.procsSec.loading = false
		if m.err != nil {
			d.procsSec.err = m.err.Error()
		} else {
			d.procs = matchContainerProcesses(m.procs, d.c.PID)
		}
	case detailLogsMsg:
		if m.seq != d.seq {
			return app
		}
		d.logsSec.loading = false
		if m.err != nil {
			d.logsSec.err = m.err.Error()
		} else {
			d.logs = m.lines
		}
	}
	app.detail = d
	return app
}

// matchContainerProcesses returns the process rows that belong to the
// container. `processes` has no parent PID column, so descendants cannot be
// told apart: only the row of the container's own PID is returned.
func matchContainerProcesses(procs []talos.ProcessInfo, pid string) []talos.ProcessInfo {
	if pid == "" || pid == "0" {
		return nil
	}
	var out []talos.ProcessInfo
	for _, p := range procs {
		if p.PID == pid {
			out = append(out, p)
		}
	}
	return out
}

func (app App) handleContainerDetailKey(msg tea.KeyMsg) (App, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc", "q":
		app = app.goBack()
	case "r":
		if app.selNode != nil {
			app.detail.seq++
			app.detail = app.detail.markLoading()
			return app, app.loadContainerDetail()
		}
	case "l":
		if app.selNode == nil {
			return app, nil
		}
		if app.detail.isSandbox() {
			app.statusMsg = warnStyle.Render("A pod sandbox has no logs; open one of its containers")
			return app, nil
		}
		ns, id := app.detail.c.Namespace, app.detail.c.ID
		run := app.runContainerLogStream
		return app.startLogRun(id, func(ctx context.Context, node, target string, ch chan<- string) {
			run(ctx, node, ns, target, ch)
		})
	}
	return app, nil
}

// --- rendering ---

func formatMB(mb float64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1f GB", mb/1024)
	}
	return fmt.Sprintf("%.1f MB", mb)
}

// memBar draws used/total as a bar of the given width.
func memBar(used, total float64, width int) string {
	if total <= 0 || width < 4 {
		return ""
	}
	frac := min(max(used/total, 0), 1)
	filled := int(frac*float64(width) + 0.5)
	style := okStyle
	switch {
	case frac >= 0.8:
		style = errStyle
	case frac >= 0.5:
		style = warnStyle
	}
	return style.Render(strings.Repeat("█", filled)) + dimStyle.Render(strings.Repeat("░", width-filled))
}

// imageLine renders an image reference with the tag in bold.
func imageLine(ref string) string {
	if ref == "" {
		return dimStyle.Render("-")
	}
	r := talos.ParseImageRef(ref)
	var sb strings.Builder
	if r.Registry != "" {
		sb.WriteString(dimStyle.Render(r.Registry + "/"))
	}
	sb.WriteString(r.Repo)
	if r.Tag != "" {
		sb.WriteString(dimStyle.Render(":") + lipgloss.NewStyle().Bold(true).Render(r.Tag))
	}
	if r.Digest != "" {
		d := r.Digest
		if len(d) > 19 {
			d = d[:19] + "…"
		}
		sb.WriteString(dimStyle.Render("@" + d))
	}
	return sb.String()
}

// clipLine cuts a styled line to width cells.
func clipLine(s string, width int) string {
	return lipgloss.NewStyle().MaxWidth(max(1, width)).Render(s)
}

func (app App) detailHeaderBox(width int) []string {
	d := app.detail
	label := func(s string) string { return dimStyle.Render(col(s, 10)) }

	var rows []string
	rows = append(rows, label("ID")+d.c.ID)
	if d.id.CRI {
		pod := d.id.Namespace + " / " + d.id.Pod
		rows = append(rows, label("Pod")+pod)
		if d.id.Name != "" {
			rows = append(rows, label("Container")+d.id.Name+dimStyle.Render("  "+d.id.ShortID))
		} else {
			rows = append(rows, label("Container")+dimStyle.Render("(pod sandbox)"))
		}
	}
	pid := d.c.PID
	if pid == "" {
		pid = "-"
	}
	rows = append(rows,
		label("Image")+imageLine(d.c.Image),
		label("PID")+col(pid, 10)+dimStyle.Render("Status ")+colorStatus(d.c.Status)+
			dimStyle.Render("   Namespace ")+d.c.Namespace,
	)

	inner := max(10, width-4)
	for i, r := range rows {
		rows[i] = clipLine(r, inner)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorCyan).
		Padding(0, 1).
		Width(max(12, width-2))
	return strings.Split(box.Render(strings.Join(rows, "\n")), "\n")
}

func (app App) detailResources(width int) []string {
	d := app.detail
	title := titleStyle.Render("Resources")
	switch {
	case d.statsSec.loading:
		return []string{title, "  " + infoStyle.Render("Loading stats…")}
	case d.statsSec.err != "":
		return []string{title, "  " + errStyle.Render("Stats: "+d.statsSec.err)}
	case d.stat == nil:
		return []string{title, "  " + dimStyle.Render("No stats for this container")}
	}
	cpu := time.Duration(d.stat.CPUNanos).Round(time.Millisecond)
	cpuLine := "  " + dimStyle.Render(col("CPU", 8)) + fmt.Sprintf("%s CPU time", cpu)
	mem := "  " + dimStyle.Render(col("Memory", 8)) + formatMB(d.stat.MemoryMB)
	if d.memTotalMB > 0 {
		pct := d.stat.MemoryMB / d.memTotalMB * 100
		bar := memBar(d.stat.MemoryMB, d.memTotalMB, min(30, max(8, width-50)))
		mem += "  " + bar + dimStyle.Render(fmt.Sprintf("  %.1f%% of %s", pct, formatMB(d.memTotalMB)))
	}
	return []string{title, clipLine(cpuLine, width), clipLine(mem, width)}
}

func (app App) detailProcesses(width, maxRows int) []string {
	d := app.detail
	title := titleStyle.Render("Processes") + dimStyle.Render("  (matched by PID; no parent PIDs available)")
	switch {
	case d.procsSec.loading:
		return []string{clipLine(title, width), "  " + infoStyle.Render("Loading processes…")}
	case d.procsSec.err != "":
		return []string{clipLine(title, width), "  " + errStyle.Render("Processes: "+d.procsSec.err)}
	case len(d.procs) == 0:
		return []string{clipLine(title, width), "  " + dimStyle.Render("No process with this PID")}
	}
	out := []string{clipLine(title, width)}
	out = append(out, clipLine("  "+colHeaderStyle.Render(col("PID", 8)+col("STATE", 7)+col("CPU-TIME", 10)+col("RESMEM", 10)+"COMMAND"), width))
	for _, p := range d.procs {
		if len(out)-2 >= maxRows {
			break
		}
		out = append(out, clipLine("  "+col(p.PID, 8)+col(p.State, 7)+col(p.CPUTime, 10)+col(p.ResMem, 10)+p.Command, width))
	}
	return out
}

func (app App) detailLogs(width, maxRows int) []string {
	d := app.detail
	title := titleStyle.Render("Recent logs") + dimStyle.Render(fmt.Sprintf("  (last %d lines, l for the live stream)", detailLogTail))
	switch {
	case d.isSandbox():
		return []string{clipLine(title, width), "  " + dimStyle.Render("A pod sandbox has no logs; open one of its containers")}
	case d.logsSec.loading:
		return []string{clipLine(title, width), "  " + infoStyle.Render("Loading logs…")}
	case d.logsSec.err != "":
		return []string{clipLine(title, width), "  " + errStyle.Render("Logs: "+d.logsSec.err)}
	case len(d.logs) == 0:
		return []string{clipLine(title, width), "  " + dimStyle.Render("No log lines")}
	}
	out := []string{clipLine(title, width)}
	lines := d.logs
	if room := maxRows - 1; room > 0 && len(lines) > room {
		lines = lines[len(lines)-room:]
	}
	lay := logLayout{width: width, wrap: false}
	for _, l := range lines {
		sl := newStyledLine(plainLogLine(l), "")
		ranges, cut := lay.chunks(sl.rs)
		text := sl.segment(ranges[0][0], ranges[0][1], false)
		if cut {
			text += logDimStyle.Render("…")
		}
		out = append(out, "  "+text)
	}
	return out
}

// renderContainerDetail lays the sections out within height rows: the header
// box and resources keep their size, processes shrink first, and the logs
// take what is left and show their newest lines.
func (app App) renderContainerDetail(height int) string {
	width := max(20, app.width)
	box := app.detailHeaderBox(width - 2)
	res := app.detailResources(width)

	// The title line of the view.
	node := ""
	if app.selNode != nil {
		node = app.selNode.Hostname
	}
	title := []string{fmt.Sprintf("  Container %s on %s", titleStyle.Render(app.detail.c.ID), titleStyle.Render(node))}
	title[0] = clipLine(title[0], width)

	// One row is left spare, as the other views do.
	avail := height - 1
	used := len(title) + len(box) + len(res)
	procRows := 3
	logsMin := 4
	for avail-used-(2+procRows) < logsMin && procRows > 1 {
		procRows--
	}
	procs := app.detailProcesses(width, procRows)
	logs := app.detailLogs(width, max(2, avail-used-len(procs)))

	var lines []string
	lines = append(lines, title...)
	for _, l := range box {
		lines = append(lines, " "+l)
	}
	lines = append(lines, res...)
	lines = append(lines, procs...)
	lines = append(lines, logs...)
	if len(lines) > avail {
		lines = lines[:max(1, avail)]
	}
	return strings.Join(lines, "\n") + "\n"
}
