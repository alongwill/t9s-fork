package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/florianspk/t9s/internal/talos"
)

func lkey(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func newLogsApp(width, height, nLines int) App {
	node := talos.Node{Hostname: "cp-1", IP: "10.0.0.2"}
	app := App{
		width: width, height: height, state: StateLogs,
		selNode: &node, logService: "apid", logStreaming: true,
		logCh: make(chan string, 1),
	}
	for i := 0; i < nLines; i++ {
		app.logLines = append(app.logLines, fmt.Sprintf("line %d", i))
		app.logArrived = append(app.logArrived, time.Date(2026, 10, 3, 12, 0, i%60, 0, time.UTC))
	}
	app.logCur = max(0, nLines-1)
	return app
}

func lpress(app App, keys ...string) App {
	for _, k := range keys {
		app, _ = app.handleLogsKey(lkey(k))
	}
	return app
}

func lfeed(app App, line string) App {
	model, _ := app.Update(logLineMsg{line: line, sessionSeq: app.logSessionSeq})
	return model.(App)
}

func TestLogIndicatorStates(t *testing.T) {
	app := newLogsApp(120, 30, 3)
	if got := stripANSI(app.logIndicator()); strings.TrimSpace(got) != "Autoscroll:On     FullScreen:Off     Timestamps:Off     Wrap:Off" {
		t.Fatalf("default indicator = %q", got)
	}
	app = lpress(app, "s", "f", "t", "w")
	// f only flips the flag here; esc leaves full screen.
	want := "Autoscroll:Off     FullScreen:On     Timestamps:On     Wrap:On"
	if got := strings.TrimSpace(stripANSI(app.logIndicator())); got != want {
		t.Fatalf("indicator = %q, want %q", got, want)
	}
}

func TestLogAutoscrollFreezeNewCountAndResume(t *testing.T) {
	app := newLogsApp(120, 30, 100)
	app = lpress(app, "s")
	if !app.logNoFollow {
		t.Fatal("s did not turn Autoscroll off")
	}
	cur, top := app.logCur, app.logTop
	for i := 0; i < 5; i++ {
		app = lfeed(app, fmt.Sprintf("new %d", i))
	}
	if app.logCur != cur || app.logTop != top {
		t.Fatalf("view moved while frozen: cur %d->%d top %d->%d", cur, app.logCur, top, app.logTop)
	}
	if got := stripANSI(app.logIndicator()); !strings.Contains(got, "+5 new") {
		t.Fatalf("indicator lacks +5 new: %q", got)
	}
	if out := stripANSI(app.renderLogs(app.logHeight())); strings.Contains(out, "new 4") {
		t.Fatal("frozen view shows a line that arrived after the freeze")
	}
	app = lpress(app, "G")
	if app.logNoFollow || app.logCur != len(app.logLines)-1 {
		t.Fatalf("G did not resume: follow=%v cur=%d", !app.logNoFollow, app.logCur)
	}
	if got := stripANSI(app.logIndicator()); strings.Contains(got, "new") {
		t.Fatalf("indicator still counts new lines: %q", got)
	}
	app = lfeed(app, "tail")
	if app.logCur != len(app.logLines)-1 {
		t.Fatal("cursor did not follow the tail after resume")
	}
	if out := stripANSI(app.renderLogs(app.logHeight())); !strings.Contains(out, "tail") {
		t.Fatal("tail line not visible while following")
	}
}

func TestLogScrollUpTurnsAutoscrollOff(t *testing.T) {
	for _, k := range []string{"k", "up", "g", "pgup"} {
		app := newLogsApp(120, 30, 100)
		app = lpress(app, k)
		if !app.logNoFollow {
			t.Errorf("%q kept Autoscroll on", k)
		}
	}
	app := lpress(newLogsApp(120, 30, 100), "k", "end")
	if app.logNoFollow {
		t.Error("end did not turn Autoscroll back on")
	}
}

func TestLogParseTimestamp(t *testing.T) {
	ref := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	cases := []struct {
		name, line, want string
	}{
		{"rfc3339 z", "2026-10-03T12:00:01.234Z hello", "2026-10-03T12:00:01.234"},
		{"rfc3339 offset", "2026-10-03T14:00:01+02:00 hello", "2026-10-03T12:00:01.000"},
		{"space separated", "x 2026-10-03 12:00:01 y", "2026-10-03T12:00:01.000"},
		{"go log", "2026/10/03 12:00:01 started", "2026-10-03T12:00:01.000"},
		{"go log frac", "2026/10/03 12:00:01.5 started", "2026-10-03T12:00:01.500"},
		{"klog", "I1003 12:00:01.123456   123 main.go:1] ok", "2026-10-03T12:00:01.123"},
		{"json ts string", `{"level":"info","ts":"2026-10-03T12:00:01.2Z"}`, "2026-10-03T12:00:01.200"},
		{"json time", `{"time":"2026-10-03T12:00:01Z","msg":"x"}`, "2026-10-03T12:00:01.000"},
		{"json epoch", `{"ts":1791028801.25,"msg":"x"}`, "2026-10-03T12:00:01.250"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := logTimestamp(tc.line, ref)
			if got != " "+tc.want {
				t.Fatalf("got %q, want %q", got, " "+tc.want)
			}
		})
	}
	// No timestamp in the line: arrival time with a leading ~.
	arrived := time.Date(2026, 10, 3, 12, 30, 0, 500e6, time.UTC)
	if got := logTimestamp("plain message", arrived); got != "~2026-10-03T12:30:00.500" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestLogTimestampsColumnRendered(t *testing.T) {
	app := newLogsApp(100, 20, 2)
	app.logLines[1] = "2026-10-03T08:00:00Z from the line"
	app = lpress(app, "t")
	out := stripANSI(app.renderLogs(app.logHeight()))
	if !strings.Contains(out, "~2026-10-03T12:00:00.000 line 0") {
		t.Errorf("arrival time missing:\n%s", out)
	}
	if !strings.Contains(out, " 2026-10-03T08:00:00.000 2026-10-03T08:00:00Z from the line") {
		t.Errorf("parsed time missing:\n%s", out)
	}
	if got := maxLineWidth(out); got > 100 {
		t.Errorf("line width %d > 100", got)
	}
}

func TestLogWrapKeepsLineCountRight(t *testing.T) {
	long := strings.Repeat("abcdefghij", 12) // 120 runes
	app := newLogsApp(42, 20, 3)             // avail = 40 -> 3 rows
	app.logLines[1] = long
	lay := app.logLayout()
	if lay.rows(long) != 1 {
		t.Fatalf("wrap off: rows = %d, want 1", lay.rows(long))
	}
	app = lpress(app, "w")
	lay = app.logLayout()
	if got := lay.rows(long); got != 3 {
		t.Fatalf("wrap on: rows = %d, want 3", got)
	}
	out := app.renderLogs(app.logHeight())
	if got := strings.Count(stripANSI(out), "abcdefghij"); got != 12 {
		t.Errorf("wrapped text lost or duplicated: %d chunks of 10, want 12", got)
	}
	if lineCount(out) > app.logHeight() {
		t.Errorf("height %d > %d", lineCount(out), app.logHeight())
	}
	if w := maxLineWidth(out); w > 42 {
		t.Errorf("line width %d > 42", w)
	}
	// Cursor and find still address logical lines.
	app = lpress(app, "g")
	if app.logCur != 0 {
		t.Fatalf("g: cursor %d", app.logCur)
	}
	app.findQuery = "abcdefghij"
	app = lpress(app, "n")
	if app.logCur != 1 {
		t.Fatalf("find n: cursor %d, want logical line 1", app.logCur)
	}
	// Off: cut with an ellipsis.
	app = lpress(app, "w")
	out = stripANSI(app.renderLogs(app.logHeight()))
	if !strings.Contains(out, "…") {
		t.Errorf("cut line lacks ellipsis:\n%s", out)
	}
}

func TestLogWrapTailStaysVisible(t *testing.T) {
	app := newLogsApp(42, 14, 0)
	for i := 0; i < 30; i++ {
		app.logLines = append(app.logLines, fmt.Sprintf("%02d %s", i, strings.Repeat("x", 100)))
	}
	app.logCur = 29
	app = lpress(app, "w")
	out := stripANSI(app.renderLogs(app.logHeight()))
	if !strings.Contains(out, "29 xxx") {
		t.Errorf("tail line not visible:\n%s", out)
	}
	if lineCount(out) > app.logHeight() {
		t.Errorf("height %d > %d", lineCount(out), app.logHeight())
	}
}

func TestLogLevelSpans(t *testing.T) {
	cases := []struct {
		line string
		kind logSpanKind
		text string
		dim  bool
	}{
		{"2026-10-03T12:00:00Z ERROR something broke", spanErr, "ERROR", false},
		{"FATAL: out of memory", spanErr, "FATAL", false},
		{"crit disk gone", spanErr, "crit", false},
		{"WARN slow", spanWarn, "WARN", false},
		{"something warning: x", spanWarn, "warning", false},
		{"INFO started", spanInfo, "INFO", false},
		{"started info", spanInfo, "info", false},
		{"DEBUG detail", spanDebug, "DEBUG", true},
		{"TRACE detail", spanDebug, "TRACE", true},
		{"level=info msg=hi", spanInfo, "info", false},
		{`level="warn" msg=hi`, spanWarn, "warn", false},
		{`{"level":"error","msg":"x"}`, spanErr, "error", false},
		{"[INFO] ready", spanInfo, "[INFO]", false},
		{"[ERROR] bad", spanErr, "[ERROR]", false},
		{"E1003 12:00:00.123456 1 f.go:1] boom", spanErr, "E1003", false},
		{"W1003 12:00:00.123456 1 f.go:1] hmm", spanWarn, "W1003", false},
		{"I1003 12:00:00.123456 1 f.go:1] ok", spanInfo, "I1003", false},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			spans, dim := logSpans(tc.line)
			if dim != tc.dim {
				t.Errorf("dim = %v, want %v", dim, tc.dim)
			}
			rs := []rune(tc.line)
			for _, sp := range spans {
				if sp.kind == tc.kind && string(rs[sp.a:sp.b]) == tc.text {
					return
				}
			}
			t.Fatalf("no %v span over %q in %+v", tc.kind, tc.text, spans)
		})
	}
	if spans, _ := logSpans("all quiet here"); len(spans) != 0 {
		t.Errorf("plain line got spans: %+v", spans)
	}
}

func TestLogTimestampAndKeySpans(t *testing.T) {
	line := "2026-10-03T12:00:00Z level=info msg=hello user=bob"
	spans, _ := logSpans(line)
	rs := []rune(line)
	got := map[logSpanKind][]string{}
	for _, sp := range spans {
		got[sp.kind] = append(got[sp.kind], string(rs[sp.a:sp.b]))
	}
	if len(got[spanTime]) != 1 || got[spanTime][0] != "2026-10-03T12:00:00Z" {
		t.Errorf("time spans = %v", got[spanTime])
	}
	if strings.Join(got[spanKey], ",") != "level,msg,user" {
		t.Errorf("key spans = %v", got[spanKey])
	}
}

func TestLogColourTouchesOnlyTheLevelToken(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	app := newLogsApp(120, 20, 0)
	app.logLines = []string{"2026-10-03T12:00:00Z ERROR disk failed"}
	app.logCur = -1 // cursor off the line so it renders unselected
	app.logNoFollow = true
	app.logCur = 0
	app.logLines = append(app.logLines, "second")
	rows := app.renderLogLine(0, false, 5)
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	row := rows[0]
	if stripANSI(row) != "  2026-10-03T12:00:00Z ERROR disk failed" {
		t.Fatalf("text changed: %q", stripANSI(row))
	}
	// The red-bold style is applied to ERROR only: the text after it carries
	// no colour of its own.
	idx := strings.Index(row, "ERROR")
	if idx < 0 {
		t.Fatalf("ERROR split by escapes: %q", row)
	}
	if !strings.Contains(row[:idx], "\x1b[") {
		t.Errorf("no style before ERROR: %q", row)
	}
	tail := row[idx+len("ERROR"):]
	if !strings.HasPrefix(tail, "\x1b[0m") {
		t.Errorf("style does not end right after ERROR: %q", tail)
	}
	if strings.Contains(strings.TrimPrefix(tail, "\x1b[0m"), "\x1b[") {
		t.Errorf("text after the level token is styled: %q", tail)
	}
	// DEBUG dims the whole line.
	app.logLines[0] = "DEBUG all of this is dim"
	row = app.renderLogLine(0, false, 5)[0]
	if !strings.HasPrefix(strings.TrimPrefix(row, "  "), "\x1b[") {
		t.Errorf("DEBUG line not styled from the start: %q", row)
	}
}

func TestLogFindHighlightSurvivesColour(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	app := newLogsApp(120, 20, 2)
	app.logLines[0] = "ERROR disk failed"
	app.findQuery = "disk"
	app.logNoFollow = true
	app.logCur = 1
	row := app.renderLogLine(0, false, 5)[0]
	hl := logFindStyle.Render("disk")
	if !strings.Contains(row, hl) {
		t.Errorf("find match not highlighted in %q (want %q)", row, hl)
	}
	// Also on the selected row.
	row = app.renderLogLine(0, true, 5)[0]
	if !strings.Contains(row, hl) {
		t.Errorf("find match not highlighted on selected row: %q", row)
	}
}

func TestLogFullScreenHeightBudget(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 40}, {200, 50}} {
		app := newLogsApp(sz[0], sz[1], 300)
		app = lpress(app, "f")
		if !app.logFull {
			t.Fatal("f did not enable FullScreen")
		}
		out := app.View()
		if got := strings.Count(out, "\n") + 1; got != sz[1] {
			t.Errorf("%dx%d: %d rows, want %d", sz[0], sz[1], got, sz[1])
		}
		if w := maxLineWidth(out); w > sz[0] {
			t.Errorf("%dx%d: line width %d", sz[0], sz[1], w)
		}
		plain := stripANSI(out)
		if strings.Contains(plain, " t9s ") {
			t.Errorf("%dx%d: header visible in full screen", sz[0], sz[1])
		}
		if !strings.Contains(plain, "line 299") || !strings.Contains(plain, "FullScreen:On") {
			t.Errorf("%dx%d: tail or indicator missing", sz[0], sz[1])
		}
	}
	// Esc leaves full screen before leaving logs.
	app := lpress(newLogsApp(80, 24, 3), "f", "esc")
	if app.logFull || app.state != StateLogs {
		t.Errorf("esc: full=%v state=%v", app.logFull, app.state)
	}
}

func TestLogNormalHeightBudget(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 40}} {
		app := newLogsApp(sz[0], sz[1], 300)
		for _, toggles := range [][]string{nil, {"t"}, {"w"}, {"t", "w", "s"}} {
			a := lpress(app, toggles...)
			out := a.renderLogs(a.mainHeight())
			if got := lineCount(out); got > a.mainHeight() {
				t.Errorf("%dx%d %v: %d rows > %d", sz[0], sz[1], toggles, got, a.mainHeight())
			}
			if w := maxLineWidth(out); w > sz[0] {
				t.Errorf("%dx%d %v: width %d", sz[0], sz[1], toggles, w)
			}
		}
	}
}
