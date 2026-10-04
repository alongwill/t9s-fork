package ui

import (
	"github.com/charmbracelet/lipgloss"
	"strings"
	"testing"
)

func typesAppAt(t *testing.T, display string) App {
	t.Helper()
	app := appWithTop(paneTypes)
	i := typeIndex(t, app, display)
	app.browser = app.browser.withTop(func(p *pane) { p.cur = i })
	return app
}

func TestNextStepTypesPane(t *testing.T) {
	app := typesAppAt(t, "Thing02") // 3 instances
	app.source = newFakeSource("grpc")
	want := "↵ 3 instances · d what is this · c compare (on an instance) · W watch (on an instance)"
	if got := app.nextStep(); got != want {
		t.Errorf("multi-instance row:\n got %q\nwant %q", got, want)
	}
	app.source = newFakeSource("cli")
	if got := app.nextStep(); strings.Contains(got, "W watch") {
		t.Errorf("W offered without gRPC: %q", got)
	}
	// greyed (absent) row
	if got := typesAppAt(t, "Thing01").nextStep(); got != "not on this node · d what is this" {
		t.Errorf("absent row: %q", got)
	}
	// one instance
	if got := typesAppAt(t, "Thing00").nextStep(); !strings.HasPrefix(got, "↵ open it") {
		t.Errorf("single row: %q", got)
	}
	// locked and uncounted
	app = typesAppAt(t, "Thing02")
	app.browser = app.browser.setCount("Thing02.net.talos.dev", countLocked)
	if got := app.nextStep(); !strings.HasPrefix(got, padlock()+" needs os:admin") {
		t.Errorf("locked row: %q", got)
	}
	delete(app.browser.counts, "Thing02.net.talos.dev")
	if got := app.nextStep(); !strings.HasPrefix(got, "counting") {
		t.Errorf("uncounted row: %q", got)
	}
}

func TestNextStepPerPaneKind(t *testing.T) {
	for _, kind := range allPaneKinds {
		app := appWithTop(kind)
		if app.nextStep() == "" {
			t.Errorf("pane %d has no next-step text", kind)
		}
	}
	if got := appWithTop(paneCategories).nextStep(); !strings.HasPrefix(got, "↵ open Networking") {
		t.Errorf("categories: %q", got)
	}
}

func TestNextStepHiddenWhenShort(t *testing.T) {
	app := appWithTop(paneTypes)
	app.height = 19
	if app.nextStepRows() != 0 {
		t.Error("next-step line shown at height 19")
	}
	app.height = 20
	if app.nextStepRows() != 1 {
		t.Error("next-step line hidden at height 20")
	}
}

func TestRenderBudgetWithNextStep(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}, {80, 19}} {
		for _, depth := range []int{1, 2, 3, 4} {
			app := browserApp(sz.w, sz.h, depth, 30)
			out := checkBudget(t, app, sz.w)
			has := strings.Contains(out, "d what is this") || strings.Contains(out, "/ find") || strings.Contains(out, "↵ open")
			if want := sz.h >= minNextStepHeight; has != want {
				t.Errorf("%dx%d depth %d: next-step line present=%v, want %v\n%s", sz.w, sz.h, depth, has, want, out)
			}
			// the cursor still fits in the shrunken panes
			app = press(t, app, "G")
			checkBudget(t, app, sz.w)
		}
	}
}

func TestTipRotation(t *testing.T) {
	old := tipStart
	tipStart = func(n int) int { return n - 2 }
	t.Cleanup(func() { tipStart = old })

	app := appWithTop(paneCategories)
	app.tipIdx = -1
	var seen []string
	for i := 0; i < 3; i++ {
		app.statusMsg = ""
		app = app.showTip()
		seen = append(seen, app.statusMsg)
	}
	n := len(browserTips)
	for i, want := range []string{browserTips[n-2], browserTips[n-1], browserTips[0]} {
		if !strings.Contains(seen[i], want) {
			t.Errorf("tip %d = %q, want %q", i, seen[i], want)
		}
	}
	// a leftover tip does not block the next one; another message does
	app = app.showTip()
	if !strings.Contains(app.statusMsg, browserTips[1]) {
		t.Errorf("leftover tip blocked rotation: %q", app.statusMsg)
	}
	app.statusMsg = "something else"
	if got := app.showTip().statusMsg; got != "something else" {
		t.Errorf("tip overwrote a status message: %q", got)
	}
}

func TestTipsOffSilencesTips(t *testing.T) {
	app := appWithTop(paneCategories)
	app.tipIdx = 0
	app, _ = app.runCommand("tips off")
	if !app.tipsOff {
		t.Fatal(":tips off did not set tipsOff")
	}
	app.statusMsg = ""
	if got := app.showTip().statusMsg; got != "" {
		t.Errorf("tip shown while off: %q", got)
	}
	// opening a category must stay quiet too
	app.statusMsg = ""
	app, _ = app.browserEnter()
	if strings.Contains(app.statusMsg, "tip:") {
		t.Errorf("category open showed a tip: %q", app.statusMsg)
	}
	app, _ = app.runCommand("tips on")
	if app.tipsOff || app.showTip().statusMsg == "" {
		t.Error(":tips on did not restore tips")
	}
}

func TestTipsOnOpenAndCategory(t *testing.T) {
	app := appWithTop(paneCategories)
	app.tipIdx = 0
	app, _ = app.browserEnter()
	if !strings.Contains(app.statusMsg, "tip: ") {
		t.Errorf("no tip when a category opened: %q", app.statusMsg)
	}
	// real list lines are short enough for 80 columns
	for i, tip := range browserTips {
		if len(tip) > 100 {
			t.Errorf("tip %d is %d chars, keep tips to one line", i, len(tip))
		}
	}
}

func TestTipFitsNarrowTerminal(t *testing.T) {
	app := appWithTop(paneCategories)
	app.width = 60
	for range browserTips {
		app.statusMsg = ""
		app = app.showTip()
		if w := lipgloss.Width(app.statusMsg); w > app.width-4 {
			t.Errorf("tip is %d cells wide in a %d-column terminal: %q", w, app.width, app.statusMsg)
		}
	}
}
