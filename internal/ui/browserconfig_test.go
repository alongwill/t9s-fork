package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/florianspk/t9s/internal/talos"
)

var testConfigDocs = []talos.ConfigDoc{
	{Kind: "v1alpha1", Name: "Config", YAML: "version: v1alpha1\n"},
	{Kind: "LinkConfig", Name: "eth0", YAML: "kind: LinkConfig\nname: eth0\n"},
	{Kind: "LinkConfig", Name: "eth1", YAML: "kind: LinkConfig\nname: eth1\n"},
	{Kind: "DHCPv4Config", Name: "", YAML: "kind: DHCPv4Config\n"},
}

// cfgApp is browserApp at the types pane with the machine config loaded.
func cfgApp(width, height, nTypes int) App {
	app := browserApp(width, height, 2, nTypes)
	app.browser.docs = testConfigDocs
	app.browser.cfgState = cfgLoaded
	return app
}

func plainLines(s string) []string {
	return strings.Split(s, "\n")
}

func TestConfigSectionsRenderConfigFirstThenResources(t *testing.T) {
	app := cfgApp(120, 40, 4)
	out := app.renderBrowser(app.mainHeight())
	iCfg, iRes := strings.Index(out, "CONFIG"), strings.Index(out, "RESOURCES")
	if iCfg < 0 || iRes < 0 || iCfg > iRes {
		t.Fatalf("want CONFIG before RESOURCES:\n%s", out)
	}
	// present kinds come before absent ones: DHCPv4Config, LinkConfig, then BondConfig
	d, l, bnd := strings.Index(out, "DHCPv4Config"), strings.Index(out, "LinkConfig"), strings.Index(out, "BondConfig")
	if d < 0 || !(d < l && l < bnd && bnd < iRes) {
		t.Errorf("config order wrong: DHCPv4Config=%d LinkConfig=%d BondConfig=%d RESOURCES=%d", d, l, bnd, iRes)
	}
	// v1.14 kind is hidden on a v1.13 node
	if strings.Contains(out, "BGPInstanceConfig") {
		t.Error("BGPInstanceConfig should be hidden on v1.13")
	}
}

func TestConfigCountColumnIsDocumentCount(t *testing.T) {
	app := cfgApp(120, 60, 2)
	out := app.renderBrowser(app.mainHeight())
	for _, l := range plainLines(out) {
		if (strings.Contains(l, "  LinkConfig ") || strings.Contains(l, "▶ LinkConfig ")) && !strings.Contains(l, "2") {
			t.Errorf("LinkConfig row should show 2: %q", l)
		}
	}
}

func TestConfigAbsentKindIsGreyed(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)

	app := cfgApp(120, 60, 2)
	app = press(t, app, "j", "j") // off the cursor row, onto LinkConfig; DHCPv4Config is unselected
	out := app.renderBrowser(app.mainHeight())
	find := func(name string) string {
		for _, l := range plainLines(out) {
			if strings.Contains(l, name) {
				return l
			}
		}
		t.Fatalf("%s not rendered", name)
		return ""
	}
	esc := func(l string) int { return strings.Count(l, "\x1b[") }
	if esc(find("BondConfig")) <= esc(find("DHCPv4Config")) {
		t.Errorf("absent BondConfig should carry more style escapes than present DHCPv4Config")
	}
}

func TestConfigPermissionDeniedShowsNoteAndKeepsResources(t *testing.T) {
	app := cfgApp(120, 40, 3)
	app.browser.docs, app.browser.cfgState = nil, cfgDenied
	out := app.renderBrowser(app.mainHeight())
	for _, want := range []string{"CONFIG", "requires os:admin", "RESOURCES", "Thing00"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "LinkConfig") {
		t.Error("config kinds must not be listed when the config is unreadable")
	}
	// the note row is not selectable: first selectable row is the first resource
	app = press(t, app, "enter")
	if p, _ := app.browser.top(); p.kind != paneInstances || p.title != "Thing00" {
		// Thing00 has count 1 but no recorded single → instances pane
		t.Errorf("enter on first row opened %+v", p)
	}
}

func TestConfigCursorSkipsHeaders(t *testing.T) {
	app := cfgApp(120, 60, 3)
	b := app.browser
	n := len(b.typeEntries(testNet, ""))
	vis := b.typeVisual(testNet, "")
	nCfg := len(b.configEntries(testNet, ""))
	if len(vis) != n+2 {
		t.Fatalf("visual rows = %d, want entries+2 headers = %d", len(vis), n+2)
	}
	// moving down across the section boundary lands on selectable rows only
	for i := 0; i < nCfg; i++ {
		app = press(t, app, "j")
	}
	p, _ := app.browser.top()
	if p.cur != nCfg {
		t.Fatalf("cur = %d, want first resource row %d", p.cur, nCfg)
	}
	e := app.browser.typeEntries(testNet, "")[p.cur]
	if e.config || e.def.DisplayType != "Thing00" {
		t.Errorf("cursor on %+v, want Thing00", e)
	}
	// G then g round-trips over the headers
	app = press(t, app, "G")
	if p, _ = app.browser.top(); p.cur != n-1 {
		t.Errorf("G cur = %d, want %d", p.cur, n-1)
	}
	app = press(t, app, "g")
	if p, _ = app.browser.top(); p.cur != 0 || p.scroll != 0 {
		t.Errorf("g cur=%d scroll=%d", p.cur, p.scroll)
	}
}

func TestConfigEnterZeroDocsSetsStatusAndDoesNotPush(t *testing.T) {
	app := cfgApp(120, 60, 2)
	// BondConfig is absent; move to it (after DHCPv4Config, LinkConfig)
	entries := app.browser.typeEntries(testNet, "")
	for i, e := range entries {
		if e.config && e.ck.Kind == "BondConfig" {
			app.browser = app.browser.withTop(func(p *pane) { p.cur = i })
		}
	}
	app = press(t, app, "enter")
	if len(app.browser.stack) != 2 || !strings.Contains(app.statusMsg, "no BondConfig in this node's config") {
		t.Errorf("stack=%d status=%q", len(app.browser.stack), app.statusMsg)
	}
}

func TestConfigEnterSingleDocPushesInstancesAndYAML(t *testing.T) {
	app := cfgApp(120, 60, 2)
	app = press(t, app, "enter") // first row: DHCPv4Config (1 doc, unnamed)
	if got := len(app.browser.stack); got != 4 {
		t.Fatalf("stack = %d, want 4 (categories, types, instances, yaml)", got)
	}
	top, _ := app.browser.top()
	if top.kind != paneYAML || top.yaml != "kind: DHCPv4Config\n" || top.meta.ID != "#1" {
		t.Errorf("yaml pane = %+v", top)
	}
	if got := app.breadcrumb(); !strings.HasSuffix(got, "Networking > DHCPv4Config > #1") {
		t.Errorf("breadcrumb = %q", got)
	}
	app = press(t, app, "esc")
	if p, _ := app.browser.top(); p.kind != paneInstances || len(p.items) != 1 || p.cfgKind != "DHCPv4Config" {
		t.Errorf("esc landed on %+v", p)
	}
	app = press(t, app, "esc", "esc")
	if app.state != StateCategories {
		t.Errorf("state = %v after popping to categories", app.state)
	}
}

func TestConfigEnterSeveralDocsListsNamesThenYAMLIsOneDocument(t *testing.T) {
	app := cfgApp(120, 60, 2)
	app = press(t, app, "j", "enter") // LinkConfig: eth0, eth1
	p, _ := app.browser.top()
	if len(app.browser.stack) != 3 || p.kind != paneInstances || len(p.items) != 2 || p.items[0].ID != "eth0" {
		t.Fatalf("stack=%d top=%+v", len(app.browser.stack), p)
	}
	out := app.renderBrowser(app.mainHeight())
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "eth1") || strings.Contains(out, "NAMESPACE") {
		t.Errorf("config instances should list NAME only:\n%s", out)
	}
	app = press(t, app, "j", "enter")
	top, _ := app.browser.top()
	if top.kind != paneYAML || top.yaml != "kind: LinkConfig\nname: eth1\n" {
		t.Errorf("YAML must be the one document, got %q", top.yaml)
	}
	// Esc depth: yaml → instances → types, same as resources
	app = press(t, app, "esc", "esc")
	if p, _ = app.browser.top(); p.kind != paneTypes {
		t.Errorf("top after two esc = %v", p.kind)
	}
}

func TestConfigFilterMatchesAcrossSectionsAndHidesEmptyHeaders(t *testing.T) {
	app := cfgApp(120, 60, 4)
	app = press(t, app, "/", "t", "h", "i", "n", "g", "0", "1")
	vis := app.browser.typeVisual(testNet, "thing01")
	var headers []string
	for _, v := range vis {
		if v.header != "" {
			headers = append(headers, v.header)
		}
	}
	if len(headers) != 1 || headers[0] != "RESOURCES" {
		t.Errorf("headers with only resource matches = %v", headers)
	}
	app.browser = app.browser.withTop(func(p *pane) { p.filter = "dhcp" })
	var cfgOnly []string
	for _, v := range app.browser.typeVisual(testNet, "dhcp") {
		if v.header != "" {
			cfgOnly = append(cfgOnly, v.header)
		}
	}
	if len(cfgOnly) != 1 || cfgOnly[0] != "CONFIG" {
		t.Errorf("headers with only config matches = %v", cfgOnly)
	}
	// "link" matches LinkConfig and nothing in resources
	if es := app.browser.typeEntries(testNet, "link"); len(es) == 0 || !es[0].config || es[0].ck.Kind != "LinkConfig" {
		t.Errorf("link entries = %+v", es)
	}
}

func TestConfigCategoryCountsSpanBothSections(t *testing.T) {
	app := cfgApp(120, 40, 4)
	var row catRow
	for _, r := range app.browser.categoryRows("") {
		if r.key == testNet {
			row = r
		}
	}
	wantKnown := len(app.browser.typesIn(testNet)) + len(app.browser.configKindsIn(testNet))
	// present: 2 resource types with count>0 (Thing00, Thing02) + LinkConfig + DHCPv4Config
	if row.known != wantKnown || row.present != 4 || !row.counted {
		t.Errorf("row = %+v, want known=%d present=4 counted", row, wantKnown)
	}
	app.browser.cfgState = cfgLoading
	for _, r := range app.browser.categoryRows("") {
		if r.key == testNet && r.counted {
			t.Error("category must stay uncounted while the config loads")
		}
	}
}

func TestConfigLoadedOnCategoryEnterAndCached(t *testing.T) {
	app := browserApp(120, 40, 4, 4)
	app.browser.stack = app.browser.stack[:1]
	app.browser.cfgState = cfgNone
	app = app.syncBrowserState()
	app, cmd := app.browserEnter()
	if cmd == nil || app.browser.cfgState != cfgLoading {
		t.Fatalf("entering a category should start the config load: state=%v", app.browser.cfgState)
	}
	app, _ = app.Update2(configDocsMsg{node: "10.9.9.9", docs: testConfigDocs})
	if app.browser.cfgState != cfgLoading {
		t.Error("stale node reply applied")
	}
	old := app.configDocs
	app, _ = app.Update2(configDocsMsg{node: app.browser.node.IP, docs: testConfigDocs})
	if app.browser.cfgState != cfgLoaded || len(app.browser.docs) != 4 {
		t.Fatalf("not loaded: %+v", app.browser.cfgState)
	}
	if len(old) != 0 || len(app.configDocs[app.browser.node.IP].docs) != 4 {
		t.Error("cache not filled copy-on-write")
	}
	// back out and re-enter: served from cache, no new command
	app = press(t, app, "esc", "esc")
	app, _ = app.openBrowser(app.nodes[1])
	if app.browser.cfgState != cfgLoaded {
		t.Errorf("openBrowser should use the cache, state=%v", app.browser.cfgState)
	}
	_, cmd = app.ensureConfig()
	if cmd != nil {
		t.Error("ensureConfig refetched a cached config")
	}
}

func TestConfigPermissionDeniedReply(t *testing.T) {
	app := browserApp(120, 40, 2, 3)
	app.browser.cfgState = cfgLoading
	app, _ = app.Update2(configDocsMsg{node: app.browser.node.IP, denied: true, err: errTestDenied})
	if app.browser.cfgState != cfgDenied {
		t.Errorf("state = %v, want cfgDenied", app.browser.cfgState)
	}
	if !app.configDocs[app.browser.node.IP].denied {
		t.Error("denial should be cached")
	}
}

func TestConfigReloadClearsCacheAndRefetches(t *testing.T) {
	app := cfgApp(120, 40, 3)
	app.configDocs = map[string]cfgCacheEntry{app.browser.node.IP: {docs: testConfigDocs}, "other": {}}
	app = press(t, app, "enter") // DHCPv4Config → YAML
	app, cmd := app.browserReload()
	if cmd == nil || app.browser.cfgState != cfgLoading {
		t.Fatalf("reload on a config pane should refetch: cmd=%v state=%v", cmd != nil, app.browser.cfgState)
	}
	if _, ok := app.configDocs[app.browser.node.IP]; ok {
		t.Error("node cache survived reload")
	}
	if _, ok := app.configDocs["other"]; !ok {
		t.Error("reload dropped another node's cache")
	}
	// new content arrives: the open YAML pane is refreshed
	changed := []talos.ConfigDoc{{Kind: "DHCPv4Config", YAML: "kind: DHCPv4Config\nchanged: true\n"}}
	app, _ = app.Update2(configDocsMsg{node: app.browser.node.IP, docs: changed})
	if top, _ := app.browser.top(); !strings.Contains(top.yaml, "changed: true") {
		t.Errorf("open YAML not refreshed: %q", top.yaml)
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

const errTestDenied = testErr("rpc error: code = PermissionDenied desc = not authorized")
