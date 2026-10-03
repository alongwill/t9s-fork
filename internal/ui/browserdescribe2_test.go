package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/florianspk/t9s/internal/talos"
)

const (
	tLink  = "LinkStatuses.net.talos.dev"
	tSpec  = "LinkSpecs.net.talos.dev"
	tAddr  = "AddressStatuses.net.talos.dev"
	tGhost = "Ghosts.net.talos.dev" // in the graph, not on the node
)

func netDef(typ, display string, aliases ...string) talos.ResourceDef {
	return talos.ResourceDef{Type: typ, DisplayType: display, Aliases: aliases, DefaultNamespace: "network"}
}

func linkGraph() talos.DepGraph {
	return talos.DepGraph{Edges: []talos.DepEdge{
		{Controller: "network.LinkStatusController", Type: tLink, Output: true},
		{Controller: "network.LinkStatusController", Type: tSpec},
		{Controller: "network.LinkStatusController", Type: tGhost},
		{Controller: "network.AddressStatusController", Type: tLink},
		{Controller: "network.AddressStatusController", Type: tAddr, Output: true},
	}}
}

// describeApp is a browser over LinkStatus (with a note), LinkSpec and
// AddressStatus, with the dependency graph cached, and the describe pane open
// on LinkStatus.
func describeApp(width, height int, g talos.DepGraph) App {
	app := browserApp(width, height, 2, 4)
	app.browser.defs = append(app.browser.defs,
		netDef(tLink, "LinkStatus", "link", "links"), netDef(tSpec, "LinkSpec"), netDef(tAddr, "AddressStatus"))
	app.browser.counts = map[string]int{tLink: 1, tSpec: 3, tAddr: 2}
	app.browser.singles = map[string]talos.ResourceMeta{
		tLink: {Namespace: "network", Type: tLink, ID: "eth0", Owner: "network.LinkStatusController"},
	}
	app = app.setDeps(app.browser.node.IP, depEntry{g: g})
	return openDescribeOn(app, tLink)
}

func openDescribeOn(app App, display string) App {
	for _, d := range app.browser.defs {
		if d.Type == display || d.DisplayType == display {
			sub := descSubject{def: d}
			if only, ok := app.browser.singles[d.Type]; ok {
				sub.meta, sub.hasMeta = only, true
			}
			app.browser = app.browser.push(pane{kind: paneDescribe, title: "Describe " + d.DisplayType, sub: sub})
			return app.syncBrowserState()
		}
	}
	panic("no def " + display)
}

func TestDescribeResourceSections(t *testing.T) {
	app := describeApp(120, 50, linkGraph())
	p, _ := app.browser.top()
	text := describeText(app, p, 110)
	for _, want := range []string{
		"LinkStatus (" + tLink + " · ns network · aliases link, links)",
		"WHAT", "live state of a network link",
		"ON UBUNTU", "ip -d link show",
		"LOOK HERE", "link is down or has the wrong MTU · a bond",
		"WRITTEN BY", "network.LinkStatusController (owner of eth0)",
		"FED BY", "├─◀ " + tGhost, "└─◀ " + tSpec,
		"FEEDS", "network.AddressStatusController", "└─▶ " + tAddr,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	// FED BY comes before FEEDS
	if strings.Index(text, "FED BY") > strings.Index(text, "FEEDS") {
		t.Error("FED BY should precede FEEDS")
	}
}

func TestDescribeOmitsSectionsWithoutData(t *testing.T) {
	app := describeApp(120, 50, linkGraph())
	app.browser = app.browser.pop()
	app = openDescribeOn(app, "Thing00") // no note, no instance owner, no graph edges
	p, _ := app.browser.top()
	text := describeText(app, p, 110)
	for _, gone := range []string{"WHAT", "ON UBUNTU", "LOOK HERE", "WRITTEN BY", "FED BY", "FEEDS"} {
		if strings.Contains(text, gone) {
			t.Errorf("section %q should be omitted:\n%s", gone, text)
		}
	}
	if !strings.Contains(text, "Thing00") {
		t.Errorf("title missing:\n%s", text)
	}
}

func TestDescribeGraphUnavailableNotes(t *testing.T) {
	cases := []struct {
		name  string
		entry *depEntry
		want  string
	}{
		{"not loaded", nil, "loading relationships"},
		{"loading", &depEntry{loading: true}, "loading relationships"},
		{"needs grpc", &depEntry{err: talos.ErrNeedsGRPC}, "relationships need the gRPC source (--source=grpc)"},
		{"other error", &depEntry{err: errors.New("rpc error: code = Unavailable desc = boom")}, "relationships unavailable: boom"},
	}
	for _, c := range cases {
		app := describeApp(120, 50, linkGraph())
		app.deps = nil
		if c.entry != nil {
			app = app.setDeps(app.browser.node.IP, *c.entry)
		}
		p, _ := app.browser.top()
		text := describeText(app, p, 110)
		if !strings.Contains(text, c.want) || strings.Contains(text, "FED BY") {
			t.Errorf("%s: want only %q:\n%s", c.name, c.want, text)
		}
		// the knowledge sections still show
		if !strings.Contains(text, "WHAT") {
			t.Errorf("%s: notes should show without a graph", c.name)
		}
	}
}

func TestDescribeTreeCollapse(t *testing.T) {
	var g talos.DepGraph
	g.Edges = append(g.Edges, talos.DepEdge{Controller: "c.Writer", Type: tLink, Output: true})
	for i := 0; i < 9; i++ {
		g.Edges = append(g.Edges, talos.DepEdge{Controller: "c.Writer", Type: fmt.Sprintf("In%02d.net.talos.dev", i)})
	}
	for i := 0; i < 8; i++ {
		g.Edges = append(g.Edges, talos.DepEdge{Controller: fmt.Sprintf("c.Reader%d", i), Type: tLink})
	}
	app := describeApp(120, 80, g)
	p, _ := app.browser.top()
	text := describeText(app, p, 110)
	if !strings.Contains(text, "… 3 more") {
		t.Errorf("9 inputs should collapse to 6 + '… 3 more':\n%s", text)
	}
	if strings.Contains(text, "In06") {
		t.Errorf("seventh input shown:\n%s", text)
	}
	if !strings.Contains(text, "… 2 more controllers") || strings.Contains(text, "c.Reader6") {
		t.Errorf("8 consumers should collapse to 6 + '… 2 more controllers':\n%s", text)
	}
}

func TestDescribeSelectsOnlyTypesOnTheNode(t *testing.T) {
	app := describeApp(120, 50, linkGraph())
	p, _ := app.browser.top()
	vl := describeVisual(app.describeRows(p), 110)
	var sel []string
	for _, i := range selectableLines(vl) {
		sel = append(sel, strings.TrimSpace(vl[i].text))
	}
	// LinkSpecs (FED BY), AddressStatuses (FEEDS): present on the node.
	// Ghosts is in the graph only: dim, not selectable.
	if len(sel) != 2 || !strings.HasSuffix(sel[0], tSpec) || !strings.HasSuffix(sel[1], tAddr) {
		t.Errorf("selectable rows = %q", sel)
	}
	for _, l := range vl {
		if strings.Contains(l.text, tGhost) && (l.sel || !l.dim) {
			t.Errorf("ghost row should be dim and not selectable: %+v", l)
		}
	}
}

func TestDescribeMoveAndJumpFromFedByRow(t *testing.T) {
	app := describeApp(120, 50, linkGraph())
	// down moves to the second selectable row (FEEDS → AddressStatus), up back
	app = press(t, app, "j")
	if p, _ := app.browser.top(); p.cur != 1 {
		t.Fatalf("j: cur = %d, want 1", p.cur)
	}
	app = press(t, app, "k")
	if p, _ := app.browser.top(); p.cur != 0 {
		t.Fatalf("k: cur = %d, want 0", p.cur)
	}
	// enter on the FED BY row jumps to LinkSpec (3 instances → instance list)
	app = press(t, app, "enter")
	top, _ := app.browser.top()
	if top.kind != paneInstances || top.def.Type != tSpec {
		t.Fatalf("after jump: top = kind %d %s, want the LinkSpec instance list", top.kind, top.def.Type)
	}
	// esc behaves like a palette jump: back to the types pane of its category
	app = press(t, app, "esc")
	top, _ = app.browser.top()
	if top.kind != paneTypes || top.category != testNet {
		t.Errorf("esc after jump: top = %+v, want the networking types pane", top)
	}
	if len(app.browser.stack) != 2 {
		t.Errorf("stack depth = %d, want 2 (categories, types)", len(app.browser.stack))
	}
}

func TestDescribeEnterOnNothingSelectableDoesNothing(t *testing.T) {
	app := describeApp(120, 50, talos.DepGraph{})
	depth := len(app.browser.stack)
	app = press(t, app, "enter")
	if len(app.browser.stack) != depth {
		t.Error("enter with no selectable row changed the stack")
	}
}

func TestDescribeSelectionStaysVisibleWhenScrolling(t *testing.T) {
	var g talos.DepGraph
	g.Edges = append(g.Edges, talos.DepEdge{Controller: "c.W", Type: tLink, Output: true})
	for i := 0; i < 6; i++ {
		g.Edges = append(g.Edges, talos.DepEdge{Controller: "c.W", Type: tSpec + fmt.Sprint(i)})
	}
	app := describeApp(80, 21, g) // short: the tree does not fit
	for i := 0; i < 6; i++ {
		app.browser.defs = append(app.browser.defs, netDef(tSpec+fmt.Sprint(i), fmt.Sprint("Spec", i)))
	}
	for i := 0; i < 5; i++ {
		app = press(t, app, "j")
		checkBudget(t, app, 80)
	}
	p, _ := app.browser.top()
	vl := describeVisual(app.describeRows(p), app.yamlInnerWidth())
	line := selectableLines(vl)
	if len(line) == 0 {
		t.Fatal("no selectable rows")
	}
	rows := app.paneInnerRows(paneDescribe)
	at := line[min(p.cur, len(line)-1)]
	if at < p.scroll || at >= p.scroll+rows {
		t.Errorf("selected line %d outside the window [%d,%d)", at, p.scroll, p.scroll+rows)
	}
}

func TestDescribeWrapsAt80Columns(t *testing.T) {
	long := "network.AVeryLongControllerNameThatDoesNotFitInAnEightyColumnTerminalAtAll"
	g := talos.DepGraph{Edges: []talos.DepEdge{
		{Controller: long, Type: tLink, Output: true},
		{Controller: long, Type: "A.Very.Long.Resource.Type.Name.That.Also.Does.Not.Fit.net.talos.dev"},
	}}
	app := describeApp(80, 40, g)
	out := checkBudget(t, app, 80)
	if !strings.Contains(out, "FED BY") {
		t.Errorf("FED BY missing:\n%s", out)
	}
	p, _ := app.browser.top()
	for _, l := range describeVisual(app.describeRows(p), 78) {
		if w := len([]rune(l.label)) + len([]rune(l.text)); l.label != "" && w > 78+descLabelW {
			t.Errorf("line too wide: %+v", l)
		}
	}
}

func TestDescribeLoadsGraphOnceAndReloads(t *testing.T) {
	fs := newFakeSource("grpc")
	fs.deps = linkGraph()
	app := browserApp(120, 40, 2, 4)
	app.source = fs
	app, cmd := app.openDescribe()
	if cmd == nil {
		t.Fatal("first describe did not load the graph")
	}
	if e := app.deps[app.browser.node.IP]; !e.loading {
		t.Errorf("entry = %+v, want loading", e)
	}
	msg := cmd().(depsMsg)
	app = app.handleDeps(msg)
	if e := app.deps[app.browser.node.IP]; e.loading || len(e.g.Edges) != len(fs.deps.Edges) {
		t.Errorf("graph not cached: %+v", e)
	}
	// describing again uses the cache
	app = press(t, app, "esc")
	if _, cmd := app.openDescribe(); cmd != nil {
		t.Error("cached graph was fetched again")
	}
	// ctrl+r on the describe pane reloads it
	app, _ = app.openDescribe()
	a2, cmd := app.browserReload()
	if cmd == nil || fs.depCalls.Load() != 1 {
		t.Fatalf("reload: cmd=%v calls=%d", cmd != nil, fs.depCalls.Load())
	}
	cmd()
	if fs.depCalls.Load() != 2 {
		t.Errorf("reload did not call Dependencies again: %d", fs.depCalls.Load())
	}
	if !a2.deps[a2.browser.node.IP].loading {
		t.Error("reload should mark the entry loading")
	}
}

func TestDescribeConfigKindFeedsFromMachineConfigConsumers(t *testing.T) {
	g := talos.DepGraph{Edges: []talos.DepEdge{
		{Controller: "network.LinkSpecController", Type: machineConfigType},
		{Controller: "network.LinkSpecController", Type: tSpec, Output: true},
		{Controller: "block.VolumeConfigController", Type: machineConfigType},
	}}
	app := cfgApp(120, 50, 3)
	app = app.setDeps(app.browser.node.IP, depEntry{g: g})
	app = press(t, app, "j", "d") // LinkConfig: group network
	p, _ := app.browser.top()
	text := describeText(app, p, 110)
	if !strings.Contains(text, "FEEDS") || !strings.Contains(text, "network.LinkSpecController") {
		t.Errorf("config FEEDS missing:\n%s", text)
	}
	if strings.Contains(text, "block.VolumeConfigController") {
		t.Errorf("a network kind should list network controllers first/only:\n%s", text)
	}
	if !strings.Contains(text, "ON UBUNTU") {
		t.Errorf("ON UBUNTU missing for a config kind:\n%s", text)
	}
}
