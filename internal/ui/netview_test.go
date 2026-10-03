package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/netmodel"
	"github.com/florianspk/t9s/internal/talos"
)

func netDefs() []talos.ResourceDef {
	mk := func(typ, display string) talos.ResourceDef {
		return talos.ResourceDef{Type: typ, DisplayType: display, DefaultNamespace: "network"}
	}
	return []talos.ResourceDef{
		mk(netmodel.TypeLinkStatus, "LinkStatus"), mk(netmodel.TypeAddrStatus, "AddressStatus"),
		mk(netmodel.TypeRouteStatus, "RouteStatus"), mk(netmodel.TypeLinkSpec, "LinkSpec"),
		mk(netmodel.TypeOperatorSpec, "OperatorSpec"),
	}
}

// netApp is a browser with the network view on top, loaded from a fixture.
func netApp(t *testing.T, fixture string, w, h int) App {
	t.Helper()
	in := netmodel.Fixture(fixture)
	app := newTestApp(w, h)
	app.nodes = makeNodes(3)
	app.nodeCur = 1
	n := app.nodes[1]
	app.selNode = &n
	app.browser = browser{node: n, defs: netDefs(), cfgState: cfgLoaded, docs: in.Docs, stack: []pane{{kind: paneCategories}}}
	app = app.syncBrowserState()
	app, _ = app.openNetwork(true)
	app = app.handleNetFetch(netFetchMsg{node: n.IP, seq: 1, res: netmodel.FetchResult{In: in}})
	return app
}

func netText(app App) string { return ansi.Strip(app.renderBrowser(app.mainHeight())) }

func TestNetTreeRendersEachFixture(t *testing.T) {
	cases := map[string][]string{
		"single-nic-dhcp": {"eth0", "10.0.0.5/24", "dhcp4", "default via 10.0.0.1", "1000Mb/s", "e1000e", "hostname cp-1", "dns 1.1.1.1, 8.8.8.8"},
		"bond-vlan-vip":   {"bond0", "bond0.100", "static", "vip", "BondConfig/bond0", "Layer2VIPConfig/10.0.0.10", "⚠ eth9", "802.3ad", "vlan 100", "↪ bond0"},
		"bridge":          {"br0", "eth0", "bridge", "BridgeConfig/br0"},
		"config-only":     {"⚠ eth9", "no such link", "10.9.0.0/16", "eth7"},
	}
	for name, wants := range cases {
		t.Run(name, func(t *testing.T) {
			out := netText(netApp(t, name, 200, 50))
			for _, w := range wants {
				if !strings.Contains(out, w) {
					t.Errorf("%q missing:\n%s", w, out)
				}
			}
		})
	}
}

func TestNetTreeShape(t *testing.T) {
	app := netApp(t, "bond-vlan-vip", 200, 50)
	p, _ := app.netPane()
	var keys []string
	for _, f := range app.netRows(p) {
		keys = append(keys, f.node.key)
	}
	got := strings.Join(keys, " ")
	// eth0 first with the bond under it, the VLAN under the bond; eth1 only refers to the bond
	order := []string{"link:eth0", "link:bond0", "addr:bond0|10.0.0.5/24", "link:bond0.100", "link:eth1", "ref:eth1>bond0", "link:lo"}
	pos := -1
	for _, k := range order {
		i := strings.Index(got, k)
		if i < 0 || i < pos {
			t.Fatalf("row %q out of order or missing in:\n%s", k, strings.Join(keys, "\n"))
		}
		pos = i
	}
	if !strings.Contains(got, "warn:0|LinkConfig|eth9") {
		t.Errorf("warning row missing:\n%s", got)
	}
}

func TestNetRendersWithinBudget(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{80, 24}, {120, 40}, {200, 50}} {
		for _, fx := range netmodel.FixtureNames() {
			sz, fx := sz, fx
			t.Run(fmt.Sprintf("%s_w%d_h%d", fx, sz.w, sz.h), func(t *testing.T) {
				app := netApp(t, fx, sz.w, sz.h)
				p, _ := app.netPane()
				n := len(app.netRows(p))
				for i := 0; i < n+1; i++ {
					out := checkBudget(t, app, sz.w)
					for j, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
						if w := ansi.StringWidth(l); w > sz.w {
							t.Fatalf("step %d line %d is %d cells, terminal is %d", i, j, w, sz.w)
						}
					}
					// the selected row is on screen
					p, _ = app.netPane()
					f, _, _ := app.netSelected(p)
					plain := ansi.Strip(app.netRowText(p.net.model, f.node))
					if head := cutWidth(plain, 8); !strings.Contains(ansi.Strip(out), head) {
						t.Fatalf("step %d: selected row %q not visible:\n%s", i, plain, ansi.Strip(out))
					}
					app = press(t, app, "down")
				}
			})
		}
	}
}

func TestNetDetailBelowTreeWhenNarrow(t *testing.T) {
	narrow := netText(netApp(t, "bond-vlan-vip", 80, 40))
	if !strings.Contains(narrow, "─────") || !strings.Contains(narrow, "physical NIC") {
		t.Errorf("narrow layout: detail should sit below the tree:\n%s", narrow)
	}
	wide := netText(netApp(t, "bond-vlan-vip", 160, 40))
	if !strings.Contains(wide, "│") || !strings.Contains(wide, "physical NIC") {
		t.Errorf("wide layout: detail should sit beside the tree:\n%s", wide)
	}
}

func selectKey(app App, key string) App {
	p, _ := app.netPane()
	flat := app.netRows(p)
	return app.netSelect(flat, netIndexOf(flat, key))
}

func TestNetMoveAndFold(t *testing.T) {
	app := netApp(t, "bond-vlan-vip", 200, 50)
	p, _ := app.netPane()
	if p.net.sel != "link:eth0" {
		t.Fatalf("first selection = %q, want link:eth0", p.net.sel)
	}
	app = press(t, app, "j", "j")
	if p, _ = app.netPane(); p.net.sel != "addr:bond0|10.0.0.5/24" {
		t.Errorf("after j j: %q", p.net.sel)
	}
	app = press(t, app, "G")
	if p, _ = app.netPane(); !strings.HasPrefix(p.net.sel, "warn:") {
		t.Errorf("G: %q, want the last row (a warning)", p.net.sel)
	}
	app = press(t, app, "g")
	if p, _ = app.netPane(); p.net.sel != "link:eth0" {
		t.Errorf("g: %q", p.net.sel)
	}
	// collapse eth0: the bond and everything under it disappears, a count shows
	app = press(t, app, "h")
	out := netText(app)
	if strings.Contains(out, "bond0.100") || !strings.Contains(out, "[+") {
		t.Errorf("collapsed eth0 still shows its subtree:\n%s", out)
	}
	app = press(t, app, "l")
	if !strings.Contains(netText(app), "bond0.100") {
		t.Error("l should expand again")
	}
	// h on a leaf goes to the parent row
	app = selectKey(app, "addr:bond0|10.0.0.5/24")
	app = press(t, app, "h")
	if p, _ = app.netPane(); p.net.sel != "link:bond0" {
		t.Errorf("h on a leaf: %q, want its parent link:bond0", p.net.sel)
	}
	app = press(t, app, "h", "h")
	if p, _ = app.netPane(); !p.net.collapsed["link:bond0"] || p.net.sel != "link:eth0" {
		t.Errorf("h h: collapsed=%v sel=%q", p.net.collapsed, p.net.sel)
	}
}

func TestNetEnterOpensYAMLAndEscReturns(t *testing.T) {
	app := netApp(t, "bond-vlan-vip", 200, 50)
	app = selectKey(app, "link:bond0")
	app, cmd := app.handleKey(key("enter"))
	top, _ := app.browser.top()
	if top.kind != paneYAML || top.meta.ID != "bond0" || top.def.Type != netmodel.TypeLinkStatus || cmd == nil {
		t.Fatalf("enter on a link: top = %+v", top)
	}
	app = press(t, app, "esc")
	if top, _ = app.browser.top(); top.kind != paneNetwork {
		t.Fatalf("esc should return to the network view, got %v", top.kind)
	}
	if top.net.sel != "link:bond0" {
		t.Errorf("selection lost: %q", top.net.sel)
	}
	// an address opens its AddressStatus
	app = selectKey(app, "addr:bond0|10.0.0.5/24")
	app, _ = app.handleKey(key("enter"))
	if top, _ = app.browser.top(); top.kind != paneYAML || top.def.Type != netmodel.TypeAddrStatus || top.meta.ID != "bond0/10.0.0.5/24" {
		t.Errorf("enter on an address: %+v", top)
	}
}

func TestNetConfigJump(t *testing.T) {
	app := netApp(t, "bond-vlan-vip", 200, 50)

	// the static address came from the BondConfig
	app = selectKey(app, "addr:bond0|10.0.0.5/24")
	a := press(t, app, "c")
	top, _ := a.browser.top()
	if top.kind != paneYAML || top.cfgKind != "BondConfig" || !strings.Contains(top.yaml, "kind: BondConfig") {
		t.Fatalf("c on the static address: %+v", top)
	}
	if a = press(t, a, "esc"); a.state == StateNodeList {
		t.Fatal("esc left the browser")
	}

	// a link with two documents: c again moves on to the next one
	app = selectKey(app, "link:bond0")
	a = press(t, app, "c")
	if top, _ = a.browser.top(); top.cfgKind != "BondConfig" {
		t.Fatalf("first c: %+v", top)
	}
	a = press(t, a, "esc", "c")
	if top, _ = a.browser.top(); top.cfgKind != "Layer2VIPConfig" {
		t.Errorf("second c: %q, want Layer2VIPConfig", top.cfgKind)
	}

	// the warning opens the document that names the missing link
	app = selectKey(app, "warn:0|LinkConfig|eth9")
	a = press(t, app, "c")
	if top, _ = a.browser.top(); top.kind != paneYAML || top.cfgKind != "LinkConfig" || top.meta.ID != "eth9" {
		t.Errorf("c on the warning: %+v", top)
	}
	a = press(t, app, "enter")
	if top, _ = a.browser.top(); top.cfgKind != "LinkConfig" {
		t.Errorf("enter on the warning: %+v", top)
	}
}

func TestNetKeysSayWhyWhenTheyDoNotApply(t *testing.T) {
	// a Talos default address: no document asked for it
	app := netApp(t, "single-nic-dhcp", 200, 50)
	app = selectKey(app, "addr:lo|127.0.0.1/8")
	if a := press(t, app, "c"); !strings.Contains(a.statusMsg, "default") {
		t.Errorf("c on a default address: status %q", a.statusMsg)
	}
	// a physical NIC no document names
	app = selectKey(app, "link:lo")
	if a := press(t, app, "c"); a.statusMsg == "" {
		t.Error("c on a link without a document said nothing")
	}

	app = netApp(t, "bond-vlan-vip", 200, 50)
	app = selectKey(app, "warn:0|LinkConfig|eth9")
	app = press(t, app, "d")
	if top, _ := app.browser.top(); top.kind != paneDescribe || !top.sub.cfg || top.sub.ck.Kind != "LinkConfig" {
		t.Errorf("d on a warning should describe the config kind: %+v", top)
	}

	// without the machine config the document keys explain themselves
	in := netmodel.Fixture("bond-vlan-vip")
	in.Docs = nil
	bare := newTestApp(200, 50)
	bare.nodes = makeNodes(3)
	n := bare.nodes[1]
	bare.browser = browser{node: n, defs: netDefs(), cfgState: cfgDenied, stack: []pane{{kind: paneCategories}}}
	bare = bare.syncBrowserState()
	bare, _ = bare.openNetwork(true)
	bare = bare.handleNetFetch(netFetchMsg{node: n.IP, seq: 1, res: netmodel.FetchResult{In: in}})
	bare = selectKey(bare, "link:bond0")
	if a := press(t, bare, "c"); !strings.Contains(a.statusMsg, "os:admin") {
		t.Errorf("c without config: %q", a.statusMsg)
	}
	if out := netText(bare); !strings.Contains(out, "config documents need os:admin") {
		t.Errorf("summary should say the documents are unavailable:\n%s", out)
	}
}

func TestNetDescribeAndRelated(t *testing.T) {
	app := netApp(t, "bond-vlan-vip", 200, 50)
	app = selectKey(app, "link:bond0")
	a := press(t, app, "d")
	top, _ := a.browser.top()
	if top.kind != paneDescribe || top.sub.def.Type != netmodel.TypeLinkStatus || !top.sub.hasMeta || top.sub.meta.ID != "bond0" {
		t.Fatalf("d: %+v", top)
	}
	a = press(t, app, "p")
	top, _ = a.browser.top()
	if top.kind != paneRelated || top.rel.subject.def.Type != netmodel.TypeLinkStatus {
		t.Fatalf("p: %+v", top)
	}
	// before the definitions arrive d and p say so
	early := netApp(t, "bond-vlan-vip", 200, 50)
	early.browser.defs = nil
	early = selectKey(early, "link:bond0")
	if a := press(t, early, "p"); !strings.Contains(a.statusMsg, "definitions") {
		t.Errorf("p without definitions: %q", a.statusMsg)
	}
	// enter still works: the YAML pane needs no definition
	if a := press(t, early, "enter"); func() bool { top, _ := a.browser.top(); return top.kind != paneYAML }() {
		t.Error("enter without definitions should still open the YAML")
	}
}

func TestNetEntryPoints(t *testing.T) {
	// N on the node list: the view is the only pane, esc returns to the node
	app := newTestApp(120, 40)
	app.nodes = makeNodes(3)
	app.nodeCur = 2
	n := press(t, app, "N")
	if top, ok := n.browser.top(); !ok || top.kind != paneNetwork || len(n.browser.stack) != 1 || n.browser.node.IP != "10.0.0.3" {
		t.Fatalf("N: stack=%v", n.browser.stack)
	}
	if !isBrowserState(n.state) {
		t.Errorf("state = %v", n.state)
	}
	if back := press(t, n, "esc"); back.state != StateNodeList || back.nodeCur != 2 {
		t.Errorf("esc: state=%v nodeCur=%d", back.state, back.nodeCur)
	}
	if back := press(t, n, "q"); back.state != StateNodeList {
		t.Errorf("q: state=%v", back.state)
	}

	// :netview and :nv from the node list
	for _, word := range []string{"netview", "nv"} {
		c, _ := app.runCommand(word)
		if top, ok := c.browser.top(); !ok || top.kind != paneNetwork || len(c.browser.stack) != 1 {
			t.Errorf(":%s from the node list: stack=%v", word, c.browser.stack)
		}
	}

	// n in the Networking lists opens it on top of the stack; elsewhere it explains
	b := browserApp(120, 40, 2, 10)
	nb := press(t, b, "n")
	if top, _ := nb.browser.top(); top.kind != paneNetwork || len(nb.browser.stack) != 3 {
		t.Errorf("n in Networking types: stack=%d top=%v", len(nb.browser.stack), top.kind)
	}
	if e := press(t, nb, "esc"); func() bool { top, _ := e.browser.top(); return top.kind != paneTypes }() {
		t.Error("esc from the network view should return to the types pane")
	}
	other := b
	other.browser = other.browser.withTop(func(p *pane) { p.category = "block" })
	other = press(t, other, "n")
	if top, _ := other.browser.top(); top.kind == paneNetwork || !strings.Contains(other.statusMsg, "Networking") {
		t.Errorf("n outside Networking: top=%v status=%q", top.kind, other.statusMsg)
	}
	// :netview inside the browser
	in, _ := b.runCommand("netview")
	if top, _ := in.browser.top(); top.kind != paneNetwork || len(in.browser.stack) != 3 {
		t.Errorf(":netview in the browser: stack=%d", len(in.browser.stack))
	}
}

func TestNetReloadAndStaleFetch(t *testing.T) {
	app := netApp(t, "single-nic-dhcp", 120, 40)
	a, cmd := app.handleKey(key("ctrl+r"))
	p, _ := a.netPane()
	if p.net.seq != 2 || !p.net.loading || cmd == nil {
		t.Fatalf("ctrl+r: seq=%d loading=%v", p.net.seq, p.net.loading)
	}
	// the old fetch (seq 1) arriving late is ignored
	stale := netmodel.Fixture("bond-vlan-vip")
	a = a.handleNetFetch(netFetchMsg{node: a.browser.node.IP, seq: 1, res: netmodel.FetchResult{In: stale}})
	p, _ = a.netPane()
	if _, ok := p.net.model.Link("bond0"); ok || !p.net.loading {
		t.Error("a stale fetch replaced the model")
	}
	// another node's reply is ignored too
	a = a.handleNetFetch(netFetchMsg{node: "10.9.9.9", seq: 2, res: netmodel.FetchResult{In: stale}})
	if p, _ = a.netPane(); !p.net.loading {
		t.Error("a reply for another node was applied")
	}
}

func TestNetConfigArrivingLaterRebuilds(t *testing.T) {
	in := netmodel.Fixture("bond-vlan-vip")
	app := newTestApp(200, 50)
	app.nodes = makeNodes(3)
	n := app.nodes[1]
	app.selNode = &n
	app.browser = browser{node: n, defs: netDefs(), cfgState: cfgLoading, stack: []pane{{kind: paneCategories}}}
	app = app.syncBrowserState()
	app, _ = app.openNetwork(true)
	withoutDocs := in
	withoutDocs.Docs = nil
	app = app.handleNetFetch(netFetchMsg{node: n.IP, seq: 1, res: netmodel.FetchResult{In: withoutDocs}})
	if out := netText(app); strings.Contains(out, "⚠ eth9") || !strings.Contains(out, "loading machine config") {
		t.Fatalf("before the config arrives:\n%s", out)
	}
	app = app.handleConfigDocs(configDocsMsg{node: n.IP, docs: in.Docs})
	if out := netText(app); !strings.Contains(out, "⚠ eth9") || !strings.Contains(out, "BondConfig/bond0") {
		t.Fatalf("after the config arrives:\n%s", out)
	}
}

func TestNetUnreadableTypesAreListed(t *testing.T) {
	in := netmodel.Fixture("single-nic-dhcp")
	app := newTestApp(200, 50)
	app.nodes = makeNodes(3)
	n := app.nodes[1]
	app.browser = browser{node: n, defs: netDefs(), cfgState: cfgLoaded, docs: in.Docs, stack: []pane{{kind: paneCategories}}}
	app = app.syncBrowserState()
	app, _ = app.openNetwork(true)
	app = app.handleNetFetch(netFetchMsg{node: n.IP, seq: 1, res: netmodel.FetchResult{In: in, Denied: []string{"OperatorSpec"}}})
	app = selectKey(app, "note:unreadable")
	out := netText(app)
	if !strings.Contains(out, "OperatorSpec (needs os:admin)") {
		t.Errorf("denied type not listed:\n%s", out)
	}
}

func TestNetHintsAndHelpListTheView(t *testing.T) {
	app := netApp(t, "bond-vlan-vip", 200, 50)
	var hints []string
	for _, h := range stateHints(app) {
		hints = append(hints, h.key+" "+h.desc)
	}
	joined := strings.Join(hints, "|")
	for _, want := range []string{"YAML", "Describe", "config document", "HTML"} {
		if !strings.Contains(joined, want) {
			t.Errorf("hint bar lacks %q: %s", want, joined)
		}
	}
	if help := buildHelpContent(); !strings.Contains(help, "network view") && !strings.Contains(help, "Network view") {
		t.Error("help does not mention the network view")
	}
}
