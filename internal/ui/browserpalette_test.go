package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

func ctrlA(t *testing.T, app App) (App, tea.Cmd) {
	t.Helper()
	return app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlA})
}

func stackSummary(app App) string {
	var sb strings.Builder
	for _, p := range app.browser.stack {
		sb.WriteString(strings.Join([]string{p.title, p.filter}, "|"))
		sb.WriteString(";")
		sb.WriteString(strings.Repeat("#", p.cur))
		sb.WriteString("/")
	}
	return sb.String()
}

func TestPaletteOpenCloseRestoresStack(t *testing.T) {
	for depth := 1; depth <= 4; depth++ {
		app := browserApp(120, 40, depth, 8)
		before := stackSummary(app)
		stateBefore := app.state
		app, _ = ctrlA(t, app)
		if p, _ := app.browser.top(); p.kind != paneAliases || !app.browser.prompting {
			t.Fatalf("depth %d: ctrl+a top=%v prompting=%v", depth, p.kind, app.browser.prompting)
		}
		if len(app.browser.stack) != depth+1 {
			t.Fatalf("depth %d: stack %d", depth, len(app.browser.stack))
		}
		app = press(t, app, "esc")
		if got := stackSummary(app); got != before || app.state != stateBefore || app.browser.prompting {
			t.Errorf("depth %d: stack not restored: %q vs %q (state %v/%v prompting %v)",
				depth, got, before, app.state, stateBefore, app.browser.prompting)
		}
	}
}

func TestPaletteEscClearsFilterBeforeClosing(t *testing.T) {
	app := browserApp(120, 40, 2, 8)
	app, _ = ctrlA(t, app)
	app = press(t, app, "t", "h", "i")
	if p, _ := app.browser.top(); p.filter != "thi" {
		t.Fatalf("filter = %q", p.filter)
	}
	app = press(t, app, "esc")
	p, _ := app.browser.top()
	if p.kind != paneAliases || p.filter != "" || !app.browser.prompting {
		t.Errorf("first esc should clear the filter only: %+v prompting=%v", p, app.browser.prompting)
	}
	app = press(t, app, "esc")
	if p, _ = app.browser.top(); p.kind != paneTypes {
		t.Errorf("second esc should close the palette, top=%v", p.kind)
	}
}

func TestPaletteListsEverythingAndFiltersLive(t *testing.T) {
	app := cfgApp(120, 40, 4)
	app, _ = ctrlA(t, app)
	all := app.browser.paletteRows("")
	var cfg, res int
	for _, e := range all {
		if e.config {
			cfg++
		} else {
			res++
		}
	}
	if res != 4 || cfg == 0 {
		t.Fatalf("palette has %d resources, %d config kinds", res, cfg)
	}
	app = press(t, app, "d", "h", "c", "p", "v", "4")
	p, _ := app.browser.top()
	rows := app.browser.paletteRows(p.filter)
	if len(rows) == 0 || rows[0].name != "DHCPv4Config" {
		t.Errorf("dhcpv4 → %+v", rows)
	}
	out := app.renderBrowser(app.mainHeight())
	for _, want := range []string{"NAME", "ALIASES", "CATEGORY", "KIND", "COUNT", "config"} {
		if !strings.Contains(out, want) {
			t.Errorf("palette render missing %q", want)
		}
	}
}

func TestPaletteExactAliasRanksFirst(t *testing.T) {
	app := browserApp(120, 40, 1, 0)
	app.browser.defs = []talos.ResourceDef{
		{Type: "AddrBooks.net.talos.dev", DisplayType: "AddrBook", Aliases: []string{"ab"}, DefaultNamespace: "network"},
		{Type: "AddressStatuses.net.talos.dev", DisplayType: "AddressStatus", Aliases: []string{"addr"}, DefaultNamespace: "network"},
	}
	rows := app.browser.paletteRows("addr")
	if len(rows) != 2 || rows[0].name != "AddressStatus" {
		t.Fatalf("exact alias must come first: %+v", rows)
	}
	if rows := app.browser.paletteRows("ADDR"); rows[0].name != "AddressStatus" {
		t.Errorf("exact alias is case-insensitive: %+v", rows)
	}
	if rows := app.browser.paletteRows("addressstatuses.net.talos.dev"); len(rows) == 0 || rows[0].name != "AddressStatus" {
		t.Errorf("full type should match exactly: %+v", rows)
	}
}

func TestPaletteJumpBuildsStackAndCursors(t *testing.T) {
	app := browserApp(120, 40, 1, 8) // Thing00…Thing07, cfg denied
	app, _ = ctrlA(t, app)
	app = press(t, app, "t", "h", "i", "n", "g", "0", "4")
	app = press(t, app, "enter")

	st := app.browser.stack
	if len(st) != 3 || st[0].kind != paneCategories || st[1].kind != paneTypes || st[2].kind != paneInstances {
		t.Fatalf("stack = %d panes: %s", len(st), stackSummary(app))
	}
	if rows := app.browser.categoryRows(""); rows[st[0].cur].key != testNet {
		t.Errorf("categories cursor on %q, want %q", rows[st[0].cur].key, testNet)
	}
	if e := app.browser.typeEntries(testNet, "")[st[1].cur]; e.name() != "Thing04" {
		t.Errorf("types cursor on %q, want Thing04", e.name())
	}
	if st[1].filter != "" || app.browser.prompting {
		t.Errorf("filter/prompt leaked: %q %v", st[1].filter, app.browser.prompting)
	}
	// esc lands in the category, not back in the palette
	app = press(t, app, "esc")
	if p, _ := app.browser.top(); p.kind != paneTypes {
		t.Fatalf("esc landed on %v, want the types pane", p.kind)
	}
	if app.hasPalette() {
		t.Error("palette still on the stack after a jump")
	}
}

func TestPaletteJumpToConfigKindOpensYAML(t *testing.T) {
	app := cfgApp(120, 40, 3)
	app, _ = ctrlA(t, app)
	app = press(t, app, "d", "h", "c", "p", "v", "4", "enter")
	top, _ := app.browser.top()
	if top.kind != paneYAML || top.yaml != "kind: DHCPv4Config\n" {
		t.Fatalf("top = %+v", top)
	}
	if len(app.browser.stack) != 4 {
		t.Errorf("stack = %d, want 4", len(app.browser.stack))
	}
	app = press(t, app, "esc", "esc")
	p, _ := app.browser.top()
	if p.kind != paneTypes || app.browser.typeEntries(testNet, "")[p.cur].name() != "DHCPv4Config" {
		t.Errorf("esc,esc landed on %+v", p)
	}
}

func TestPaletteStartsCountsForUncountedTypes(t *testing.T) {
	app := browserApp(120, 40, 1, 6)
	app.browser.counts = map[string]int{"Thing00.net.talos.dev": 1}
	app, cmd := ctrlA(t, app)
	if cmd == nil {
		t.Fatal("no commands returned")
	}
	if got := len(app.browser.loading); got != 5 {
		t.Errorf("loading = %d types, want the 5 uncounted", got)
	}
	// replies fill rows in
	app, _ = app.Update2(resourceCountMsg{node: app.browser.node.IP, typ: "Thing03.net.talos.dev", n: 9})
	for _, e := range app.browser.paletteRows("") {
		if e.name == "Thing03" {
			if txt, _ := app.browser.paletteCell(e); txt != "9" {
				t.Errorf("count cell = %q", txt)
			}
		}
	}
}

func TestPaletteWaitsForDefsThenCounts(t *testing.T) {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(1)
	app, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	if !app.hasPalette() || !app.browser.defsLoading {
		t.Fatalf("node-list ctrl+a: palette=%v defsLoading=%v", app.hasPalette(), app.browser.defsLoading)
	}
	defs, _ := makeBrowserDefs(3)
	app, cmd := app.Update2(resourceDefsMsg{node: "10.0.0.1", defs: defs})
	if cmd == nil || len(app.browser.loading) != 3 {
		t.Errorf("defs arrival should start counting: cmd=%v loading=%d", cmd != nil, len(app.browser.loading))
	}
}

func TestPaletteFromNodeListUsesSelectedNode(t *testing.T) {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(3)
	app.nodeCur = 2
	app, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyCtrlA})
	if app.browser.node.IP != "10.0.0.3" {
		t.Errorf("node = %s", app.browser.node.IP)
	}
	if p, _ := app.browser.top(); p.kind != paneAliases || len(app.browser.stack) != 2 {
		t.Errorf("stack = %s", stackSummary(app))
	}
	app = press(t, app, "esc", "esc") // palette → categories → node list
	if app.state != StateNodeList || app.nodeCur != 2 {
		t.Errorf("state=%v nodeCur=%d", app.state, app.nodeCur)
	}
}

func TestPaletteTypedKeysAreNotGlobals(t *testing.T) {
	app := browserApp(120, 40, 2, 5)
	app, _ = ctrlA(t, app)
	app = press(t, app, "x", "?", "q")
	if app.state != StateBrowser || !app.hasPalette() {
		t.Errorf("global key leaked: state=%v", app.state)
	}
	if p, _ := app.browser.top(); p.filter != "x?q" {
		t.Errorf("filter = %q", p.filter)
	}
}
