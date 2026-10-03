package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const cmpType = "Thing00.net.talos.dev"

func resYAML(node, version, addr string) string {
	return fmt.Sprintf("node: %s\nmetadata:\n    namespace: network\n    type: %s\n    id: eth0\n    version: %s\n    created: 2026-01-0%sT10:00:00Z\n    updated: 2026-02-0%sT10:00:00Z\n    phase: running\nspec:\n    address: %s\n", node, cmpType, version, version, version, addr)
}

func TestNormalizeDropsPerNodeFields(t *testing.T) {
	norm, ver := normalizeResourceYAML(resYAML("10.0.0.2", "7", "10.0.0.1/24"))
	if ver != "7" {
		t.Errorf("version = %q", ver)
	}
	for _, gone := range []string{"node:", "version:", "created:", "updated:"} {
		if strings.Contains(norm, gone) {
			t.Errorf("%q must be dropped:\n%s", gone, norm)
		}
	}
	for _, kept := range []string{"namespace: network", "id: eth0", "phase: running", "address: 10.0.0.1/24"} {
		if !strings.Contains(norm, kept) {
			t.Errorf("%q must stay:\n%s", kept, norm)
		}
	}
	// same document from two nodes with different bookkeeping normalises identically
	a, _ := normalizeResourceYAML(resYAML("10.0.0.2", "7", "10.0.0.1/24"))
	b, _ := normalizeResourceYAML(resYAML("10.0.0.3", "9", "10.0.0.1/24"))
	if a != b {
		t.Errorf("normalised documents differ:\n%s\n---\n%s", a, b)
	}
	c, _ := normalizeResourceYAML(resYAML("10.0.0.3", "9", "10.0.0.9/24"))
	if a == c {
		t.Error("a real spec difference must survive normalisation")
	}
	// unparsable input is returned unchanged
	if got, _ := normalizeResourceYAML(": : ["); got != ": : [" {
		t.Errorf("garbage altered: %q", got)
	}
}

// compareApp: instances pane of Thing00 on node 10.0.0.2 (nodes[1]) with a
// gRPC fake that knows the instance on nodes 1 (base) and 2 (different), and
// not on node 3; node 4 is denied, node 5 errors.
func compareApp(t *testing.T, depth int) (App, *fakeSource) {
	t.Helper()
	fs := newFakeSource("grpc")
	app := browserApp(120, 40, depth, 10)
	app.nodes = makeNodes(5)
	app.nodes[0].Role, app.nodes[1].Role = "controlplane", "controlplane"
	app.source = fs
	app.resSem = make(chan struct{}, 8)
	id := sampleMetas(1)[0].ID
	key := func(ip string) string { return ip + "|" + cmpType + "|" + id }
	fs.yamls[key("10.0.0.1")] = resYAML("10.0.0.1", "3", "10.0.0.1/24") // same as base
	fs.yamls[key("10.0.0.2")] = resYAML("10.0.0.2", "4", "10.0.0.1/24") // base
	fs.yamls[key("10.0.0.3")] = resYAML("10.0.0.3", "5", "10.0.0.7/24") // different
	fs.yerrs[key("10.0.0.4")] = errors.New("rpc error: code = NotFound desc = resource doesn't exist")
	fs.yerrs[key("10.0.0.5")] = errors.New("rpc error: code = PermissionDenied desc = nope")
	return app, fs
}

func loadedCompare(t *testing.T, app App) App {
	t.Helper()
	app = press(t, app, "c")
	top, _ := app.browser.top()
	if top.kind != paneCompare {
		t.Fatalf("c did not open the compare pane (top=%v, status=%q)", top.kind, app.statusMsg)
	}
	return feed(t, app, collectMsgs(app.loadCompare(top.cmp))...)
}

func rowOf(app App, host string) cmpRow {
	top, _ := app.browser.top()
	for _, r := range top.cmp.rows {
		if r.node.Hostname == host {
			return r
		}
	}
	return cmpRow{}
}

func TestCompareRowsPresentAbsentDifferent(t *testing.T) {
	app, fs := compareApp(t, 3)
	app = loadedCompare(t, app)
	top, _ := app.browser.top()
	cv := top.cmp
	if len(cv.rows) != 5 || cv.base != "10.0.0.2" {
		t.Fatalf("rows=%d base=%q", len(cv.rows), cv.base)
	}
	if got := int(fs.yamlCalls.Load()); got != 5 {
		t.Errorf("fetches = %d, want one per node", got)
	}
	want := []struct {
		host, present, same, version string
	}{
		{"talos-node-00.example.internal", "yes", "yes", "3"},
		{"talos-node-01.example.internal", "yes", "base", "4"},
		{"talos-node-02.example.internal", "yes", "no", "5"},
		{"talos-node-03.example.internal", "no", "-", ""},
		{"talos-node-04.example.internal", "lock", "-", ""},
	}
	for _, w := range want {
		r := rowOf(app, w.host)
		if r.presentText() != w.present || cv.sameText(r) != w.same || r.version != w.version {
			t.Errorf("%s: present=%q same=%q version=%q, want %q/%q/%q", w.host, r.presentText(), cv.sameText(r), r.version, w.present, w.same, w.version)
		}
	}
}

func TestCompareErrorRowAndLoadingRows(t *testing.T) {
	app, fs := compareApp(t, 3)
	id := sampleMetas(1)[0].ID
	fs.yerrs["10.0.0.3|"+cmpType+"|"+id] = errors.New("dial tcp: i/o timeout")
	app = press(t, app, "c")
	top, _ := app.browser.top()
	for _, r := range top.cmp.rows {
		if r.presentText() != "…" || top.cmp.sameText(r) != "…" && r.node.IP != top.cmp.base {
			t.Fatalf("before the replies every row is loading: %+v", r)
		}
	}
	app = feed(t, app, collectMsgs(app.loadCompare(top.cmp))...)
	r := rowOf(app, "talos-node-02.example.internal")
	if r.state != cmpError || r.presentText() != "err" || !strings.Contains(r.err, "timeout") {
		t.Fatalf("error row = %+v", r)
	}
}

func TestCompareOpensFromYAMLTypesAndConfig(t *testing.T) {
	// YAML pane
	app, _ := compareApp(t, 4)
	app = press(t, app, "c")
	if top, _ := app.browser.top(); top.kind != paneCompare || top.cmp.subject.id != sampleMetas(1)[0].ID {
		t.Fatalf("from YAML: %+v", top.cmp.subject)
	}

	// types pane: only a type with exactly one instance
	app, _ = compareApp(t, 2)
	app = press(t, app, "c")
	if top, _ := app.browser.top(); top.kind == paneCompare || !strings.Contains(app.statusMsg, "exactly one instance") {
		t.Fatalf("multi-instance type must refuse (top=%v status=%q)", top.kind, app.statusMsg)
	}
	one := sampleMetas(1)[0]
	app.browser = app.browser.setCount(cmpType, 1).setSingle(cmpType, one, true)
	app.browser = app.browser.withTop(func(p *pane) { p.cur = 0 })
	// the first selectable row is a config kind or a resource depending on the fixture: find the type row
	entries := app.browser.typeEntries(testNet, "")
	for i, e := range entries {
		if !e.config && e.def.Type == cmpType {
			app.browser = app.browser.withTop(func(p *pane) { p.cur = i })
		}
	}
	app = press(t, app, "c")
	if top, _ := app.browser.top(); top.kind != paneCompare || top.cmp.subject.id != one.ID {
		t.Fatalf("single-instance type: top=%v status=%q", top.kind, app.statusMsg)
	}

	// config kind with exactly one document (DHCPv4Config)
	app = cfgApp(120, 40, 4)
	app.nodes = makeNodes(3)
	app.browser = app.browser.withTop(func(p *pane) {
		for i, e := range app.browser.typeEntries(testNet, "") {
			if e.config && e.ck.Kind == "DHCPv4Config" {
				p.cur = i
			}
		}
	})
	app = press(t, app, "c")
	top, _ := app.browser.top()
	if top.kind != paneCompare || !top.cmp.subject.cfg || top.cmp.subject.kind != "DHCPv4Config" {
		t.Fatalf("config single: top=%v subj=%+v status=%q", top.kind, top.cmp.subject, app.statusMsg)
	}
	// LinkConfig has two documents: refuse from the types pane
	app = cfgApp(120, 40, 4)
	app.browser = app.browser.withTop(func(p *pane) {
		for i, e := range app.browser.typeEntries(testNet, "") {
			if e.config && e.ck.Kind == "LinkConfig" {
				p.cur = i
			}
		}
	})
	app = press(t, app, "c")
	if top, _ := app.browser.top(); top.kind == paneCompare {
		t.Fatal("two LinkConfig documents: types-pane compare must refuse")
	}
}

func TestCompareConfigDocumentsAcrossNodes(t *testing.T) {
	cfgStream := func(addr string, withLink bool) string {
		s := "version: v1alpha1\n---\nkind: DHCPv4Config\nname: eth0\nclientIdentifier: " + addr + "\n"
		if !withLink {
			s = "version: v1alpha1\n"
		}
		return s
	}
	by := map[string]string{
		"10.0.0.1": cfgStream("mac", true),
		"10.0.0.2": cfgStream("mac", true),
		"10.0.0.3": cfgStream("duid", true),
		"10.0.0.4": cfgStream("", false),
	}
	subj := compareSubject{cfg: true, kind: "DHCPv4Config", name: "eth0", label: "DHCPv4Config/eth0"}
	get := func(_ context.Context, ip string) (string, error) {
		if ip == "10.0.0.5" {
			return "", errors.New("PermissionDenied")
		}
		return by[ip], nil
	}
	m := map[string]compareNodeMsg{}
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"} {
		m[ip] = fetchCompare(context.Background(), nil, get, subj, ip, 1)
	}
	if m["10.0.0.1"].state != cmpPresent || m["10.0.0.1"].yaml != m["10.0.0.2"].yaml {
		t.Errorf("identical docs: %+v vs %+v", m["10.0.0.1"], m["10.0.0.2"])
	}
	if m["10.0.0.3"].yaml == m["10.0.0.1"].yaml || !strings.Contains(m["10.0.0.3"].yaml, "duid") {
		t.Errorf("different doc: %+v", m["10.0.0.3"])
	}
	if m["10.0.0.4"].state != cmpAbsent || m["10.0.0.5"].state != cmpLocked {
		t.Errorf("absent/locked: %v %v", m["10.0.0.4"].state, m["10.0.0.5"].state)
	}
}

func TestCompareEnterOpensDiffAndEscBacksOut(t *testing.T) {
	app, _ := compareApp(t, 3)
	app = loadedCompare(t, app)
	depth := len(app.browser.stack)

	// enter on the base row, an absent row and a denied row does not diff
	for _, i := range []int{1, 3, 4} {
		app.browser = app.browser.withTop(func(p *pane) { p.cur = i })
		app = press(t, app, "enter")
		if top, _ := app.browser.top(); top.kind != paneCompare {
			t.Fatalf("row %d must not open a diff", i)
		}
	}
	// the different node: diff
	app.browser = app.browser.withTop(func(p *pane) { p.cur = 2 })
	app = press(t, app, "enter")
	top, _ := app.browser.top()
	if top.kind != paneDiff || len(app.browser.stack) != depth+1 {
		t.Fatalf("top=%v depth=%d", top.kind, len(app.browser.stack))
	}
	out := render(top.diff)
	for _, want := range []string{
		"--- talos-node-01.example.internal (10.0.0.2)",
		"+++ talos-node-02.example.internal (10.0.0.3)",
		"-    address: 10.0.0.1/24",
		"+    address: 10.0.0.7/24",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("diff missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "version") || strings.Contains(out, "node: ") {
		t.Errorf("the diff must show normalised YAML:\n%s", out)
	}
	// the identical node: diff pane opens and says so
	app = press(t, app, "esc")
	app.browser = app.browser.withTop(func(p *pane) { p.cur = 0 })
	app = press(t, app, "enter")
	top, _ = app.browser.top()
	if top.kind != paneDiff || diffChanged(top.diff) || !strings.Contains(app.statusMsg, "identical") {
		t.Fatalf("identical node: kind=%v changed=%v status=%q", top.kind, diffChanged(top.diff), app.statusMsg)
	}
	// esc backs out one level at a time: diff → compare → instances
	app = press(t, app, "esc")
	if top, _ = app.browser.top(); top.kind != paneCompare {
		t.Fatalf("after esc from diff: %v", top.kind)
	}
	app = press(t, app, "q")
	if top, _ = app.browser.top(); top.kind != paneInstances || len(app.browser.stack) != depth-1 {
		t.Fatalf("after q from compare: %v depth=%d", top.kind, len(app.browser.stack))
	}
}

func TestCompareDropsStaleReplies(t *testing.T) {
	app, _ := compareApp(t, 3)
	app = press(t, app, "c")
	top, _ := app.browser.top()
	app = app.handleCompareNode(compareNodeMsg{seq: top.cmp.seq + 7, ip: "10.0.0.1", state: cmpPresent, yaml: "x: 1\n"})
	if r := rowOf(app, "talos-node-00.example.internal"); r.state != cmpLoading {
		t.Fatalf("a reply of another compare must be dropped: %+v", r)
	}
	// ctrl+r starts a new generation; replies of the old one no longer apply
	old := top.cmp.seq
	app, cmd := app.reloadCompare()
	if cmd == nil {
		t.Fatal("ctrl+r must refetch")
	}
	if top, _ = app.browser.top(); top.cmp.seq == old {
		t.Fatal("reload must bump the generation")
	}
	app = app.handleCompareNode(compareNodeMsg{seq: old, ip: "10.0.0.1", state: cmpPresent})
	if r := rowOf(app, "talos-node-00.example.internal"); r.state != cmpLoading {
		t.Fatalf("stale reply applied after reload: %+v", r)
	}
	// after leaving the browser a late reply is harmless
	app = press(t, app, "esc", "esc", "esc", "esc")
	_ = app.handleCompareNode(compareNodeMsg{seq: old + 1, ip: "10.0.0.1"})
}

func TestCompareNeedsNoSourceWatch(t *testing.T) {
	// compare works with the CLI source too (it only needs GetYAML)
	app, _ := compareApp(t, 3)
	cli := newFakeSource("cli")
	cli.yamls = app.source.(*fakeSource).yamls
	cli.yerrs = app.source.(*fakeSource).yerrs
	app.source = cli
	app = loadedCompare(t, app)
	if r := rowOf(app, "talos-node-02.example.internal"); r.state != cmpPresent {
		t.Fatalf("cli compare: %+v", r)
	}
}

func TestCompareKeyIsNotTheContextSwitcher(t *testing.T) {
	// `x` is global (context switcher) and never reaches the browser table
	for _, kind := range []paneKind{paneTypes, paneInstances, paneYAML} {
		for _, a := range browserActionsFor(kind) {
			for _, k := range a.keys {
				if k == "x" {
					t.Errorf("pane %v binds x, which the global handler takes first", kind)
				}
			}
		}
	}
	var seen = map[string]string{}
	for _, kind := range []paneKind{paneTypes, paneInstances, paneYAML} {
		clear(seen)
		for _, a := range browserActionsFor(kind) {
			for _, k := range a.keys {
				if prev, dup := seen[k]; dup {
					t.Errorf("pane %v: key %q bound twice (%s, %s)", kind, k, prev, a.desc)
				}
				seen[k] = a.desc
			}
		}
	}
}
