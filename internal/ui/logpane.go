package ui

import "time"

// logKind says which stream a logPane shows. It only picks the colouring and
// timestamp rules; the pane behaves the same for both.
type logKind int

const (
	logKindService logKind = iota
	logKindDmesg
)

// logPane is the state of one scrolling log view (service logs or dmesg):
// the buffered lines, the cursor, autoscroll and the filter. cur, top and
// frozenN count visible lines: all of them without a filter, the filtered
// subset with one.
type logPane struct {
	kind     logKind
	lines    []string
	arrived  []time.Time
	cur      int
	top      int // first visible line shown while autoscroll is off
	frozenN  int // visible count when autoscroll was switched off
	noFollow bool
	fs       logFilterState
}

// logPaneFor assembles the pane of a stream from the App fields.
func (app App) logPaneFor(k logKind) logPane {
	if k == logKindDmesg {
		return logPane{
			kind: k, lines: app.dmesgLines, arrived: app.dmesgArrived, cur: app.dmesgCur,
			top: app.dmesgTop, frozenN: app.dmesgFrozenN, noFollow: app.dmesgNoFollow, fs: app.dmesgFS,
		}.sync()
	}
	return logPane{
		kind: k, lines: app.logLines, arrived: app.logArrived, cur: app.logCur,
		top: app.logTop, frozenN: app.logFrozenN, noFollow: app.logNoFollow, fs: app.logFS,
	}.sync()
}

// setLogPane writes a pane back to the App fields of its stream.
func (app App) setLogPane(p logPane) App {
	if p.kind == logKindDmesg {
		app.dmesgLines, app.dmesgArrived, app.dmesgCur = p.lines, p.arrived, p.cur
		app.dmesgTop, app.dmesgFrozenN, app.dmesgNoFollow, app.dmesgFS = p.top, p.frozenN, p.noFollow, p.fs
		return app
	}
	app.logLines, app.logArrived, app.logCur = p.lines, p.arrived, p.cur
	app.logTop, app.logFrozenN, app.logNoFollow, app.logFS = p.top, p.frozenN, p.noFollow, p.fs
	return app
}

// activeLog is the pane the current state shows.
func (app App) activeLog() logPane {
	if app.state == StateDmesg {
		return app.logPaneFor(logKindDmesg)
	}
	return app.logPaneFor(logKindService)
}

// sync brings the filter index up to date with the buffer.
func (p logPane) sync() logPane {
	p.fs = p.fs.sync(p.lines)
	return p
}

func (p logPane) filtered() bool { return p.fs.f.active() }

// n is the number of visible lines.
func (p logPane) n() int {
	if p.filtered() {
		return len(p.fs.vis)
	}
	return len(p.lines)
}

// orig maps a visible index to an index into lines.
func (p logPane) orig(i int) int {
	if p.filtered() {
		return p.fs.vis[i]
	}
	return i
}

// line is visible line i.
func (p logPane) line(i int) string { return p.lines[p.orig(i)] }

// text is the visible lines as a slice.
func (p logPane) text() []string {
	if !p.filtered() {
		return p.lines
	}
	out := make([]string, len(p.fs.vis))
	for i, j := range p.fs.vis {
		out[i] = p.lines[j]
	}
	return out
}

// arrival returns when visible line i arrived (zero when unknown).
func (p logPane) arrival(i int) time.Time {
	j := p.orig(i)
	if j < len(p.arrived) {
		return p.arrived[j]
	}
	return time.Time{}
}

// appended is called after a line was added to lines and arrived: it indexes
// the line and keeps the cursor on the tail while autoscroll is on.
func (p logPane) appended() logPane {
	p = p.sync()
	if !p.noFollow {
		p.cur = max(0, p.n()-1)
	}
	return p
}

// freeze stops autoscroll, keeping the viewport where it is.
func (p logPane) freeze(lay logLayout, rows int) logPane {
	if p.noFollow {
		return p
	}
	n := p.n()
	p.top = lay.topFor(p.text(), n-1, rows)
	p.noFollow = true
	p.frozenN = n
	return p
}

// resume turns autoscroll back on and jumps to the tail.
func (p logPane) resume() logPane {
	p.noFollow = false
	p.frozenN = 0
	p.cur = max(0, p.n()-1)
	return p
}

// move puts the cursor on visible line idx. Anywhere but the tail turns
// autoscroll off, as in k9s.
func (p logPane) move(lay logLayout, rows, idx int) logPane {
	n := p.n()
	if n == 0 {
		return p
	}
	idx = min(max(idx, 0), n-1)
	if !p.noFollow && idx == n-1 {
		p.cur = idx
		return p
	}
	p = p.freeze(lay, rows)
	p.cur = idx
	p.top = lay.ensureVisible(p.text(), p.top, idx, rows)
	return p
}

// newCount is the number of visible lines that arrived while autoscroll is off.
func (p logPane) newCount() int {
	if !p.noFollow {
		return 0
	}
	return max(0, p.n()-p.frozenN)
}

// setFilter replaces the filter text. The cursor stays on the same line, or
// the next visible one, so narrowing as you type does not lose your place.
func (p logPane) setFilter(raw string) logPane {
	var curOrig int
	if p.n() > 0 {
		curOrig = p.orig(min(max(p.cur, 0), p.n()-1))
	}
	p.fs = p.fs.withText(raw, p.lines)
	n := p.n()
	switch {
	case n == 0:
		p.cur = 0
	case !p.filtered():
		p.cur = min(curOrig, n-1)
	default:
		p.cur = n - 1
		for k, j := range p.fs.vis {
			if j >= curOrig {
				p.cur = k
				break
			}
		}
	}
	p.top = min(p.top, max(0, n-1))
	if p.noFollow {
		p.frozenN = n
	} else {
		p.cur = max(0, n-1)
	}
	return p
}

// step moves to the next (dir 1) or previous (dir -1) visible line, wrapping.
func (p logPane) step(lay logLayout, rows, dir int) logPane {
	n := p.n()
	if n == 0 || !p.filtered() {
		return p
	}
	return p.move(lay, rows, ((p.cur+dir)%n+n)%n)
}
