package ui

import (
	"strings"
	"testing"
)

var allPaneKinds = []paneKind{paneCategories, paneTypes, paneInstances, paneYAML, paneDescribe, paneAliases, paneCompare, paneDiff, paneRelated, paneNetwork}

func boundIn(key string, kind paneKind) bool {
	for _, a := range browserActionsFor(kind) {
		for _, k := range a.keys {
			if k == key {
				return true
			}
		}
	}
	return false
}

// appWithTop returns a browser whose top pane is of the given kind.
func appWithTop(kind paneKind) App {
	app := browserApp(120, 40, 2, 10)
	if kind == paneCategories || kind == paneTypes {
		if kind == paneCategories {
			app.browser = app.browser.pop()
		}
		return app.syncBrowserState()
	}
	app = browserApp(120, 40, 4, 10)
	switch kind {
	case paneInstances:
		app.browser = app.browser.pop()
	case paneDescribe, paneAliases, paneCompare, paneDiff, paneRelated, paneNetwork:
		app.browser = app.browser.push(pane{kind: kind, title: "x"})
	}
	return app.syncBrowserState()
}

func TestBrowserKeyTableMatchesBindings(t *testing.T) {
	for _, k := range browserKeyTable {
		want := map[paneKind]bool{}
		for _, p := range k.panes {
			want[p] = true
		}
		for _, kind := range allPaneKinds {
			if got := boundIn(k.key, kind); got != want[kind] {
				t.Errorf("key %q pane %d: bound=%v, table says %v", k.key, kind, got, want[kind])
			}
		}
	}
}

func TestBrowserKeysActOrExplain(t *testing.T) {
	for _, k := range browserKeyTable {
		for _, kind := range allPaneKinds {
			if boundIn(k.key, kind) {
				continue
			}
			app := appWithTop(kind)
			if top, _ := app.browser.top(); top.kind != kind {
				t.Fatalf("setup: top = %d, want %d", top.kind, kind)
			}
			app = press(t, app, k.key)
			if app.statusMsg == "" {
				t.Errorf("key %q in pane %d did nothing and said nothing", k.key, kind)
			}
		}
	}
}

func TestUnboundKeyNamesTheSelectedType(t *testing.T) {
	app := appWithTop(paneTypes)
	app = press(t, app, "y")
	if !strings.Contains(app.statusMsg, "y works on an instance list") || !strings.Contains(app.statusMsg, "press Enter on Thing") {
		t.Errorf("status = %q", app.statusMsg)
	}
	// on the categories pane there is no type to name
	app = press(t, appWithTop(paneCategories), "W")
	if !strings.Contains(app.statusMsg, "W works on") || strings.Contains(app.statusMsg, "press Enter on") {
		t.Errorf("status = %q", app.statusMsg)
	}
}

// typeIndex finds a resource row by display type in the types pane.
func typeIndex(t *testing.T, app App, display string) int {
	t.Helper()
	p, _ := app.browser.top()
	for i, e := range app.browser.typeEntries(p.category, p.filter) {
		if !e.config && e.def.DisplayType == display {
			return i
		}
	}
	t.Fatalf("no row %s", display)
	return 0
}

func TestCompareAndWatchOnTypeOpenInstances(t *testing.T) {
	for _, letter := range []string{"c", "W"} {
		app := appWithTop(paneTypes)
		app.source = newFakeSource("grpc")
		i := typeIndex(t, app, "Thing02") // 3 instances
		app.browser = app.browser.withTop(func(p *pane) { p.cur = i })
		app = press(t, app, letter)
		top, _ := app.browser.top()
		if top.kind != paneInstances || top.def.DisplayType != "Thing02" {
			t.Fatalf("%s: top = %+v, want Thing02 instance list", letter, top)
		}
		if !strings.Contains(app.statusMsg, "pick one, then press "+letter) {
			t.Errorf("%s: status = %q", letter, app.statusMsg)
		}
	}
}

func TestWatchOnTypesPaneNeedsGRPC(t *testing.T) {
	app := appWithTop(paneTypes)
	app.source = newFakeSource("cli")
	app = press(t, app, "W")
	if !strings.Contains(app.statusMsg, "gRPC") {
		t.Errorf("status = %q", app.statusMsg)
	}
}
