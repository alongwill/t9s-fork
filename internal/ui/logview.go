package ui

import (
	"fmt"
	"strings"

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
// title, indicator and (when open) the filter prompt come off the top and
// bottom.
func (app App) logRowsFor(height int) int {
	fb := 0
	if app.findActive {
		fb = 1
	}
	return max(1, height-3-fb)
}

// logFreeze stops autoscroll, keeping the viewport where it is.
func (app App) logFreeze() App {
	return app.setLogPane(app.activeLog().freeze(app.logLayout(), app.logRowsFor(app.logHeight())))
}

// logResume turns autoscroll back on and jumps to the tail.
func (app App) logResume() App {
	return app.setLogPane(app.activeLog().resume())
}

// logMove puts the cursor on visible line idx. Anywhere but the tail turns
// autoscroll off, as in k9s.
func (app App) logMove(idx int) App {
	return app.setLogPane(app.activeLog().move(app.logLayout(), app.logRowsFor(app.logHeight()), idx))
}

// logNewCount is the number of lines that arrived while autoscroll is off.
func (app App) logNewCount() int { return app.activeLog().newCount() }

func onOff(label string, on bool) string {
	v := dimStyle.Render("Off")
	if on {
		v = okStyle.Render("On")
	}
	return dimStyle.Render(label+":") + v
}

// logIndicator is the k9s style state line under the logs title.
func (app App) logIndicator() string { return app.paneIndicator(app.activeLog()) }

func (app App) paneIndicator(p logPane) string {
	parts := []string{
		onOff("Autoscroll", !p.noFollow),
		onOff("FullScreen", app.logFull),
		onOff("Timestamps", app.logTS),
		onOff("Wrap", app.logWrap),
	}
	s := "  " + strings.Join(parts, "     ")
	if n := p.newCount(); n > 0 {
		s += "     " + warnStyle.Render(fmt.Sprintf("+%d new", n))
	}
	if p.fs.f.set {
		s += "     " + dimStyle.Render("Filter:") + keyStyle.Render(p.fs.raw)
		if p.fs.f.invalid {
			s += " " + warnStyle.Render("(invalid regex)")
		} else {
			s += dimStyle.Render(fmt.Sprintf(" (%d/%d)", p.n(), len(p.lines)))
		}
	}
	return lipgloss.NewStyle().MaxWidth(max(1, app.width)).Render(s)
}

// styledLine is a plain log line with its colour spans resolved per rune.
type styledLine struct {
	rs    []rune
	kinds []int // span kind per rune, -1 for none
	dim   bool  // DEBUG/TRACE: the whole line is dim
}

// newStyledLine resolves the colour spans of plain; find, when set, adds the
// find highlight on top of them.
func newStyledLine(plain, find string) styledLine {
	spans, dim := logSpans(plain)
	return buildStyledLine(plain, spans, dim, findSpans(plain, find))
}

// buildStyledLine lays the colour spans of a line, then the highlight spans on
// top of them, over its runes. A dim line drops the colour spans.
func buildStyledLine(plain string, spans []logSpan, dim bool, highlight []logSpan) styledLine {
	rs := []rune(plain)
	if dim {
		spans = nil
	}
	spans = append(append([]logSpan(nil), spans...), highlight...)
	kinds := make([]int, len(rs))
	for j := range kinds {
		kinds[j] = -1
	}
	for _, sp := range spans {
		for j := max(sp.a, 0); j < min(sp.b, len(rs)); j++ {
			kinds[j] = int(sp.kind)
		}
	}
	return styledLine{rs: rs, kinds: kinds, dim: dim}
}

// segment renders runes [from, to) with their colours. selected styles sit on
// the cursor row's background.
func (sl styledLine) segment(from, to int, selected bool) string {
	base := lipgloss.NewStyle()
	switch {
	case selected:
		base = selectedStyle
	case sl.dim:
		base = logDimStyle
	}
	var sb strings.Builder
	for a := from; a < to; {
		b := a
		for b < to && sl.kinds[b] == sl.kinds[a] {
			b++
		}
		st := base
		if sl.kinds[a] >= 0 {
			st = logSpanStyle(logSpanKind(sl.kinds[a]), selected)
		}
		sb.WriteString(st.Render(string(sl.rs[a:b])))
		a = b
	}
	return sb.String()
}

// styleLine resolves the colours of visible line i of the pane: the stream's
// own level/timestamp colouring, then the filter highlight.
func (p logPane) styleLine(plain string) styledLine {
	var spans []logSpan
	var dim bool
	if p.kind == logKindDmesg {
		spans, dim = dmesgSpans(plain)
	} else {
		spans, dim = logSpans(plain)
	}
	return buildStyledLine(plain, spans, dim, p.fs.f.spans(plain))
}

// timestamp is the Timestamps prefix of visible line i.
func (p logPane) timestamp(plain string, i int) string {
	if p.kind == logKindDmesg {
		return dmesgTimestamp(plain, p.arrival(i))
	}
	return logTimestamp(plain, p.arrival(i))
}

// renderLogLine renders one logical line of the current view as terminal rows.
func (app App) renderLogLine(i int, selected bool, maxRows int) []string {
	return app.renderPaneLine(app.activeLog(), i, selected, maxRows)
}

// renderPaneLine renders visible line i of p as terminal rows.
func (app App) renderPaneLine(p logPane, i int, selected bool, maxRows int) []string {
	lay := app.logLayout()
	plain := plainLogLine(p.line(i))
	sl := p.styleLine(plain)

	ranges, cut := lay.chunks(sl.rs)
	if len(ranges) > maxRows {
		ranges = ranges[:maxRows]
	}
	prefix := ""
	if app.logTS {
		prefix = p.timestamp(plain, i)
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
		text := sl.segment(r[0], r[1], selected)
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

// renderLogs renders the service logs view.
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
	return app.renderLogPane(app.logPaneFor(logKindService), title, "Waiting for logs…", height)
}

// renderLogPane renders a log pane: title, indicator line, the visible lines
// and the filter prompt. It is shared by the service logs and dmesg views.
func (app App) renderLogPane(p logPane, title, waiting string, height int) string {
	head := title + app.paneIndicator(p) + "\n"

	if len(p.lines) == 0 {
		if bar := app.renderFilterBar(); bar != "" {
			return head + "  " + infoStyle.Render(waiting) + "\n" + bar
		}
		return head + "  " + infoStyle.Render(waiting)
	}
	n := p.n()
	if n == 0 {
		return head + "  " + dimStyle.Render(fmt.Sprintf("No lines match (0/%d)", len(p.lines))) + "\n" + app.renderFilterBar()
	}

	maxRows := app.logRowsFor(height)
	lay := app.logLayout()
	lines := p.text()
	cur := min(max(p.cur, 0), n-1)
	var top int
	if p.noFollow {
		top = lay.ensureVisible(lines, p.top, cur, maxRows)
	} else {
		cur = n - 1
		top = lay.topFor(lines, n-1, maxRows)
	}

	var sb strings.Builder
	sb.WriteString(head)
	used := 0
	for i := top; i < n && used < maxRows; i++ {
		for _, row := range app.renderPaneLine(p, i, i == cur, maxRows-used) {
			sb.WriteString(row)
			sb.WriteByte('\n')
			used++
		}
	}
	return sb.String() + app.renderFilterBar()
}

// renderFilterBar is the prompt line shown while the filter is being typed.
func (app App) renderFilterBar() string {
	if !app.findActive {
		return ""
	}
	return keyStyle.Render("/") + " " + app.findInput.View() + "\n"
}
