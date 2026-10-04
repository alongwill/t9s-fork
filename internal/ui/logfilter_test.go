package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestLogFilterGrammar(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		line   string
		want   bool
	}{
		{"word hit", "dns", "resolver: DNS lookup failed", true},
		{"word miss", "dns", "resolver: http ok", false},
		{"words any order", "timeout dns", "dns query timeout after 5s", true},
		{"all words needed", "dns timeout", "dns query ok", false},
		{"case-insensitive", "ERROR", "an error occurred", true},
		{"negation drops", "error !probe", "error: probe failed", false},
		{"negation keeps", "error !probe", "error: disk full", true},
		{"negation only", "!probe", "disk full", true},
		{"negation only drops", "!probe", "Probe ok", false},
		{"lone bang ignored", "dns !", "dns ok", true},
		{"regex hit", "-r ^E\\d+ ", "E1003 12:00 boom", true},
		{"regex miss", "-r ^E\\d+ ", "I1003 12:00 boom", false},
		{"regex case-insensitive", "-r fail(ed|ure)", "Operation FAILURE", true},
		{"invalid regex shows all", "-r [", "anything", true},
		{"fuzzy subsequence", "-f dsknfl", "disk not found: full", false},
		{"fuzzy word-start hit", "-f apid", "apid: starting", true},
		{"fuzzy scattered line hidden", "-f xyz", "a very long line with x then y then z far apart", false},
		{"fuzzy negated", "-f !apid", "apid: starting", false},
		{"fuzzy negated keeps", "-f !apid", "kubelet: starting", true},
		{"empty matches all", "", "x", true},
		{"flag without term", "-f", "x", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseLogFilter(tc.filter).match(tc.line); got != tc.want {
				t.Errorf("filter %q on %q = %v, want %v", tc.filter, tc.line, got, tc.want)
			}
		})
	}
}

func TestLogFilterInvalidRegexFlagged(t *testing.T) {
	f := parseLogFilter("-r [")
	if !f.invalid || f.active() {
		t.Errorf("invalid regex: invalid=%v active=%v, want true/false", f.invalid, f.active())
	}
	if f := parseLogFilter("-r [a-z]+"); f.invalid || !f.active() {
		t.Errorf("valid regex: invalid=%v active=%v", f.invalid, f.active())
	}
}

func filterApp(lines ...string) App {
	app := newLogsApp(120, 30, 0)
	for _, l := range lines {
		app = lfeed(app, l)
	}
	return app
}

func visible(app App) []string { return app.activeLog().text() }

func TestLogFilterNarrowsLiveAsYouType(t *testing.T) {
	app := filterApp("dns ok", "disk full", "dns timeout", "kubelet up")
	app = lpress(app, "/")
	if !app.findActive {
		t.Fatal("/ should open the prompt")
	}
	steps := []struct {
		typed string
		want  int
	}{{"d", 3}, {"n", 2}, {"s", 2}, {" ", 2}, {"t", 1}}
	for _, st := range steps {
		app = lpress(app, st.typed)
		if got := len(visible(app)); got != st.want {
			t.Fatalf("after typing %q: %d visible (%v), want %d", st.typed, got, visible(app), st.want)
		}
	}
	if v := visible(app); v[0] != "dns timeout" {
		t.Errorf("visible = %v", v)
	}
}

func TestLogFilterEnterKeepsEscInPromptRestores(t *testing.T) {
	app := filterApp("dns ok", "disk full", "dns timeout")
	app = lpress(app, "/", "dns", "enter")
	if app.findActive || len(visible(app)) != 2 {
		t.Fatalf("enter should keep the filter: active=%v visible=%v", app.findActive, visible(app))
	}
	// Reopen, change, cancel: the previous filter comes back.
	app = lpress(app, "/")
	if got := app.findInput.Value(); got != "dns" {
		t.Fatalf("prompt should reopen with the filter, got %q", got)
	}
	app = lpress(app, " timeout")
	if len(visible(app)) != 1 {
		t.Fatalf("live narrowing: %v", visible(app))
	}
	app = lpress(app, "esc")
	if app.findActive {
		t.Fatal("esc should close the prompt")
	}
	if app.logFS.raw != "dns" || len(visible(app)) != 2 {
		t.Errorf("esc in prompt should restore %q, got %q (%v)", "dns", app.logFS.raw, visible(app))
	}
}

func TestLogFilterEscOutsidePromptClearsThenLeaves(t *testing.T) {
	app := filterApp("dns ok", "disk full")
	app.logOrigin = StateServices
	app = lpress(app, "/", "disk", "enter")
	if len(visible(app)) != 1 {
		t.Fatalf("visible = %v", visible(app))
	}
	app = lpress(app, "esc")
	if app.logFS.raw != "" || len(visible(app)) != 2 {
		t.Errorf("esc should clear the filter: raw=%q visible=%v", app.logFS.raw, visible(app))
	}
	if app.state != StateLogs {
		t.Errorf("first esc should stay in the logs view, state=%v", app.state)
	}
	app = lpress(app, "esc")
	if app.state == StateLogs {
		t.Error("second esc should leave the logs view")
	}
}

func TestLogFilterEscKeepsPlaceOnClear(t *testing.T) {
	app := filterApp("a1", "b2", "a3", "b4", "a5")
	app = lpress(app, "/", "b", "enter", "g") // cursor on "b2"
	if p := app.activeLog(); p.line(p.cur) != "b2" {
		t.Fatalf("cursor on %q", p.line(p.cur))
	}
	app = lpress(app, "esc")
	if p := app.activeLog(); p.line(p.cur) != "b2" {
		t.Errorf("after clearing, cursor on %q, want b2", p.line(p.cur))
	}
}

func TestLogFilterNewLinesTestedAsTheyArrive(t *testing.T) {
	app := filterApp("dns ok", "disk full")
	app = lpress(app, "/", "dns", "enter")
	app = lfeed(app, "kubelet up")
	app = lfeed(app, "dns timeout")
	if v := visible(app); len(v) != 2 || v[1] != "dns timeout" {
		t.Fatalf("visible = %v", v)
	}
	if len(app.logLines) != 4 {
		t.Errorf("buffer should keep every line, got %d", len(app.logLines))
	}
	// Clearing brings the hidden ones back.
	app = lpress(app, "esc")
	if len(visible(app)) != 4 {
		t.Errorf("after clear: %v", visible(app))
	}
}

func TestLogFilterAutoscrollFollowsMatchingLines(t *testing.T) {
	app := filterApp("dns 1", "other", "dns 2")
	app = lpress(app, "/", "dns", "enter")
	app = lfeed(app, "other 2")
	app = lfeed(app, "dns 3")
	out := stripANSI(app.renderLogs(app.logHeight()))
	if !strings.Contains(out, "▶ dns 3") {
		t.Errorf("cursor should follow the newest matching line:\n%s", out)
	}
	if strings.Contains(out, "other") {
		t.Errorf("hidden lines rendered:\n%s", out)
	}
	// Frozen: only matching arrivals count as new.
	app = lpress(app, "s")
	app = lfeed(app, "other 3")
	if got := stripANSI(app.logIndicator()); strings.Contains(got, "new") {
		t.Errorf("a hidden line counted as new: %q", got)
	}
	app = lfeed(app, "dns 4")
	if got := stripANSI(app.logIndicator()); !strings.Contains(got, "+1 new") {
		t.Errorf("indicator = %q, want +1 new", got)
	}
	app = lpress(app, "G")
	if p := app.activeLog(); p.line(p.cur) != "dns 4" || p.noFollow {
		t.Errorf("G should resume on the last match, got %q follow=%v", p.line(p.cur), !p.noFollow)
	}
}

func TestLogFilterIndicatorShowsCounts(t *testing.T) {
	app := filterApp("dns ok", "disk full", "dns timeout")
	app = lpress(app, "/", "dns", "enter")
	got := stripANSI(app.logIndicator())
	if !strings.Contains(got, "Filter:dns (2/3)") {
		t.Errorf("indicator = %q, want Filter:dns (2/3)", got)
	}
	app = lpress(app, "esc")
	if got := stripANSI(app.logIndicator()); strings.Contains(got, "Filter") {
		t.Errorf("indicator still shows a filter: %q", got)
	}
	app = lpress(app, "/", "-r [", "enter")
	if got := stripANSI(app.logIndicator()); !strings.Contains(got, "invalid regex") {
		t.Errorf("indicator = %q, want invalid regex note", got)
	}
}

func TestLogFilterNextPrevStepThroughVisible(t *testing.T) {
	app := filterApp("a1", "b", "a2", "b", "a3")
	app = lpress(app, "/", "a", "enter", "g")
	for i, want := range []string{"a2", "a3", "a1"} {
		app = lpress(app, "n")
		if p := app.activeLog(); p.line(p.cur) != want {
			t.Fatalf("n #%d: on %q, want %q", i, p.line(p.cur), want)
		}
	}
	app = lpress(app, "N")
	if p := app.activeLog(); p.line(p.cur) != "a3" {
		t.Errorf("N: on %q, want a3", p.line(p.cur))
	}
}

func TestLogFilterNoMatchesRendersMessage(t *testing.T) {
	app := filterApp("dns ok", "disk full")
	app = lpress(app, "/", "zzz", "enter")
	out := stripANSI(app.renderLogs(app.logHeight()))
	if !strings.Contains(out, "No lines match (0/2)") {
		t.Errorf("render:\n%s", out)
	}
}

func TestLogFilterHighlightsTerms(t *testing.T) {
	app := filterApp("dns timeout ok")
	app = lpress(app, "/", "dns", "enter")
	row := app.renderLogLine(0, false, 3)[0]
	if !strings.Contains(row, logFindStyle.Render("dns")) && !strings.Contains(stripANSI(row), "dns timeout ok") {
		t.Errorf("row = %q", row)
	}
}

func TestLogFilterHeightBudgetWithPrompt(t *testing.T) {
	var lines []string
	for i := 0; i < 80; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	app := filterApp(lines...)
	app = lpress(app, "/", "line")
	out := app.renderLogs(app.logHeight())
	if got := lineCount(out); got > app.logHeight() {
		t.Errorf("height %d > %d with prompt open", got, app.logHeight())
	}
}
