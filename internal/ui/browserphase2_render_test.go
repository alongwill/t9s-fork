package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/catalog"
)

func checkBudget(t *testing.T, app App, w int) string {
	t.Helper()
	out := app.renderBrowser(app.mainHeight())
	if got := lineCount(out); got != app.mainHeight() {
		t.Errorf("output has %d lines, want exactly %d\n%s", got, app.mainHeight(), out)
	}
	if got := maxLineWidth(out); got > w {
		t.Errorf("a line is %d cells wide, terminal is %d\n%s", got, w, out)
	}
	return out
}

func TestRenderTwoSectionTypesPane(t *testing.T) {
	for _, sz := range browserSizes {
		sz := sz
		t.Run(fmt.Sprintf("w%d_h%d", sz.w, sz.h), func(t *testing.T) {
			app := cfgApp(sz.w, sz.h, 60)
			out := checkBudget(t, app, sz.w)
			if !strings.Contains(out, "CONFIG") {
				t.Errorf("first header not visible at the top:\n%s", out)
			}
			n := len(app.browser.typeEntries(testNet, ""))
			if n < app.paneInnerRows(paneTypes)+10 {
				t.Fatalf("fixture too small to scroll: %d entries", n)
			}
			// walk past the config section, across the boundary, to the end
			for i := 0; i < n+3; i++ {
				app = press(t, app, "down")
				p, _ := app.browser.top()
				e := app.browser.typeEntries(testNet, "")[p.cur]
				out = checkBudget(t, app, sz.w)
				if !strings.Contains(out, "▶ "+cutWidth(e.name(), 14)) {
					t.Fatalf("step %d: cursor row %q not visible (cur=%d scroll=%d)\n%s", i, e.name(), p.cur, p.scroll, out)
				}
			}
			if !strings.Contains(out, "RESOURCES") && !strings.Contains(out, "Thing") {
				t.Errorf("resources section never rendered:\n%s", out)
			}
			// and back to the top: the CONFIG header comes back into view
			app = press(t, app, "g")
			if out = checkBudget(t, app, sz.w); !strings.Contains(out, "CONFIG") {
				t.Errorf("g did not bring the CONFIG header back:\n%s", out)
			}
		})
	}
}

func TestRenderDescribePane(t *testing.T) {
	for _, sz := range browserSizes {
		for _, kind := range []string{"config", "resource"} {
			sz, kind := sz, kind
			t.Run(fmt.Sprintf("w%d_h%d_%s", sz.w, sz.h, kind), func(t *testing.T) {
				app := cfgApp(sz.w, sz.h, 6)
				if kind == "resource" {
					nCfg := len(app.browser.configEntries(testNet, ""))
					for i := 0; i < nCfg; i++ {
						app = press(t, app, "down")
					}
				}
				app = press(t, app, "d")
				out := checkBudget(t, app, sz.w)
				if !strings.Contains(out, "Describe") {
					t.Errorf("title missing:\n%s", out)
				}
				for _, k := range []string{"G", "g", "ctrl+f"} {
					if k == "ctrl+f" {
						app, _ = app.handleKey(keyCtrl('f'))
					} else {
						app = press(t, app, k)
					}
					checkBudget(t, app, sz.w)
				}
			})
		}
	}
}

func TestRenderDescribeLongTextWrapsAndScrolls(t *testing.T) {
	app := cfgApp(80, 24, 3)
	// CPUScalingConfig-style long descriptions: wrap inside the pane, scroll to the end
	sub := descSubject{cfg: true, ck: catalog.ConfigKind{Kind: "X", Group: "network", Desc: strings.Repeat("word ", 400)}}
	app.browser = app.browser.push(pane{kind: paneDescribe, title: "Describe X", sub: sub})
	app = app.syncBrowserState()
	out := checkBudget(t, app, 80)
	app = press(t, app, "G")
	p, _ := app.browser.top()
	if p.scroll == 0 {
		t.Errorf("long text did not scroll (len=%d)", app.paneLen(p))
	}
	checkBudget(t, app, 80)
	if !strings.Contains(out, "word") {
		t.Error("text not rendered")
	}
}

func TestRenderPalette(t *testing.T) {
	for _, sz := range browserSizes {
		sz := sz
		t.Run(fmt.Sprintf("w%d_h%d", sz.w, sz.h), func(t *testing.T) {
			app := cfgApp(sz.w, sz.h, 60)
			app, _ = ctrlA(t, app)
			out := checkBudget(t, app, sz.w)
			for _, want := range []string{"NAME", "COUNT", "Aliases ("} {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q:\n%s", want, out)
				}
			}
			if !strings.Contains(out, "▶ ") {
				t.Errorf("no cursor row:\n%s", out)
			}
			rows := app.paneInnerRows(paneAliases)
			for i := 0; i < rows+15; i++ {
				app = press(t, app, "down")
			}
			p, _ := app.browser.top()
			e := app.browser.paletteRows(p.filter)[p.cur]
			out = checkBudget(t, app, sz.w)
			if !strings.Contains(out, "▶ "+e.name) {
				t.Errorf("cursor row %q not visible after scrolling (cur=%d scroll=%d)\n%s", e.name, p.cur, p.scroll, out)
			}
			// a narrow filter result still fits
			app = press(t, app, "t", "h", "i", "n", "g", "0")
			checkBudget(t, app, sz.w)
		})
	}
}

func TestRenderCommandFooter(t *testing.T) {
	app := typeCmd(t, browserApp(80, 24, 2, 12), "thing0")
	out := app.renderFooter()
	if n := len(strings.Split(out, "\n")); n != 2 || maxLineWidth(out) > 80 {
		t.Errorf("footer lines=%d width=%d", len(strings.Split(out, "\n")), maxLineWidth(out))
	}
	if !strings.Contains(out, "thing0") {
		t.Errorf("footer = %q", out)
	}
}

func keyCtrl(_ rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlF} }

func TestPhase2KeysAreInHintsAndHelp(t *testing.T) {
	app := browserApp(120, 40, 2, 5)
	have := map[string]bool{}
	for _, h := range stateHints(app) {
		have[h.key] = true
	}
	for _, want := range []string{"d", "^a", ":"} {
		if !have[want] {
			t.Errorf("types-pane hint %q missing", want)
		}
	}
	// the node list advertises them too
	nl := newTestApp(120, 40)
	nl.nodes = makeNodes(1)
	have = map[string]bool{}
	for _, h := range stateHints(nl) {
		have[h.key] = true
	}
	if !have["^a"] || !have[":"] {
		t.Errorf("node-list hints missing ^a or :")
	}
	help := buildHelpContent()
	for _, want := range []string{"ctrl+a", "Command mode", "Describe", "describe pane", "All types"} {
		if !strings.Contains(help, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
	// describe-pane and palette tables are listed through the same key table
	for _, kind := range []paneKind{paneTypes, paneInstances, paneYAML} {
		var found bool
		for _, a := range browserActionsFor(kind) {
			for _, k := range a.keys {
				found = found || k == "ctrl+a"
			}
		}
		if !found {
			t.Errorf("ctrl+a missing from the key table of pane kind %d", kind)
		}
	}
}
