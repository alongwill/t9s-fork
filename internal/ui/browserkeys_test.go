package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(t *testing.T, app App, keys ...string) App {
	t.Helper()
	for _, k := range keys {
		app, _ = app.handleKey(key(k))
	}
	return app
}

const testNet = "networking"

// makeBrowserDefs returns n net-suffix types named Thing00…; even ones have a
// non-zero count, odd ones are empty.
func makeBrowserDefs(n int) ([]talos.ResourceDef, map[string]int) {
	defs := make([]talos.ResourceDef, n)
	counts := make(map[string]int, n)
	for i := range defs {
		typ := fmt.Sprintf("Thing%02d.net.talos.dev", i)
		defs[i] = talos.ResourceDef{
			Type:             typ,
			DisplayType:      fmt.Sprintf("Thing%02d", i),
			Aliases:          []string{fmt.Sprintf("thing%02d", i)},
			DefaultNamespace: "network",
		}
		if i%2 == 0 {
			counts[typ] = i + 1
		} else {
			counts[typ] = 0
		}
	}
	return defs, counts
}

func sampleMetas(n int) []talos.ResourceMeta {
	out := make([]talos.ResourceMeta, n)
	for i := range out {
		out[i] = talos.ResourceMeta{Namespace: "network", Type: "Thing00.net.talos.dev", ID: fmt.Sprintf("eth%d/10.0.0.%d/24", i, i), Version: "3", Phase: "running"}
	}
	return out
}

const sampleYAML = "node: 10.0.0.1\nmetadata:\n    namespace: network\n    id: eth0\nspec:\n    address: 10.0.0.1/24\n    note: a very long line that will need to be wrapped when the wrap toggle is on in a narrow pane\n"

// browserApp builds an App already inside the browser. depth: 1 categories,
// 2 +types, 3 +instances, 4 +yaml.
func browserApp(width, height, depth, nTypes int) App {
	app := newTestApp(width, height)
	app.nodes = makeNodes(3)
	app.nodeCur = 1
	defs, counts := makeBrowserDefs(nTypes)
	n := app.nodes[1]
	app.selNode = &n
	b := browser{node: n, defs: defs, counts: counts, cfgState: cfgDenied}
	b.stack = []pane{{kind: paneCategories, title: "Categories"}}
	if depth >= 2 {
		b.stack = append(b.stack, pane{kind: paneTypes, title: "Networking", category: testNet})
	}
	if depth >= 3 {
		b.stack = append(b.stack, pane{kind: paneInstances, title: "Thing00", def: defs[0], items: sampleMetas(60)})
	}
	if depth >= 4 {
		b.stack = append(b.stack, pane{kind: paneYAML, title: "eth0", def: defs[0], meta: sampleMetas(1)[0], yaml: sampleYAML})
	}
	app.browser = b
	return app.syncBrowserState()
}

func TestBrowserEscClearsFilterBeforePopping(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	app.browser = app.browser.withTop(func(p *pane) { p.filter = "thing0"; p.cur = 3 })
	app = press(t, app, "esc")
	if len(app.browser.stack) != 2 {
		t.Fatalf("first esc popped the pane (stack=%d), want it to clear the filter", len(app.browser.stack))
	}
	if p, _ := app.browser.top(); p.filter != "" || p.cur != 0 {
		t.Errorf("filter not cleared: %+v", p)
	}
	app = press(t, app, "esc")
	if len(app.browser.stack) != 1 || app.state != StateCategories {
		t.Errorf("second esc: stack=%d state=%v, want 1 / StateCategories", len(app.browser.stack), app.state)
	}
}

func TestBrowserEscOnLastPaneReturnsToNodeList(t *testing.T) {
	app := browserApp(120, 40, 1, 10)
	app = press(t, app, "esc")
	if app.state != StateNodeList {
		t.Fatalf("state = %v, want StateNodeList", app.state)
	}
	if app.nodeCur != 1 {
		t.Errorf("nodeCur = %d, want 1 (node must stay selected)", app.nodeCur)
	}
	if len(app.browser.stack) != 0 || app.selNode != nil {
		t.Errorf("browser not reset: stack=%d selNode=%v", len(app.browser.stack), app.selNode)
	}
}

func TestBrowserQEqualsEsc(t *testing.T) {
	for depth := 1; depth <= 4; depth++ {
		a := press(t, browserApp(120, 40, depth, 10), "esc")
		b := press(t, browserApp(120, 40, depth, 10), "q")
		if a.state != b.state || len(a.browser.stack) != len(b.browser.stack) || a.nodeCur != b.nodeCur {
			t.Errorf("depth %d: esc → (%v,%d) but q → (%v,%d)", depth,
				a.state, len(a.browser.stack), b.state, len(b.browser.stack))
		}
	}
	// q also clears a filter first
	app := browserApp(120, 40, 2, 10)
	app.browser = app.browser.withTop(func(p *pane) { p.filter = "x" })
	if app = press(t, app, "q"); len(app.browser.stack) != 2 {
		t.Error("q popped instead of clearing the filter")
	}
}

func TestBrowserEnterOnEmptyTypeDoesNotPush(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	app = press(t, app, "down") // Thing01: count 0
	app = press(t, app, "enter")
	if len(app.browser.stack) != 2 {
		t.Fatalf("stack = %d, want 2 (no push)", len(app.browser.stack))
	}
	if !strings.Contains(app.statusMsg, "no Thing01 on this node") {
		t.Errorf("status = %q", app.statusMsg)
	}
}

func TestBrowserEnterOnLockedTypeDoesNotPush(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	app.browser = app.browser.setCount("Thing00.net.talos.dev", countLocked)
	app = press(t, app, "enter")
	if len(app.browser.stack) != 2 || !strings.Contains(app.statusMsg, "os:admin") {
		t.Errorf("stack=%d status=%q", len(app.browser.stack), app.statusMsg)
	}
}

func TestBrowserEnterSingleInstanceOpensListAndYAML(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	only := sampleMetas(1)[0]
	app.browser = app.browser.setCount("Thing00.net.talos.dev", 1).setSingle("Thing00.net.talos.dev", only, true)
	app = press(t, app, "enter")
	if got := len(app.browser.stack); got != 4 {
		t.Fatalf("stack = %d, want 4 (categories, types, instances, yaml)", got)
	}
	app = press(t, app, "esc") // YAML → the one-row instance list
	if p, _ := app.browser.top(); p.kind != paneInstances || len(p.items) != 1 {
		t.Errorf("esc from YAML landed on %+v", p)
	}
}

func TestBrowserEnterMultiInstanceOpensListOnly(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	app = press(t, app, "enter") // Thing00: count 1 but no single recorded → list
	if p, _ := app.browser.top(); p.kind != paneInstances || !p.loading {
		t.Errorf("top = %+v, want loading instances pane", p)
	}
	app, _ = app.Update2(resourceInstancesMsg{node: app.browser.node.IP, typ: "Thing00.net.talos.dev", items: sampleMetas(3)})
	if p, _ := app.browser.top(); p.loading || len(p.items) != 3 {
		t.Errorf("instances not filled: %+v", p)
	}
	app = press(t, app, "y")
	if p, _ := app.browser.top(); p.kind != paneYAML || p.meta.ID != "eth0/10.0.0.0/24" {
		t.Errorf("y did not open YAML for the selection: %+v", p)
	}
}

// Update2 is a test helper that keeps the concrete App type.
func (app App) Update2(msg tea.Msg) (App, tea.Cmd) {
	m, cmd := app.Update(msg)
	return m.(App), cmd
}

func TestBrowserStaleCountForOtherNodeIgnored(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	typ := "Thing01.net.talos.dev"
	app, _ = app.Update2(resourceCountMsg{node: "10.9.9.9", typ: typ, n: 42})
	if app.browser.counts[typ] != 0 {
		t.Errorf("stale count applied: %d", app.browser.counts[typ])
	}
	app, _ = app.Update2(resourceCountMsg{node: app.browser.node.IP, typ: typ, n: 42})
	if app.browser.counts[typ] != 42 {
		t.Errorf("fresh count not applied: %d", app.browser.counts[typ])
	}
	// after leaving the browser every reply is stale
	app = press(t, app, "esc", "esc")
	app, _ = app.Update2(resourceCountMsg{node: "10.0.0.2", typ: typ, n: 7})
	if len(app.browser.counts) != 0 {
		t.Error("count applied after the browser was closed")
	}
}

func TestNodeListAOpensBrowserAndShiftAOpensAddresses(t *testing.T) {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(3)
	app.nodeCur = 2

	b := press(t, app, "a")
	if b.state != StateCategories {
		t.Fatalf("a → state %v, want StateCategories", b.state)
	}
	if b.browser.node.IP != "10.0.0.3" || !b.browser.defsLoading {
		t.Errorf("browser node=%s defsLoading=%v", b.browser.node.IP, b.browser.defsLoading)
	}

	c := press(t, app, "A")
	if c.state != StateAddresses {
		t.Errorf("A → state %v, want StateAddresses", c.state)
	}
}

func TestBrowserCachedDefsSkipLoad(t *testing.T) {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(1)
	defs, _ := makeBrowserDefs(3)
	app.resourceDefs = map[string][]talos.ResourceDef{"10.0.0.1": defs}
	app = press(t, app, "a")
	if app.browser.defsLoading || len(app.browser.defs) != 3 {
		t.Errorf("cache not used: loading=%v defs=%d", app.browser.defsLoading, len(app.browser.defs))
	}
}

func TestBrowserDefsLoadedFillsCacheWithoutMutatingOld(t *testing.T) {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(1)
	app = press(t, app, "a")
	old := app.resourceDefs
	defs, _ := makeBrowserDefs(3)
	app, _ = app.Update2(resourceDefsMsg{node: "10.0.0.1", defs: defs})
	if len(app.resourceDefs["10.0.0.1"]) != 3 || app.browser.defsLoading {
		t.Errorf("defs not stored: %+v", app.browser)
	}
	if len(old) != 0 {
		t.Error("previous cache map was mutated")
	}
}

func TestBrowserFilterPromptSwallowsGlobalKeys(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	app = press(t, app, "/")
	if !app.browser.prompting {
		t.Fatal("/ did not open the prompt")
	}
	// x (context switcher) and ? (help) must be typed, not trigger globals
	app = press(t, app, "x", "?")
	if app.state != StateBrowser {
		t.Fatalf("global key leaked through the prompt: state %v", app.state)
	}
	app = press(t, app, "enter")
	if p, _ := app.browser.top(); p.filter != "x?" {
		t.Errorf("filter = %q, want %q", p.filter, "x?")
	}
	// esc cancels without touching the applied filter
	app = press(t, app, "/", "esc")
	if p, _ := app.browser.top(); p.filter != "x?" || app.browser.prompting {
		t.Errorf("esc should cancel the prompt only: %+v prompting=%v", p, app.browser.prompting)
	}
}

func TestBrowserFilterNarrowsRowsAndResetsCursor(t *testing.T) {
	app := browserApp(120, 40, 2, 20)
	app = press(t, app, "down", "down", "down")
	app = press(t, app, "/", "1", "5", "enter")
	rows := app.browser.typeRows(testNet, "15")
	if len(rows) != 1 || rows[0].DisplayType != "Thing15" {
		t.Fatalf("rows = %+v", rows)
	}
	if p, _ := app.browser.top(); p.cur != 0 {
		t.Errorf("cursor not reset: %d", p.cur)
	}
}

func TestMatcher(t *testing.T) {
	cases := []struct {
		term   string
		field  string
		invert bool
		want   bool
	}{
		{"link", "LinkStatus", true, true},
		{"^addr", "AddressStatus", true, true},
		{"^addr", "LinkAddress", true, false},
		{"!link", "LinkStatus", true, false},
		{"!link", "Disk", true, true},
		{"[", "a[b", true, true}, // invalid regex → substring
		{"[", "ab", true, false}, // …and only substring
		{"!", "anything", true, true},
		{"!x", "!x", false, true}, // find mode: ! is literal
	}
	for _, c := range cases {
		if got := newMatcher(c.term, c.invert).match(c.field); got != c.want {
			t.Errorf("match(%q, %q) = %v, want %v", c.term, c.field, got, c.want)
		}
	}
}

func TestBrowserYAMLFindAndWrap(t *testing.T) {
	app := browserApp(100, 30, 4, 10)
	app = press(t, app, "/")
	app = press(t, app, "n", "a", "m", "e", "s", "p", "enter")
	if hits := app.browser.findHits; len(hits) != 1 || hits[0] != 2 {
		t.Fatalf("hits = %v, want [2]", hits)
	}
	app = press(t, app, "n") // wraps around to the same single hit
	if app.browser.findIdx != 0 {
		t.Errorf("findIdx = %d", app.browser.findIdx)
	}
	// esc clears the search before popping
	app = press(t, app, "esc")
	if app.browser.find != "" || len(app.browser.stack) != 4 {
		t.Errorf("esc: find=%q stack=%d", app.browser.find, len(app.browser.stack))
	}
	before := app.paneLen(app.browser.stack[3])
	app = press(t, app, "w")
	if !app.browser.wrap || app.paneLen(app.browser.stack[3]) <= before {
		t.Errorf("wrap should add visual lines: before=%d after=%d", before, app.paneLen(app.browser.stack[3]))
	}
	app = press(t, app, "f")
	if !app.browser.fullscreen {
		t.Error("f did not toggle full screen")
	}
	app = press(t, app, "esc") // pop YAML → fullscreen off
	if app.browser.fullscreen {
		t.Error("fullscreen survived leaving the YAML pane")
	}
}

func TestBrowserReloadDefsClearsCache(t *testing.T) {
	app := browserApp(120, 40, 1, 5)
	app.resourceDefs = map[string][]talos.ResourceDef{app.browser.node.IP: app.browser.defs, "other": nil}
	app, cmd := app.handleBrowserKey(tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil || !app.browser.defsLoading || len(app.browser.defs) != 0 {
		t.Errorf("reload did not restart the defs load: %+v", app.browser)
	}
	if _, ok := app.resourceDefs[app.browser.node.IP]; ok {
		t.Error("node cache entry survived ctrl+r")
	}
	if _, ok := app.resourceDefs["other"]; !ok {
		t.Error("ctrl+r dropped another node's cache")
	}
}

func TestBrowserHintsAndHelpComeFromKeyTable(t *testing.T) {
	app := browserApp(120, 40, 2, 5)
	var labels []string
	for _, h := range stateHints(app) {
		labels = append(labels, h.key)
	}
	for _, want := range []string{"↵", "Esc/q", "^r", "?"} {
		found := false
		for _, l := range labels {
			found = found || l == want
		}
		if !found {
			t.Errorf("hint %q missing from %v", want, labels)
		}
	}
	help := buildHelpContent()
	for _, want := range []string{"Resource Browser", "ctrl+f / pgdown", "Toggle full screen", "Resource browser"} {
		if !strings.Contains(help, want) {
			t.Errorf("help overlay missing %q", want)
		}
	}
}

func TestFuzzyScoreAndRanking(t *testing.T) {
	if _, ok := fuzzyScore("adst", "AddressStatus"); !ok {
		t.Error("adst should match AddressStatus")
	}
	if _, ok := fuzzyScore("xyz", "AddressStatus"); ok {
		t.Error("xyz should not match")
	}
	if _, ok := fuzzyScore("ts", "stat"); ok {
		t.Error("order matters: ts must not match stat")
	}
	items := []string{"LinkAddressThing", "AddressStatus", "AddressSpec", "Disk"}
	got := rankFilter(items, "addr", func(s string) []string { return []string{s} })
	if len(got) != 3 || got[0] == "LinkAddressThing" || got[2] != "LinkAddressThing" {
		t.Errorf("prefix matches should outrank mid-word: %v", got)
	}
	got = rankFilter(items, "!addr", func(s string) []string { return []string{s} })
	if len(got) != 1 || got[0] != "Disk" {
		t.Errorf("inverse: %v", got)
	}
}

func TestBrowserFuzzyFilterIsLiveAndArrowsMove(t *testing.T) {
	app := browserApp(120, 40, 2, 20)
	app = press(t, app, "/", "t", "h", "1")
	// live: rows are already narrowed before enter, prompt still open
	if !app.browser.prompting {
		t.Fatal("prompt closed early")
	}
	p, _ := app.browser.top()
	rows := app.browser.typeRows(testNet, p.filter)
	if p.filter != "th1" || len(rows) != 11 {
		t.Fatalf("live filter %q → %d rows, want 11 (Thing10-19 plus Thing01)", p.filter, len(rows))
	}
	app = press(t, app, "down", "down")
	if p, _ = app.browser.top(); p.cur != 2 || !app.browser.prompting {
		t.Errorf("arrows while typing: cur=%d prompting=%v", p.cur, app.browser.prompting)
	}
	app = press(t, app, "enter")
	if p, _ = app.browser.top(); app.browser.prompting || p.filter != "th1" || p.cur != 2 {
		t.Errorf("enter should keep filter and cursor: %+v", p)
	}
	// esc inside a new prompt restores the applied filter
	app = press(t, app, "/", "x", "esc")
	if p, _ = app.browser.top(); p.filter != "th1" {
		t.Errorf("esc should restore previous filter, got %q", p.filter)
	}
}
