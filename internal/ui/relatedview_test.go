package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/talos"
)

const (
	tLinkSpec    = "LinkSpecs.net.talos.dev"
	tLinkStatus  = "LinkStatuses.net.talos.dev"
	tAddrSpec    = "AddressSpecs.net.talos.dev"
	tAddrStatus  = "AddressStatuses.net.talos.dev"
	tNodeAddress = "NodeAddresses.net.talos.dev"
	tHostnameSt  = "HostnameStatuses.net.talos.dev"
)

func relNetDef(typ, display string) talos.ResourceDef {
	return talos.ResourceDef{Type: typ, DisplayType: display, DefaultNamespace: "network"}
}

// relGraph is a small network pipeline: config → specs → merged → status.
func relGraph() talos.DepGraph {
	in := func(c, t string) talos.DepEdge { return talos.DepEdge{Controller: c, Type: t} }
	out := func(c, t string) talos.DepEdge { return talos.DepEdge{Controller: c, Type: t, Output: true} }
	return talos.DepGraph{Edges: []talos.DepEdge{
		in("network.LinkConfigController", machineConfigType),
		out("network.LinkConfigController", tLinkSpec),
		in("network.PlatformConfigController", machineConfigType),
		out("network.PlatformConfigController", tLinkSpec),
		in("network.LinkSpecController", tLinkSpec),
		out("network.LinkSpecController", tLinkStatus),
		in("network.AddressConfigController", machineConfigType),
		out("network.AddressConfigController", tAddrSpec),
		in("network.AddressSpecController", tAddrSpec),
		in("network.AddressSpecController", tLinkStatus),
		out("network.AddressSpecController", tAddrStatus),
		in("network.NodeAddressController", tAddrStatus),
		out("network.NodeAddressController", tNodeAddress),
		in("network.HostnameController", tLinkStatus),
		out("network.HostnameController", tHostnameSt),
	}}
}

// relApp is a browser on the networking types pane with a LinkSpec related
// view on top, fully loaded.
func relApp(w, h int) App {
	app := newTestApp(w, h)
	app.nodes = makeNodes(3)
	app.nodeCur = 1
	n := app.nodes[1]
	app.selNode = &n
	defs := []talos.ResourceDef{
		relNetDef(tLinkSpec, "LinkSpec"), relNetDef(tLinkStatus, "LinkStatus"),
		relNetDef(tAddrSpec, "AddressSpec"), relNetDef(tAddrStatus, "AddressStatus"),
		relNetDef(tNodeAddress, "NodeAddress"), relNetDef(tHostnameSt, "HostnameStatus"),
		{Type: machineConfigType, DisplayType: "MachineConfig", DefaultNamespace: "config"},
	}
	counts := map[string]int{tLinkSpec: 3, tLinkStatus: 4, tAddrSpec: 2, tAddrStatus: 2, tNodeAddress: 1, tHostnameSt: 0, machineConfigType: 1}
	b := browser{node: n, defs: defs, counts: counts, cfgState: cfgLoaded,
		docs: []talos.ConfigDoc{{Kind: "LinkConfig", Name: "eth0", YAML: "apiVersion: v1alpha1\nkind: LinkConfig\nname: eth0\nup: true\n"}}}
	b.stack = []pane{
		{kind: paneCategories, title: "Categories"},
		{kind: paneTypes, title: "Networking", category: testNet},
	}
	app.browser = b
	app.deps = map[string]depEntry{n.IP: {g: relGraph()}}
	app = app.syncBrowserState()
	app, _ = app.pushRelated(subjectOfDef(defs[0]), nil)
	list := func(ns, typ string, ids ...string) {
		app = app.handleRelList(relListMsg{node: n.IP, ns: ns, typ: typ, items: metas(ns, typ, ids...)})
	}
	list("network-config", tLinkSpec, "configuration/eth0", "operator/eth0", "platform/eth1", "default/lo")
	list("network", tLinkSpec, "eth0", "eth1", "lo")
	list("network", tLinkStatus, "eth0", "eth1", "lo", "eth9")
	return app
}

var relSizes = []struct{ w, h int }{{80, 24}, {120, 40}, {200, 50}}

func TestRelatedRendersWithinBudget(t *testing.T) {
	for _, sz := range relSizes {
		sz := sz
		t.Run(fmt.Sprintf("w%d_h%d", sz.w, sz.h), func(t *testing.T) {
			app := relApp(sz.w, sz.h)
			out := checkBudget(t, app, sz.w)
			wants := []string{"PIPELINE", "FAMILY", "LinkSpec", "eth0"}
			if sz.w >= 120 {
				wants = append(wants, "LinkStatus", "lo")
			}
			for _, want := range wants {
				if !strings.Contains(ansi.Strip(out), want) {
					t.Errorf("%q missing:\n%s", want, ansi.Strip(out))
				}
			}
			// every line, ANSI stripped, fits the terminal
			for i, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if w := ansi.StringWidth(l); w > sz.w {
					t.Errorf("line %d is %d cells, terminal is %d", i, w, sz.w)
				}
			}
			// walk every box and cell: the layout must hold everywhere
			for i := 0; i < 6; i++ {
				app = press(t, app, "right")
				checkBudget(t, app, sz.w)
			}
			app = press(t, app, "tab")
			for i := 0; i < 12; i++ {
				app = press(t, app, "right")
				app = press(t, app, "down")
				checkBudget(t, app, sz.w)
			}
		})
	}
}

func TestRelatedLayoutSwitchesAt120(t *testing.T) {
	stages, _ := relApp(120, 40).relPipeline(subjectOfDef(relNetDef(tLinkSpec, "LinkSpec")))
	if h, _, _ := relLayout(118, stages); !h {
		t.Error("120 columns should lay the pipeline out left to right")
	}
	if h, _, _ := relLayout(78, stages); h {
		t.Error("80 columns should stack the pipeline")
	}
	out := ansi.Strip(checkBudget(t, relApp(80, 24), 80))
	if !strings.Contains(out, "▼") {
		t.Errorf("stacked layout has no ▼ connector:\n%s", out)
	}
	wide := ansi.Strip(checkBudget(t, relApp(200, 50), 200))
	if !strings.Contains(wide, "▶") || strings.Contains(wide, "▼") {
		t.Errorf("wide layout should use ▶ connectors only:\n%s", wide)
	}
}

func TestRelatedPipelineShowsConfigRootAndControllers(t *testing.T) {
	out := ansi.Strip(checkBudget(t, relApp(200, 50), 200))
	for _, want := range []string{"MachineConfig", "LinkConfig", "PlatformConfig", "LinkStatus"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing:\n%s", want, out)
		}
	}
}

func TestRelatedKeysAndFocus(t *testing.T) {
	app := relApp(200, 50)
	p, _ := app.relPane()
	if p.rel.focus != relFocusPipeline {
		t.Fatal("pipeline should have the focus first")
	}
	// the cursor starts on the subject and moves to the next stage with →
	if got := app.relSelectedTyp(p); got != tLinkSpec {
		t.Fatalf("selected = %q, want the subject", got)
	}
	app = press(t, app, "right")
	p, _ = app.relPane()
	if got := app.relSelectedTyp(p); got != tLinkStatus {
		t.Errorf("after → selected = %q, want %q", got, tLinkStatus)
	}
	app = press(t, app, "left", "left")
	p, _ = app.relPane()
	if got := app.relSelectedTyp(p); got != machineConfigType {
		t.Errorf("two ← selected = %q, want the machine config", got)
	}
	app = press(t, app, "tab")
	if p, _ := app.relPane(); p.rel.focus != relFocusTable {
		t.Error("tab did not move the focus to the table")
	}
	app = press(t, app, "esc")
	if app.state != StateCategories && len(app.browser.stack) != 2 {
		t.Errorf("esc did not return to the types pane (stack %d)", len(app.browser.stack))
	}
}

func TestRelatedMarkAndDiffTwoCells(t *testing.T) {
	app := relApp(200, 50)
	app = press(t, app, "tab")
	p, _ := app.relPane()
	tb := app.relTable(p.rel)
	// columns: LinkConfig, LinkSpec@configuration, @operator, @platform, @default, LinkSpec, LinkStatus
	if len(tb.cols) != 7 {
		t.Fatalf("columns = %d", len(tb.cols))
	}
	// row 0 is eth0. Mark the merged LinkSpec (col 5), then LinkStatus (col 6).
	app = press(t, app, "right", "right", "right", "right", "right", " ")
	if p, _ := app.relPane(); len(p.rel.marks) != 1 {
		t.Fatalf("marks = %v", p.rel.marks)
	}
	app = press(t, app, "c")
	if len(app.browser.stack) != 3 || !strings.Contains(app.statusMsg, "mark two") {
		t.Fatalf("c with one mark should not diff: %q", app.statusMsg)
	}
	app = press(t, app, "right", " ")
	p, _ = app.relPane()
	if len(p.rel.marks) != 2 {
		t.Fatalf("marks = %v", p.rel.marks)
	}
	// a third mark drops the oldest
	app = press(t, app, "down", " ")
	p, _ = app.relPane()
	if len(p.rel.marks) != 2 || p.rel.marks[0] != (relMark{"eth0", "LinkStatus"}) || p.rel.marks[1] != (relMark{"eth1", "LinkStatus"}) {
		t.Errorf("marks after a third = %v", p.rel.marks)
	}
	// unmarking: space on a marked cell
	app = press(t, app, " ")
	if p, _ := app.relPane(); len(p.rel.marks) != 1 {
		t.Errorf("space on a marked cell should unmark it: %v", p.rel.marks)
	}
	app = app.setRel(func(rv *relatedView) { rv.marks = []relMark{{"eth0", "LinkSpec"}, {"eth0", "LinkStatus"}} })
	app, _ = app.relDiff()
	p, _ = app.relPane()
	if p.rel.dseq == 0 {
		t.Fatal("diff did not start")
	}
	y := func(extra string) string {
		return "node: 10.0.0.1\nmetadata:\n  namespace: network\n  type: T\n  id: eth0\n  version: 3\n  phase: running\nspec:\n  up: true\n" + extra
	}
	app = app.handleRelYAML(relYAMLMsg{node: app.browser.node.IP, seq: p.rel.dseq, slot: 0, yaml: y("  mtu: 1500\n")})
	app = app.handleRelYAML(relYAMLMsg{node: app.browser.node.IP, seq: p.rel.dseq, slot: 1, yaml: y("  mtu: 1450\n")})
	top, _ := app.browser.top()
	if top.kind != paneDiff {
		t.Fatalf("top = %v, want the diff pane", top.kind)
	}
	var minus, plus bool
	for _, l := range top.diff {
		minus = minus || (l.op == '-' && strings.Contains(l.text, "1500"))
		plus = plus || (l.op == '+' && strings.Contains(l.text, "1450"))
		if strings.Contains(l.text, "version") || strings.Contains(l.text, "node:") {
			t.Errorf("metadata leaked into the diff: %+v", l)
		}
	}
	if !minus || !plus {
		t.Errorf("diff lacks the mtu change: %+v", top.diff)
	}
	checkBudget(t, app, 200)
	app = press(t, app, "esc")
	if p, ok := app.relPane(); !ok {
		t.Error("esc from the diff should return to the related view")
	} else if len(p.rel.marks) != 2 {
		t.Error("marks should survive the diff")
	}
}

func TestRelatedEscClearsMarksFirst(t *testing.T) {
	app := relApp(120, 40)
	app = press(t, app, "tab", " ")
	if p, _ := app.relPane(); len(p.rel.marks) != 1 {
		// cursor starts on the config column of eth0, which is present
		t.Fatalf("marks = %v", p.rel.marks)
	}
	app = press(t, app, "esc")
	p, ok := app.relPane()
	if !ok || len(p.rel.marks) != 0 {
		t.Fatalf("first esc should clear the marks, not close the view (ok=%v)", ok)
	}
	app = press(t, app, "esc")
	if _, ok := app.relPane(); ok {
		t.Error("second esc should close the view")
	}
}

func TestRelatedEnterOpensYAMLAndEscReturns(t *testing.T) {
	app := relApp(120, 40)
	app = press(t, app, "tab", "right", "enter") // eth0 / LinkSpec@configuration
	top, _ := app.browser.top()
	if top.kind != paneYAML || top.meta.ID != "configuration/eth0" {
		t.Fatalf("top = %+v", top)
	}
	out := checkBudget(t, app, 120)
	if strings.Contains(ansi.Strip(out), "PIPELINE") {
		t.Error("the related view should not show beside the YAML pane")
	}
	app = press(t, app, "esc")
	if _, ok := app.relPane(); !ok {
		t.Error("esc from the YAML did not return to the related view")
	}
}

func TestRelatedEnterOnBoxPushesTypeOnTop(t *testing.T) {
	app := relApp(200, 50)
	app = press(t, app, "right", "enter") // LinkStatus has 4 instances
	top, _ := app.browser.top()
	if top.kind != paneInstances || top.def.Type != tLinkStatus {
		t.Fatalf("top = %+v", top)
	}
	app = press(t, app, "esc")
	if _, ok := app.relPane(); !ok {
		t.Error("esc did not return to the related view")
	}
}

func TestRelatedOpenedWithPFromEveryPane(t *testing.T) {
	app := browserApp(120, 40, 3, 4) // instances pane
	app = press(t, app, "p")
	if top, _ := app.browser.top(); top.kind != paneRelated {
		t.Errorf("p on instances: top = %v", top.kind)
	}
	app = browserApp(120, 40, 2, 4)
	app = press(t, app, "p")
	if top, _ := app.browser.top(); top.kind != paneRelated {
		t.Errorf("p on types: top = %v", top.kind)
	}
	app = browserApp(120, 40, 4, 4)
	app = press(t, app, "p")
	if top, _ := app.browser.top(); top.kind != paneRelated {
		t.Errorf("p on YAML: top = %v", top.kind)
	}
}

func TestRelatedTableScrollsToLastRowAtSmallSize(t *testing.T) {
	app := relApp(80, 24)
	app = press(t, app, "tab", "G")
	out := ansi.Strip(checkBudget(t, app, 80))
	if !strings.Contains(out, "lo ") {
		t.Errorf("last row not visible after G:\n%s", out)
	}
}

// instApp is relApp's browser with a LinkStatus instance list on top.
func instApp(owner string) App {
	app := relApp(200, 50)
	app, _ = app.handleKey(key("esc")) // back to the types pane
	def := relNetDef(tLinkStatus, "LinkStatus")
	m := talos.ResourceMeta{Namespace: "network", Type: tLinkStatus, ID: "eth0", Owner: owner}
	app.browser = app.browser.push(pane{kind: paneInstances, title: "LinkStatus", def: def, items: []talos.ResourceMeta{m}})
	return app.syncBrowserState()
}

func TestJumpToWriterSingleInputJumpsThere(t *testing.T) {
	// LinkSpecController reads only LinkSpecs and LinkStatuses (as inputs): extend with a one-input controller
	app := instApp("network.NodeAddressController") // reads AddressStatuses only
	app = press(t, app, "J")
	top, _ := app.browser.top()
	if top.kind != paneInstances || top.def.Type != tAddrStatus {
		t.Fatalf("top = %+v, want AddressStatus instances", top)
	}
}

func TestJumpToWriterSeveralInputsOpensRelatedHighlighted(t *testing.T) {
	app := instApp("network.AddressSpecController") // reads AddressSpecs and LinkStatuses
	app = press(t, app, "J")
	p, ok := app.relPane()
	if !ok || !p.rel.hi[tAddrSpec] || !p.rel.hi[tLinkStatus] || p.rel.hiNote == "" {
		t.Fatalf("related view not opened with highlights: ok=%v %+v", ok, p.rel.hi)
	}
	checkBudget(t, app, 200)
}

func TestJumpToWriterExplainsWhenThereIsNoOwner(t *testing.T) {
	app := instApp("")
	app = press(t, app, "J")
	if !strings.Contains(app.statusMsg, "no owner controller") {
		t.Errorf("status = %q", app.statusMsg)
	}
	if top, _ := app.browser.top(); top.kind != paneInstances {
		t.Error("J without an owner must not move")
	}
}
