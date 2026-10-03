package ui

import (
	"fmt"
	"strings"
	"testing"
)

func compareRenderApp(t *testing.T, w, h, nodes int) App {
	t.Helper()
	app, fs := compareApp(t, 3)
	app.width, app.height = w, h
	app.nodes = makeNodes(nodes)
	for i := range app.nodes {
		app.nodes[i].Role = "worker"
	}
	app.browser.node = app.nodes[1]
	id := sampleMetas(1)[0].ID
	for i, n := range app.nodes {
		fs.yamls[n.IP+"|"+cmpType+"|"+id] = resYAML(n.IP, "3", fmt.Sprintf("10.0.%d.1/24", i%3))
		delete(fs.yerrs, n.IP+"|"+cmpType+"|"+id)
	}
	return loadedCompare(t, app)
}

func TestRenderComparePane(t *testing.T) {
	for _, sz := range browserSizes {
		sz := sz
		t.Run(fmt.Sprintf("w%d_h%d", sz.w, sz.h), func(t *testing.T) {
			app := compareRenderApp(t, sz.w, sz.h, 40) // more rows than fit
			out := checkBudget(t, app, sz.w)
			for _, want := range []string{"NODE", "PRESENT", "SAME?", "base", "* "} {
				if want == "* " {
					want = " *"
				}
				if !strings.Contains(out, want) {
					t.Errorf("missing %q:\n%s", want, out)
				}
			}
			if sz.w >= 120 && !strings.Contains(out, "ROLE") {
				t.Errorf("ROLE column missing at %d cols", sz.w)
			}
			// walk to the bottom: the cursor row stays visible
			for i := 0; i < 45; i++ {
				app = press(t, app, "down")
				top, _ := app.browser.top()
				out = checkBudget(t, app, sz.w)
				host := cutWidth(app.compareHost(top.cur), 14)
				if !strings.Contains(out, "▶ "+host) {
					t.Fatalf("step %d: cursor row %q not visible\n%s", i, host, out)
				}
			}
		})
	}
}

func (app App) compareHost(i int) string {
	top, _ := app.browser.top()
	r := top.cmp.rows[i]
	if r.node.Hostname != "" {
		return r.node.Hostname
	}
	return r.node.IP
}

func TestRenderDiffPane(t *testing.T) {
	for _, sz := range browserSizes {
		sz := sz
		t.Run(fmt.Sprintf("w%d_h%d", sz.w, sz.h), func(t *testing.T) {
			app := compareRenderApp(t, sz.w, sz.h, 3)
			app.browser = app.browser.withTop(func(p *pane) { p.cur = 0 }) // node 0 differs from base (node 1)
			app = press(t, app, "enter")
			if top, _ := app.browser.top(); top.kind != paneDiff {
				t.Fatalf("no diff pane (status %q)", app.statusMsg)
			}
			out := checkBudget(t, app, sz.w)
			for _, want := range []string{"--- ", "+++ ", "@@", "-    address", "+    address"} {
				if !strings.Contains(out, want) {
					t.Errorf("diff pane missing %q:\n%s", want, out)
				}
			}
			// long diff scrolls and stays within budget
			long := make([]diffLine, 200)
			for i := range long {
				long[i] = diffLine{'+', strings.Repeat("wide ", 80)}
			}
			app.browser = app.browser.withTop(func(p *pane) { p.diff = long })
			for _, k := range []string{"G", "g", "ctrl+f", "j"} {
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

func TestRenderCompareHintsAndHelpListNewKeys(t *testing.T) {
	app, _ := compareApp(t, 3)
	var hints []string
	for _, h := range stateHints(app) {
		hints = append(hints, h.key+" "+h.desc)
	}
	joined := strings.Join(hints, "|")
	for _, want := range []string{"W Live watch", "c Compare"} {
		if !strings.Contains(joined, want) {
			t.Errorf("instances hints lack %q: %s", want, joined)
		}
	}
	help := buildHelpContent()
	for _, want := range []string{"Compare on all nodes", "Live watch on/off", "Diff against the browser's node"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}
