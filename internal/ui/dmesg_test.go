package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/florianspk/t9s/internal/talos"
)

// kmsgLine builds a line the way machined formats it: "%s: %7s: [%s]: %s".
func kmsgLine(facility, level, ts, msg string) string {
	return fmt.Sprintf("%s: %7s: [%s]: %s", facility, level, ts, msg)
}

const dts = "2026-10-04T12:34:56.123456789Z"

func newDmesgApp(width, height int, lines ...string) App {
	node := talos.Node{Hostname: "cp-1", IP: "10.0.0.2"}
	app := App{
		width: width, height: height, state: StateDmesg,
		selNode: &node, dmesgStreaming: true,
		dmesgCh: make(chan string, 1), findInput: textinput.New(),
	}
	for _, l := range lines {
		model, _ := app.Update(dmesgLineMsg(l))
		app = model.(App)
	}
	return app
}

func dpress(app App, keys ...string) App {
	for _, k := range keys {
		app, _ = app.handleDmesgKey(lkey(k))
	}
	return app
}

func TestDmesgParseEachLevel(t *testing.T) {
	tests := []struct {
		level string
		kind  logSpanKind
	}{
		{"emerg", spanErr}, {"alert", spanErr}, {"crit", spanErr}, {"err", spanErr},
		{"warning", spanWarn}, {"notice", spanInfo}, {"info", spanInfo}, {"debug", spanDebug},
	}
	for _, tc := range tests {
		t.Run(tc.level, func(t *testing.T) {
			line := kmsgLine("kern", tc.level, dts, "usb 1-1: new device")
			d, ok := parseDmesgLine(line)
			if !ok {
				t.Fatalf("not parsed: %q", line)
			}
			if d.levelName != tc.level || d.levelKind != tc.kind {
				t.Errorf("level = %q/%v, want %q/%v", d.levelName, d.levelKind, tc.level, tc.kind)
			}
			if got := line[d.level[0]:d.level[1]]; got != tc.level {
				t.Errorf("level span covers %q", got)
			}
			spans, dim := dmesgSpans(line)
			if dim != (tc.kind == spanDebug) {
				t.Errorf("dim = %v for %s", dim, tc.level)
			}
			var gotLevel logSpanKind = -1
			rs := []rune(line)
			for _, sp := range spans {
				if string(rs[sp.a:sp.b]) == tc.level {
					gotLevel = sp.kind
				}
			}
			if gotLevel != tc.kind {
				t.Errorf("level span kind = %v, want %v", gotLevel, tc.kind)
			}
		})
	}
}

func TestDmesgParseFacilityAndTimestamp(t *testing.T) {
	for _, fac := range []string{"kern", "user", "daemon", "authpriv", "local7"} {
		line := kmsgLine(fac, "info", dts, "hello")
		d, ok := parseDmesgLine(line)
		if !ok || d.facilityTx != fac {
			t.Fatalf("%s: ok=%v facility=%q", fac, ok, d.facilityTx)
		}
		if got := line[d.stamp[0]:d.stamp[1]]; got != dts {
			t.Errorf("%s: stamp = %q", fac, got)
		}
		if !d.tsOK || d.ts != time.Date(2026, 10, 4, 12, 34, 56, 123456789, time.UTC) {
			t.Errorf("%s: ts = %v ok=%v", fac, d.ts, d.tsOK)
		}
	}
	// A node prefix from the CLI is tolerated.
	d, ok := parseDmesgLine("172.30.0.2: " + kmsgLine("kern", "err", dts, "boom"))
	if !ok || d.facilityTx != "kern" || d.levelName != "err" {
		t.Errorf("with node prefix: ok=%v %+v", ok, d)
	}
	if _, ok := parseDmesgLine("ERROR: exec: not found"); ok {
		t.Error("non-dmesg line parsed")
	}
}

func TestDmesgTimestampNormalisedOrArrival(t *testing.T) {
	arrived := time.Date(2026, 10, 4, 13, 0, 0, 0, time.UTC)
	if got := dmesgTimestamp(kmsgLine("kern", "info", dts, "x"), arrived); got != " 2026-10-04T12:34:56.123" {
		t.Errorf("kernel timestamp = %q", got)
	}
	if got := dmesgTimestamp("ERROR: stream failed", arrived); got != "~2026-10-04T13:00:00.000" {
		t.Errorf("fallback = %q", got)
	}
	// A timestamp inside the message must not win over the kernel's.
	if got := dmesgTimestamp(kmsgLine("kern", "info", dts, "rtc 2001-01-01T00:00:00Z"), arrived); !strings.HasPrefix(got, " 2026-10-04") {
		t.Errorf("message timestamp won: %q", got)
	}
}

func TestDmesgColourTouchesLevelFacilityAndStamp(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	app := newDmesgApp(140, 20, kmsgLine("kern", "err", dts, "I/O error"))
	row := app.renderPaneLine(app.logPaneFor(logKindDmesg), 0, false, 3)[0]
	for _, want := range []string{
		logErrStyle.Render("err"),
		logDimStyle.Render("kern"),
		logDimStyle.Render("[" + dts + "]"),
	} {
		if !strings.Contains(row, want) {
			t.Errorf("row lacks %q:\n%q", want, row)
		}
	}
	if got := stripANSI(row); !strings.Contains(got, "I/O error") {
		t.Errorf("message lost: %q", got)
	}

	warn := newDmesgApp(140, 20, kmsgLine("kern", "warning", dts, "slow"))
	if r := warn.renderPaneLine(warn.logPaneFor(logKindDmesg), 0, false, 3)[0]; !strings.Contains(r, logWarnStyle.Render("warning")) {
		t.Errorf("warning not yellow: %q", r)
	}
	info := newDmesgApp(140, 20, kmsgLine("kern", "notice", dts, "ok"))
	if r := info.renderPaneLine(info.logPaneFor(logKindDmesg), 0, false, 3)[0]; !strings.Contains(r, logInfoStyle.Render("notice")) {
		t.Errorf("notice not blue: %q", r)
	}
}

func TestDmesgViewHasIndicatorAndOptions(t *testing.T) {
	app := newDmesgApp(120, 30, kmsgLine("kern", "info", dts, "a"), kmsgLine("kern", "err", dts, "b"))
	out := stripANSI(app.renderDmesg(app.logHeight()))
	if !strings.Contains(out, "Autoscroll:On     FullScreen:Off     Timestamps:Off     Wrap:Off") {
		t.Errorf("indicator missing:\n%s", out)
	}
	app = dpress(app, "t")
	out = stripANSI(app.renderDmesg(app.logHeight()))
	if !strings.Contains(out, "Timestamps:On") || !strings.Contains(out, " 2026-10-04T12:34:56.123") {
		t.Errorf("timestamps:\n%s", out)
	}
	app = dpress(app, "w", "f")
	if !app.logWrap || !app.logFull {
		t.Errorf("w/f not toggled: wrap=%v full=%v", app.logWrap, app.logFull)
	}
	if got := strings.Count(app.View(), "\n") + 1; got != app.height {
		t.Errorf("full screen dmesg height %d, want %d", got, app.height)
	}
}

func TestDmesgAutoscrollFreezeResume(t *testing.T) {
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, kmsgLine("kern", "info", dts, fmt.Sprintf("msg %d", i)))
	}
	app := newDmesgApp(120, 20, lines...)
	app = dpress(app, "s")
	app, _ = func() (App, struct{}) {
		model, _ := app.Update(dmesgLineMsg(kmsgLine("kern", "info", dts, "msg 40")))
		return model.(App), struct{}{}
	}()
	if got := stripANSI(app.paneIndicator(app.logPaneFor(logKindDmesg))); !strings.Contains(got, "+1 new") || !strings.Contains(got, "Autoscroll:Off") {
		t.Errorf("indicator = %q", got)
	}
	app = dpress(app, "G")
	if p := app.logPaneFor(logKindDmesg); p.noFollow || p.cur != 40 {
		t.Errorf("G: follow=%v cur=%d", !p.noFollow, p.cur)
	}
}

func TestDmesgFilterByLevelAndWord(t *testing.T) {
	app := newDmesgApp(120, 30,
		kmsgLine("kern", "info", dts, "eth0: link up"),
		kmsgLine("kern", "err", dts, "nvme0: I/O error"),
		kmsgLine("kern", "warning", dts, "eth0: slow"),
	)
	app = dpress(app, "/", "eth0 !slow", "enter")
	if v := app.logPaneFor(logKindDmesg).text(); len(v) != 1 || !strings.Contains(v[0], "link up") {
		t.Errorf("visible = %v", v)
	}
	if app.dmesgFS.raw != "eth0 !slow" {
		t.Errorf("filter stored on the dmesg pane: %q", app.dmesgFS.raw)
	}
	if app.logFS.raw != "" {
		t.Errorf("dmesg filter leaked into the service logs pane: %q", app.logFS.raw)
	}
	out := stripANSI(app.renderDmesg(app.logHeight()))
	if !strings.Contains(out, "Filter:eth0 !slow (1/3)") || strings.Contains(out, "nvme0") {
		t.Errorf("render:\n%s", out)
	}
	app = dpress(app, "esc")
	if len(app.logPaneFor(logKindDmesg).text()) != 3 {
		t.Error("esc should clear the dmesg filter")
	}
}

// The service logs and dmesg views drive the same pane code: the same keys on
// the same lines leave both in the same state.
func TestLogPaneSharedBehaviour(t *testing.T) {
	keys := []string{"/", "err", "enter", "n", "s", "t", "w", "esc"}
	var lines []string
	for i := 0; i < 12; i++ {
		lines = append(lines, fmt.Sprintf("line %d err=%v", i, i%3 == 0))
	}
	svc := newLogsApp(100, 24, 0)
	dm := newDmesgApp(100, 24)
	for _, l := range lines {
		svc = lfeed(svc, l)
		model, _ := dm.Update(dmesgLineMsg(l))
		dm = model.(App)
	}
	for _, k := range keys {
		svc, _ = svc.handleLogsKey(lkey(k))
		dm, _ = dm.handleDmesgKey(lkey(k))
	}
	a, b := svc.logPaneFor(logKindService), dm.logPaneFor(logKindDmesg)
	if a.cur != b.cur || a.noFollow != b.noFollow || a.n() != b.n() || a.frozenN != b.frozenN {
		t.Errorf("panes diverged: service cur=%d follow=%v n=%d, dmesg cur=%d follow=%v n=%d",
			a.cur, !a.noFollow, a.n(), b.cur, !b.noFollow, b.n())
	}
}
