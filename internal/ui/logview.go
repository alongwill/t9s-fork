package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// logLayout holds what decides how many terminal rows a log line takes.
type logLayout struct {
	width int
	ts    bool
	wrap  bool
}

// plainLogLine strips colour codes and control characters so the line can be
// measured and cut by rune.
func plainLogLine(s string) string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\t", "    ")
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// avail is the text width: terminal width minus the 2 cell cursor gutter and
// the timestamp column.
func (l logLayout) avail() int {
	w := l.width - 2
	if l.ts {
		w -= logTSPrefixW
	}
	return max(1, w)
}

// chunks splits rs into the rune ranges shown on each terminal row: hard
// wrapped by display width when wrapping, otherwise one range cut to fit.
// cut reports whether the single range was truncated.
func (l logLayout) chunks(rs []rune) (ranges [][2]int, cut bool) {
	avail := l.avail()
	if !l.wrap {
		w, end := 0, 0
		for end < len(rs) {
			rw := runewidth.RuneWidth(rs[end])
			if w+rw > avail {
				break
			}
			w += rw
			end++
		}
		if end < len(rs) {
			// Leave a cell for the ellipsis.
			for end > 0 && w+1 > avail {
				end--
				w -= runewidth.RuneWidth(rs[end])
			}
			return [][2]int{{0, end}}, true
		}
		return [][2]int{{0, len(rs)}}, false
	}
	start, w := 0, 0
	for i, r := range rs {
		rw := runewidth.RuneWidth(r)
		if w+rw > avail && i > start {
			ranges = append(ranges, [2]int{start, i})
			start, w = i, 0
		}
		w += rw
	}
	return append(ranges, [2]int{start, len(rs)}), false
}

// rows is the number of terminal rows line takes.
func (l logLayout) rows(line string) int {
	if !l.wrap {
		return 1
	}
	r, _ := l.chunks([]rune(plainLogLine(line)))
	return len(r)
}

// topFor returns the lowest line index such that lines top..last fit in
// maxRows terminal rows (at least last itself).
func (l logLayout) topFor(lines []string, last, maxRows int) int {
	if last < 0 || len(lines) == 0 {
		return 0
	}
	last = min(last, len(lines)-1)
	top, budget := last, maxRows-l.rows(lines[last])
	for top > 0 {
		n := l.rows(lines[top-1])
		if n > budget {
			break
		}
		budget -= n
		top--
	}
	return top
}

// ensureVisible moves top just far enough that cur is on screen.
func (l logLayout) ensureVisible(lines []string, top, cur, maxRows int) int {
	if len(lines) == 0 {
		return 0
	}
	top = min(max(top, 0), len(lines)-1)
	if cur < top {
		return cur
	}
	if t := l.topFor(lines, cur, maxRows); t > top {
		return t
	}
	return top
}

func (app App) logLayout() logLayout {
	return logLayout{width: app.width, ts: app.logTS, wrap: app.logWrap}
}

// logHeight is the number of terminal rows the logs view gets.
func (app App) logHeight() int {
	if app.logFull {
		return app.height
	}
	return app.mainHeight()
}

// logRowsFor is the number of rows left for log lines in a view height: the
// title, indicator and (when open) find bar come off the top and bottom.
func (app App) logRowsFor(height int) int {
	fb := 0
	if app.findActive || app.findQuery != "" {
		fb = 1
	}
	return max(1, height-3-fb)
}

// logFreeze stops autoscroll, keeping the viewport where it is.
func (app App) logFreeze() App {
	if app.logNoFollow {
		return app
	}
	n := len(app.logLines)
	app.logTop = app.logLayout().topFor(app.logLines, n-1, app.logRowsFor(app.logHeight()))
	app.logNoFollow = true
	app.logFrozenN = n
	return app
}

// logResume turns autoscroll back on and jumps to the tail.
func (app App) logResume() App {
	app.logNoFollow = false
	app.logFrozenN = 0
	app.logCur = max(0, len(app.logLines)-1)
	return app
}

// logMove puts the cursor on line idx. Anywhere but the tail turns autoscroll
// off, as in k9s.
func (app App) logMove(idx int) App {
	n := len(app.logLines)
	if n == 0 {
		return app
	}
	idx = min(max(idx, 0), n-1)
	if !app.logNoFollow && idx == n-1 {
		app.logCur = idx
		return app
	}
	app = app.logFreeze()
	app.logCur = idx
	app.logTop = app.logLayout().ensureVisible(app.logLines, app.logTop, idx, app.logRowsFor(app.logHeight()))
	return app
}

// logNewCount is the number of lines that arrived while autoscroll is off.
func (app App) logNewCount() int {
	if !app.logNoFollow {
		return 0
	}
	return max(0, len(app.logLines)-app.logFrozenN)
}

func onOff(label string, on bool) string {
	v := dimStyle.Render("Off")
	if on {
		v = okStyle.Render("On")
	}
	return dimStyle.Render(label+":") + v
}

// logIndicator is the k9s style state line under the logs title.
func (app App) logIndicator() string {
	parts := []string{
		onOff("Autoscroll", !app.logNoFollow),
		onOff("FullScreen", app.logFull),
		onOff("Timestamps", app.logTS),
		onOff("Wrap", app.logWrap),
	}
	s := "  " + strings.Join(parts, "     ")
	if n := app.logNewCount(); n > 0 {
		s += "     " + warnStyle.Render(fmt.Sprintf("+%d new", n))
	}
	return lipgloss.NewStyle().MaxWidth(max(1, app.width)).Render(s)
}

// logArrival returns when line i arrived (zero when unknown).
func (app App) logArrival(i int) time.Time {
	if i < len(app.logArrived) {
		return app.logArrived[i]
	}
	return time.Time{}
}

// renderLogLine renders one logical line as terminal rows.
func (app App) renderLogLine(i int, selected bool, maxRows int) []string {
	lay := app.logLayout()
	raw := app.logLines[i]
	plain := plainLogLine(raw)
	rs := []rune(plain)

	spans, dim := logSpans(plain)
	if dim {
		spans = nil
	}
	spans = append(spans, findSpans(plain, app.findQuery)...)
	kinds := make([]int, len(rs))
	for j := range kinds {
		kinds[j] = -1
	}
	for _, sp := range spans {
		for j := max(sp.a, 0); j < min(sp.b, len(rs)); j++ {
			kinds[j] = int(sp.kind)
		}
	}

	base := lipgloss.NewStyle()
	switch {
	case selected:
		base = selectedStyle
	case dim:
		base = logDimStyle
	}
	seg := func(from, to int) string {
		var sb strings.Builder
		for a := from; a < to; {
			b := a
			for b < to && kinds[b] == kinds[a] {
				b++
			}
			st := base
			if kinds[a] >= 0 {
				st = logSpanStyle(logSpanKind(kinds[a]), selected)
			}
			sb.WriteString(st.Render(string(rs[a:b])))
			a = b
		}
		return sb.String()
	}

	ranges, cut := lay.chunks(rs)
	if len(ranges) > maxRows {
		ranges = ranges[:maxRows]
	}
	prefix := ""
	if app.logTS {
		prefix = logTimestamp(plain, app.logArrival(i))
	}
	out := make([]string, 0, len(ranges))
	for k, r := range ranges {
		gutter := "  "
		if k == 0 && selected {
			gutter = "▶ "
		}
		pre := ""
		if app.logTS {
			pre = strings.Repeat(" ", logTSPrefixW)
			if k == 0 {
				pre = logDimStyle.Render(prefix) + " "
				if selected {
					pre = selectedStyle.Render(prefix + " ")
				}
			}
		}
		text := seg(r[0], r[1])
		if cut {
			text += logDimStyle.Render("…")
		}
		row := gutter + pre + text
		if selected {
			row = selectedStyle.Render(gutter) + pre + text
			if pad := app.width - lipgloss.Width(row); pad > 0 {
				row += selectedStyle.Render(strings.Repeat(" ", pad))
			}
		}
		out = append(out, row)
	}
	return out
}

// renderLogs renders the logs view: title, indicator line, the lines, and the
// find bar.
func (app App) renderLogs(height int) string {
	node := ""
	if app.selNode != nil {
		node = app.selNode.Hostname
	}
	streaming := dimStyle.Render(" [stopped]")
	if app.logStreaming {
		streaming = infoStyle.Render(" [streaming]")
	}
	title := fmt.Sprintf("  Logs: %s on %s%s\n",
		titleStyle.Render(app.logService),
		titleStyle.Render(node),
		streaming,
	)
	head := title + app.logIndicator() + "\n"

	if len(app.logLines) == 0 {
		return head + "  " + infoStyle.Render("Waiting for logs…")
	}

	maxRows := app.logRowsFor(height)
	lay := app.logLayout()
	n := len(app.logLines)
	cur := min(max(app.logCur, 0), n-1)
	var top int
	if app.logNoFollow {
		top = lay.ensureVisible(app.logLines, app.logTop, cur, maxRows)
	} else {
		cur = n - 1
		top = lay.topFor(app.logLines, n-1, maxRows)
	}

	var sb strings.Builder
	sb.WriteString(head)
	used := 0
	for i := top; i < n && used < maxRows; i++ {
		for _, row := range app.renderLogLine(i, i == cur, maxRows-used) {
			sb.WriteString(row)
			sb.WriteByte('\n')
			used++
		}
	}
	return sb.String() + app.renderFindBar(app.logLines)
}
