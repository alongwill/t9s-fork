package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/reflow/wrap"
)

func (app App) handleLogsKey(msg tea.KeyMsg) (App, tea.Cmd) {
	return app.handleLogPaneKey(msg, logKindService)
}

// handleLogPaneKey is the key handling shared by the service logs and dmesg
// views.
func (app App) handleLogPaneKey(msg tea.KeyMsg, kind logKind) (App, tea.Cmd) {
	if app.findActive {
		return app.handleFilterKey(msg, kind)
	}

	rows := app.logRowsFor(app.logHeight())
	page := max(1, rows/2)
	lay := app.logLayout()
	p := app.logPaneFor(kind)

	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc", "q":
		if p.fs.raw != "" {
			p = p.setFilter("")
			p.fs.prev = ""
			return app.setLogPane(p), nil
		}
		if app.logFull {
			app.logFull = false
			return app, nil
		}
		if kind == logKindDmesg {
			app.stopDmesg()
		} else {
			app.stopLogs()
		}
		app = app.goBack()
		return app, nil
	case "up", "k":
		app = app.setLogPane(p.move(lay, rows, p.cur-1))
	case "down", "j":
		app = app.setLogPane(p.move(lay, rows, p.cur+1))
	case "pgup":
		app = app.setLogPane(p.move(lay, rows, p.cur-page))
	case "pgdown":
		app = app.setLogPane(p.move(lay, rows, p.cur+page))
	case "g", "home":
		app = app.setLogPane(p.move(lay, rows, 0))
	case "G", "end":
		app = app.setLogPane(p.resume())
	case "s":
		if p.noFollow {
			app = app.setLogPane(p.resume())
		} else {
			app = app.setLogPane(p.freeze(lay, rows))
		}
	case "f":
		app.logFull = !app.logFull
	case "t":
		app.logTS = !app.logTS
	case "w":
		app.logWrap = !app.logWrap
	case "/":
		p.fs.prev = p.fs.raw
		app = app.setLogPane(p)
		app.findActive = true
		app.findInput.SetValue(p.fs.raw)
		app.findInput.CursorEnd()
		return app, app.findInput.Focus()
	case "n":
		app = app.setLogPane(p.step(lay, rows, 1))
	case "N":
		app = app.setLogPane(p.step(lay, rows, -1))
	}
	return app, nil
}

// handleFilterKey routes keypresses while the filter prompt is open. The
// filter applies live: every keystroke narrows the view. enter keeps it, esc
// restores the filter the prompt opened with.
func (app App) handleFilterKey(msg tea.KeyMsg, kind logKind) (App, tea.Cmd) {
	p := app.logPaneFor(kind)
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc":
		app.findActive = false
		app.findInput.Blur()
		return app.setLogPane(p.setFilter(p.fs.prev)), nil
	case "enter":
		app.findActive = false
		app.findInput.Blur()
		return app, nil
	}
	var cmd tea.Cmd
	app.findInput, cmd = app.findInput.Update(msg)
	return app.setLogPane(p.setFilter(app.findInput.Value())), cmd
}

// findLineNext returns the index of the next line containing q, starting at
// from and wrapping around. Returns -1 when no match exists.
func findLineNext(lines []string, from int, q string) int {
	if q == "" || len(lines) == 0 {
		return -1
	}
	q = strings.ToLower(q)
	n := len(lines)
	for i := 0; i < n; i++ {
		idx := (from + i) % n
		if strings.Contains(strings.ToLower(lines[idx]), q) {
			return idx
		}
	}
	return -1
}

// findLinePrev returns the index of the previous line containing q, starting
// at from and wrapping around. Returns -1 when no match exists.
func findLinePrev(lines []string, from int, q string) int {
	if q == "" || len(lines) == 0 {
		return -1
	}
	q = strings.ToLower(q)
	n := len(lines)
	for i := 0; i < n; i++ {
		idx := ((from-i)%n + n) % n
		if strings.Contains(strings.ToLower(lines[idx]), q) {
			return idx
		}
	}
	return -1
}

// countMatches returns how many lines contain q (case-insensitive).
func countMatches(lines []string, q string) int {
	if q == "" {
		return 0
	}
	q = strings.ToLower(q)
	n := 0
	for _, l := range lines {
		if strings.Contains(strings.ToLower(l), q) {
			n++
		}
	}
	return n
}

// renderLinesCursor renders a scrollable, cursor-highlighted list of lines.
// findQuery, when non-empty, marks matching lines with a ▸ prefix.
// scrollStart is the caller's persisted scroll offset, used when the cursor
// is above the anchor window so the cursor moves within the view instead of
// being pinned to the top row.
func renderLinesCursor(lines []string, cur, width, maxRows, scrollStart int, findQuery string) string {
	if len(lines) == 0 || maxRows <= 0 {
		return ""
	}
	if cur < 0 {
		cur = 0
	}
	if cur >= len(lines) {
		cur = len(lines) - 1
	}

	w := width
	if w <= 0 {
		w = 80
	}

	fq := strings.ToLower(findQuery)

	// Anchor the view to the LAST line of the log buffer.
	// Walk backward from len(lines)-1 until we fill the screen.
	// The cursor then moves freely inside this window without
	// shifting the view — the latest log stays visible at the
	// bottom until the cursor scrolls above the top of the window.
	anchorStart := len(lines) - 1
	budget := maxRows - 1
	for anchorStart > 0 && budget > 0 {
		prev := anchorStart - 1
		n := strings.Count(wrap.String(lines[prev], max(1, w-2)), "\n") + 1
		if n > budget {
			break
		}
		budget -= n
		anchorStart = prev
	}

	// If the cursor is inside the anchor window, keep the anchor so the
	// last line stays pinned at the bottom.
	// If the cursor moved above the anchor window, apply the same
	// boundary-scroll logic as list views: cursor moves freely inside the
	// visible area and the window only scrolls when the cursor hits an edge.
	//
	// clampScrollStart counts logical items, not physical rows. For wrapped
	// lines the two differ, so we verify the cursor is reachable from the
	// chosen start and fall back to start=cur (cursor at top) if not.
	var start int
	if cur >= anchorStart {
		start = anchorStart
	} else {
		candidate := clampScrollStart(scrollStart, cur, len(lines), maxRows)
		rows := 0
		curReachable := false
		for i := candidate; i < len(lines) && rows < maxRows; i++ {
			n := strings.Count(wrap.String(lines[i], max(1, w-2)), "\n") + 1
			if n > maxRows-rows {
				n = maxRows - rows
			}
			rows += n
			if i == cur {
				curReachable = true
				break
			}
		}
		if curReachable {
			start = candidate
		} else {
			start = cur // fallback: cursor pinned to top
		}
	}

	var sb strings.Builder
	lineCount := 0

	for i := start; i < len(lines) && lineCount < maxRows; i++ {
		raw := lines[i]

		// Detect severity from the full line, then wrap raw text.
		// Applying the style per physical sub-line ensures continuation
		// lines keep the correct colour even after a wrap.
		applyColor := lineLogStyle(raw)
		wrapped := wrap.String(raw, max(1, w-2))
		physLines := strings.Split(wrapped, "\n")

		// Trim to fit remaining budget.
		remaining := maxRows - lineCount
		if len(physLines) > remaining {
			physLines = physLines[:remaining]
		}

		selected := i == cur
		isMatch := fq != "" && strings.Contains(strings.ToLower(raw), fq)

		for j, pl := range physLines {
			switch {
			case selected:
				// All physical lines of the selected item are highlighted.
				prefix := "  "
				if j == 0 {
					prefix = "▶ "
				}
				sb.WriteString(selectedStyle.Width(w).Render(prefix + pl))
			case isMatch && j == 0:
				sb.WriteString(warnStyle.Render("▸") + " " + applyColor(pl))
			default:
				sb.WriteString("  " + applyColor(pl))
			}
			sb.WriteByte('\n')
			lineCount++
		}
	}

	return sb.String()
}

// lineLogStyle returns a coloring function based on keywords in the full
// original log line. Called once per logical line so continuation physical
// lines share the same colour.
func lineLogStyle(fullLine string) func(string) string {
	switch {
	case containsAny(fullLine, "ERROR", "FATAL", "CRIT", "error", "fatal", "crit"):
		return func(s string) string { return errStyle.Render(s) }
	case containsAny(fullLine, "WARN", "WARNING", "warn"):
		return func(s string) string { return warnStyle.Render(s) }
	case containsAny(fullLine, "DEBUG", "debug", "TRACE", "trace"):
		return func(s string) string { return dimStyle.Render(s) }
	}
	return func(s string) string { return s }
}
