package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

func typeCmd(t *testing.T, app App, text string) App {
	t.Helper()
	app = press(t, app, ":")
	for _, r := range text {
		app, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return app
}

func submit(t *testing.T, app App) (App, tea.Cmd) {
	t.Helper()
	return app.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
}

func run(t *testing.T, app App, text string) (App, tea.Cmd) {
	t.Helper()
	return submit(t, typeCmd(t, app, text))
}

func nodeListApp(n, cur int) App {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(n)
	app.nodeCur = cur
	return app
}

func TestCommandOpensFromNodeListAndBrowser(t *testing.T) {
	for name, app := range map[string]App{"nodes": nodeListApp(2, 0), "browser": browserApp(120, 40, 2, 5), "yaml": browserApp(120, 40, 4, 5)} {
		a := press(t, app, ":")
		if !a.cmd.active {
			t.Errorf("%s: : did not open the prompt", name)
		}
		// typed letters are not global keys
		a = press(t, a, "x", "?")
		if a.state != app.state || a.cmd.input.Value() != "x?" {
			t.Errorf("%s: global key leaked, state=%v value=%q", name, a.state, a.cmd.input.Value())
		}
	}
	// not offered in other views
	h := nodeListApp(1, 0)
	h.state = StateServices
	if press(t, h, ":").cmd.active {
		t.Error(": opened in the services view")
	}
}

func TestCommandEscCancelsAndClearKeys(t *testing.T) {
	app := typeCmd(t, nodeListApp(1, 0), "net")
	for _, k := range []tea.KeyMsg{{Type: tea.KeyCtrlU}, {Type: tea.KeyCtrlW}} {
		a, _ := app.handleKey(k)
		if a.cmd.input.Value() != "" || !a.cmd.active {
			t.Errorf("%v should clear the buffer and keep the prompt", k)
		}
	}
	app, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if app.cmd.input.Value() != "ne" {
		t.Errorf("backspace → %q", app.cmd.input.Value())
	}
	app = press(t, app, "esc")
	if app.cmd.active {
		t.Error("esc did not cancel")
	}
}

func TestCommandNodes(t *testing.T) {
	for _, w := range []string{"nodes", "no", "NODES"} {
		app, _ := run(t, browserApp(120, 40, 3, 5), w)
		if app.state != StateNodeList || app.nodeCur != 1 {
			t.Errorf(":%s → state %v nodeCur %d", w, app.state, app.nodeCur)
		}
	}
	app, _ := run(t, nodeListApp(2, 1), "nodes")
	if app.state != StateNodeList {
		t.Error(":nodes on the node list changed state")
	}
}

func TestCommandQuitAndHelp(t *testing.T) {
	for _, w := range []string{"q", "q!", "quit"} {
		_, cmd := run(t, nodeListApp(1, 0), w)
		if cmd == nil {
			t.Fatalf(":%s returned no command", w)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf(":%s did not quit", w)
		}
	}
	for _, w := range []string{"?", "h", "help"} {
		app, _ := run(t, browserApp(120, 40, 2, 5), w)
		if app.state != StateHelp {
			t.Errorf(":%s → state %v", w, app.state)
		}
	}
}

func TestCommandCategoryByKeyOrLabelPrefix(t *testing.T) {
	for _, w := range []string{"net", "Networking", "networking"} {
		app, _ := run(t, browserApp(120, 40, 4, 5), w)
		st := app.browser.stack
		if len(st) != 2 || st[1].kind != paneTypes || st[1].category != testNet {
			t.Errorf(":%s → %s", w, stackSummary(app))
		}
		if rows := app.browser.categoryRows(""); rows[st[0].cur].key != testNet {
			t.Errorf(":%s categories cursor on %q", w, rows[st[0].cur].key)
		}
	}
	app, _ := run(t, browserApp(120, 40, 2, 5), "block")
	if p, _ := app.browser.top(); p.kind != paneTypes || p.category != "block" {
		t.Errorf(":block → %+v", p)
	}
}

func TestCommandTypeByAliasDisplayFullTypeAndConfigKind(t *testing.T) {
	for _, w := range []string{"thing04", "Thing04", "thing04.net.talos.dev"} {
		app, _ := run(t, browserApp(120, 40, 1, 8), w)
		st := app.browser.stack
		if len(st) != 3 || st[2].kind != paneInstances || st[2].def.DisplayType != "Thing04" {
			t.Errorf(":%s → %s", w, stackSummary(app))
		}
	}
	app, _ := run(t, cfgApp(120, 40, 3), "dhcpv4config")
	if top, _ := app.browser.top(); top.kind != paneYAML || top.cfgKind != "DHCPv4Config" {
		t.Errorf(":dhcpv4config → %+v", top)
	}
	// esc lands in the category, as with the palette
	app = press(t, app, "esc", "esc")
	if p, _ := app.browser.top(); p.kind != paneTypes {
		t.Errorf("esc,esc → %v", p.kind)
	}
}

func TestCommandAliasesOpensPalette(t *testing.T) {
	for _, w := range []string{"a", "alias", "aliases"} {
		app, _ := run(t, browserApp(120, 40, 2, 5), w)
		if p, _ := app.browser.top(); p.kind != paneAliases {
			t.Errorf(":%s → top %v", w, p.kind)
		}
	}
}

func TestCommandUnknownShowsErrorAndClosesPrompt(t *testing.T) {
	app, _ := run(t, browserApp(120, 40, 2, 5), "bogus")
	if app.cmd.active || !strings.Contains(app.statusMsg, "unknown command: bogus") {
		t.Errorf("active=%v status=%q", app.cmd.active, app.statusMsg)
	}
	if len(app.browser.stack) != 2 {
		t.Error("an unknown command must not change the stack")
	}
}

func TestCommandFromNodeListUsesSelectedNode(t *testing.T) {
	app := nodeListApp(3, 2)
	defs, _ := makeBrowserDefs(6)
	app.resourceDefs = map[string][]talos.ResourceDef{"10.0.0.3": defs}
	app, _ = run(t, app, "net")
	if app.browser.node.IP != "10.0.0.3" || len(app.browser.stack) != 2 {
		t.Errorf("node=%s stack=%s", app.browser.node.IP, stackSummary(app))
	}
	// nodes/quit/help don't need a node
	if a, _ := run(t, nodeListApp(0, 0), "help"); a.state != StateHelp {
		t.Error(":help needs no node")
	}
	if a, _ := run(t, nodeListApp(0, 0), "net"); !strings.Contains(a.statusMsg, "no node selected") {
		t.Errorf("status = %q", a.statusMsg)
	}
}

func TestCommandWaitsForDefsThenRuns(t *testing.T) {
	app := nodeListApp(1, 0)
	app, _ = run(t, app, "thing03")
	if app.browser.pendingCmd != "thing03" || app.cmd.active {
		t.Fatalf("pending = %q", app.browser.pendingCmd)
	}
	defs, counts := makeBrowserDefs(6)
	app, _ = app.Update2(resourceDefsMsg{node: "10.0.0.1", defs: defs})
	_ = counts
	st := app.browser.stack
	if app.browser.pendingCmd != "" || len(st) != 2 || st[1].category != testNet {
		t.Errorf("pending not run: %q %s", app.browser.pendingCmd, stackSummary(app))
	}
	// …and an unresolvable one reports unknown once the definitions are in
	app = nodeListApp(1, 0)
	app, _ = run(t, app, "nonsense")
	app, _ = app.Update2(resourceDefsMsg{node: "10.0.0.1", defs: defs})
	if !strings.Contains(app.statusMsg, "unknown command: nonsense") {
		t.Errorf("status = %q", app.statusMsg)
	}
}

func TestCommandSuggestionAcceptAndCycle(t *testing.T) {
	app := browserApp(120, 40, 2, 12)
	app = typeCmd(t, app, "thing0")
	sugs := app.cmdSuggestions("thing0")
	if len(sugs) < 3 || !strings.HasPrefix(sugs[0], "thing0") {
		t.Fatalf("suggestions = %v", sugs)
	}
	first := app.cmdSuffix()
	if first != sugs[0][len("thing0"):] {
		t.Errorf("suffix %q, want %q", first, sugs[0][len("thing0"):])
	}
	app = press(t, app, "down")
	if app.cmdSuffix() != sugs[1][len("thing0"):] {
		t.Errorf("down did not show the next suggestion: %q", app.cmdSuffix())
	}
	app, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	app, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyUp}) // wraps to the last
	if app.cmdSuffix() != sugs[len(sugs)-1][len("thing0"):] {
		t.Errorf("up wrap: %q", app.cmdSuffix())
	}
	app, _ = app.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if want := sugs[len(sugs)-1]; app.cmd.input.Value() != want {
		t.Errorf("tab → %q, want %q", app.cmd.input.Value(), want)
	}
	// right also accepts
	b := typeCmd(t, browserApp(120, 40, 2, 12), "ali")
	b, _ = b.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	if b.cmd.input.Value() != "alias" && b.cmd.input.Value() != "aliases" {
		t.Errorf("right → %q", b.cmd.input.Value())
	}
	// no suggestion for something nothing starts with
	if c := typeCmd(t, browserApp(120, 40, 2, 3), "zzz"); c.cmdSuffix() != "" {
		t.Errorf("suffix for zzz = %q", c.cmdSuffix())
	}
	out := app.renderFooter()
	if !strings.Contains(out, ":") || !strings.Contains(out, app.cmd.input.Value()) {
		t.Errorf("footer = %q", out)
	}
}

func TestCommandHistoryCycling(t *testing.T) {
	app := browserApp(120, 40, 2, 5)
	for _, w := range []string{"net", "bogus", "net", "help"} {
		app, _ = run(t, app, w)
		app.state = StateBrowser // leave help
	}
	if want := []string{"net", "bogus", "net", "help"}; strings.Join(app.cmdHistory, ",") != strings.Join(want, ",") {
		t.Fatalf("history = %v", app.cmdHistory)
	}
	app = press(t, app, ":")
	val := func() string { return app.cmd.input.Value() }
	up := func() { app = press(t, app, "up") }
	down := func() { app = press(t, app, "down") }
	up()
	if val() != "help" {
		t.Errorf("up 1 = %q", val())
	}
	up()
	up()
	if val() != "bogus" {
		t.Errorf("up 3 = %q", val())
	}
	up()
	up() // clamps at the oldest
	if val() != "net" {
		t.Errorf("up 5 = %q", val())
	}
	down()
	if val() != "bogus" {
		t.Errorf("down = %q", val())
	}
	down()
	down()
	down() // past the newest: empty
	if val() != "" {
		t.Errorf("down past newest = %q", val())
	}
	// consecutive duplicates are not recorded twice
	app = press(t, app, "esc")
	app, _ = run(t, app, "net")
	app, _ = run(t, app, "net")
	if n := len(app.cmdHistory); n != 5 {
		t.Errorf("history len = %d, want 5 (net,bogus,net,help,net)", n)
	}
}
