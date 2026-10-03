package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

// keyAction is one entry of the browser's k9s-style key table. Hints, the help
// overlay and dispatch all read from browserActionsFor so they cannot drift.
type keyAction struct {
	keys    []string // bubbletea msg.String() values
	label   string   // hint-bar label; defaults to keys joined by "/"
	desc    string
	visible bool // shown in the hint bar
	fn      func(App) (App, tea.Cmd)
}

func (a keyAction) hintLabel() string {
	if a.label != "" {
		return a.label
	}
	return strings.Join(a.keys, "/")
}

const bigMove = 1 << 30

func moveAction(delta int) func(App) (App, tea.Cmd) {
	return func(app App) (App, tea.Cmd) { return app.browserMove(delta), nil }
}

func pageAction(dir int) func(App) (App, tea.Cmd) {
	return func(app App) (App, tea.Cmd) {
		p, ok := app.browser.top()
		if !ok {
			return app, nil
		}
		return app.browserMove(dir * app.paneInnerRows(p.kind)), nil
	}
}

// browserActionsFor returns the key table for a pane kind. It is a pure
// function (no App) so the help overlay can list every table.
func browserActionsFor(kind paneKind) []keyAction {
	if kind == paneRelated {
		return relatedActions()
	}
	if kind == paneNetwork {
		return networkActions()
	}
	as := []keyAction{
		{keys: []string{"up", "k"}, label: "↑↓", desc: "Navigate", visible: true, fn: moveAction(-1)},
		{keys: []string{"down", "j"}, desc: "Move down", fn: moveAction(1)},
		{keys: []string{"g", "home"}, label: "g/G", desc: "Top / bottom", visible: kind == paneYAML, fn: moveAction(-bigMove)},
		{keys: []string{"G", "end"}, desc: "Bottom", fn: moveAction(bigMove)},
		{keys: []string{"ctrl+f", "pgdown"}, desc: "Page down", fn: pageAction(1)},
		{keys: []string{"ctrl+b", "pgup"}, desc: "Page up", fn: pageAction(-1)},
	}
	switch kind {
	case paneCompare:
		as = append(as,
			keyAction{keys: []string{"enter"}, label: "↵", desc: "Diff against the browser's node", visible: true, fn: (App).compareEnter},
		)
	case paneDiff:
		// scrolling only
	case paneAliases:
		as = append(as,
			keyAction{keys: []string{"enter"}, label: "↵", desc: "Jump to type", visible: true, fn: (App).paletteSelect},
		)
	case paneDescribe:
		as = append(as,
			keyAction{keys: []string{"enter"}, label: "↵", desc: "Jump to the selected type", visible: true, fn: (App).describeJump},
			keyAction{keys: []string{"y"}, desc: "YAML", visible: true, fn: (App).describeToYAML},
			keyAction{keys: []string{"d"}, desc: "Back (toggle describe)", visible: true, fn: func(app App) (App, tea.Cmd) { return app.popPane(), nil }},
		)
	case paneYAML:
		as = append(as,
			keyAction{keys: []string{"/"}, desc: "Find", visible: true, fn: func(app App) (App, tea.Cmd) { return app.openPrompt(promptFind) }},
			keyAction{keys: []string{"n"}, label: "n/N", desc: "Next / prev match", visible: true, fn: func(app App) (App, tea.Cmd) { return app.findStep(1), nil }},
			keyAction{keys: []string{"N"}, desc: "Previous match", fn: func(app App) (App, tea.Cmd) { return app.findStep(-1), nil }},
			keyAction{keys: []string{"w"}, desc: "Toggle wrap", visible: true, fn: func(app App) (App, tea.Cmd) { return app.toggleBrowserWrap(), nil }},
			keyAction{keys: []string{"f"}, desc: "Toggle full screen", visible: true, fn: func(app App) (App, tea.Cmd) {
				app.browser.fullscreen = !app.browser.fullscreen
				return app.clampYAMLScroll(), nil
			}},
		)
	default:
		as = append(as,
			keyAction{keys: []string{"enter"}, label: "↵", desc: "Open", visible: true, fn: (App).browserEnter},
			keyAction{keys: []string{"/"}, desc: "Filter", visible: true, fn: func(app App) (App, tea.Cmd) { return app.openPrompt(promptFilter) }},
		)
		if kind == paneInstances {
			as = append(as, keyAction{keys: []string{"y"}, desc: "YAML", visible: true, fn: (App).browserEnter})
		}
		if kind == paneTypes || kind == paneInstances {
			as = append(as, keyAction{keys: []string{"d"}, desc: "Describe", visible: true, fn: (App).openDescribe})
		}
	}
	if kind == paneTypes || kind == paneInstances || kind == paneYAML || kind == paneDescribe {
		as = append(as,
			keyAction{keys: []string{"W"}, desc: "Live watch on/off (gRPC)", visible: kind == paneInstances, fn: (App).watchPress},
		)
	}
	if kind == paneTypes || kind == paneInstances || kind == paneYAML || kind == paneDescribe {
		as = append(as,
			keyAction{keys: []string{"p"}, desc: "Related resources (pipeline and family)", visible: true, fn: (App).openRelated},
		)
	}
	if kind == paneInstances || kind == paneYAML || kind == paneDescribe {
		as = append(as,
			keyAction{keys: []string{"J"}, desc: "Jump to what the writer controller reads", visible: kind != paneDescribe, fn: (App).jumpToWriter},
		)
	}
	if kind == paneTypes || kind == paneInstances || kind == paneYAML {
		as = append(as,
			keyAction{keys: []string{"c"}, desc: "Compare on all nodes", visible: true, fn: (App).comparePress},
		)
	}
	if kind == paneCategories || kind == paneTypes || kind == paneInstances {
		as = append(as, keyAction{keys: []string{"n"}, desc: "Network view (Networking category)", visible: kind != paneCategories, fn: (App).netKey})
	}
	if kind != paneAliases && kind != paneCompare && kind != paneDiff {
		as = append(as,
			keyAction{keys: []string{"ctrl+a"}, label: "^a", desc: "All types (aliases palette)", visible: true, fn: (App).openPalette},
			keyAction{keys: []string{":"}, desc: "Command mode (:nodes :net :addr :q)", visible: true, fn: (App).openCommandPrompt},
		)
	}
	return append(as,
		keyAction{keys: []string{"esc", "q"}, label: "Esc/q", desc: "Back (clears filter first)", visible: true, fn: (App).browserBack},
		keyAction{keys: []string{"ctrl+r"}, label: "^r", desc: "Reload", visible: true, fn: (App).browserReload},
		keyAction{keys: []string{"ctrl+c"}, desc: "Quit", fn: func(app App) (App, tea.Cmd) {
			app.cleanup()
			return app, tea.Quit
		}},
	)
}

func (app App) browserActions() []keyAction {
	p, ok := app.browser.top()
	if !ok {
		return nil
	}
	return browserActionsFor(p.kind)
}

func (app App) handleBrowserKey(msg tea.KeyMsg) (App, tea.Cmd) {
	s := msg.String()
	for _, a := range app.browserActions() {
		for _, k := range a.keys {
			if k == s {
				return a.fn(app)
			}
		}
	}
	if note, ok := app.unboundKeyNote(s); ok {
		app.statusMsg = dimStyle.Render(note)
	}
	return app, nil
}

// browserKey documents a key that only works in some panes, so pressing it
// elsewhere can say where it does work instead of doing nothing.
type browserKey struct {
	key   string
	panes []paneKind // the panes whose table binds it
	what  string     // "an instance list": where it works
	how   string     // how to get there from the types pane
}

// browserKeyTable lists every pane-specific browser key. A test checks it
// against browserActionsFor so it cannot drift from the real bindings.
var browserKeyTable = []browserKey{
	{"d", []paneKind{paneTypes, paneInstances, paneDescribe, paneNetwork}, "a type or instance list, or the network view", "open a category first"},
	{"y", []paneKind{paneInstances, paneDescribe}, "an instance list", "Enter on a type first"},
	{"c", []paneKind{paneTypes, paneInstances, paneYAML, paneRelated, paneNetwork}, "a type, an instance list, a YAML pane, the related table or the network view", "open a category first"},
	{"o", []paneKind{paneNetwork}, "the network view", "press N on the node list, or n in the Networking category"},
	{"W", []paneKind{paneTypes, paneInstances, paneYAML, paneDescribe}, "an instance list (gRPC source)", "Enter on a type first"},
	{"w", []paneKind{paneYAML}, "the YAML pane", "open an instance first"},
	{"f", []paneKind{paneYAML}, "the YAML pane", "open an instance first"},
	{"n", []paneKind{paneYAML, paneCategories, paneTypes, paneInstances}, "the YAML pane (after / search) and the Networking lists (opens the network view)", "open an instance first"},
	{"N", []paneKind{paneYAML}, "the YAML pane, after / search", "open an instance first"},
	{"/", []paneKind{paneCategories, paneTypes, paneInstances, paneYAML}, "a list or the YAML pane", "go back to a list"},
}

// unboundKeyNote explains a browser key pressed in a pane where it is not
// bound. ok is false for keys that are not in the table.
func (app App) unboundKeyNote(s string) (string, bool) {
	p, ok := app.browser.top()
	if !ok {
		return "", false
	}
	for _, k := range browserKeyTable {
		if k.key != s {
			continue
		}
		note := fmt.Sprintf("%s works on %s", k.key, k.what)
		if p.kind == paneTypes {
			if name := app.selectedTypeName(p); name != "" && strings.Contains(k.how, "Enter on a type") {
				return fmt.Sprintf("%s: press Enter on %s first", note, name), true
			}
		}
		return fmt.Sprintf("%s (%s)", note, k.how), true
	}
	return "", false
}

// selectedTypeName is the display name of the types-pane row under the cursor.
func (app App) selectedTypeName(p pane) string {
	rows := app.browser.typeEntries(p.category, p.filter)
	if p.cur >= len(rows) {
		return ""
	}
	return rows[p.cur].name()
}

// keyOnType implements `c` and `W` on a types-pane row: open the instance list
// as Enter would, then say what to press next. ok is false when the key should
// take its normal path (not a resource row, or nothing to open).
func (app App) keyOnType(letter string) (App, tea.Cmd, bool) {
	p, ok := app.browser.top()
	if !ok || p.kind != paneTypes {
		return app, nil, false
	}
	rows := app.browser.typeEntries(p.category, p.filter)
	if p.cur >= len(rows) || rows[p.cur].config {
		return app, nil, false
	}
	d := rows[p.cur].def
	n, counted := app.browser.counts[d.Type]
	if !counted || n < 1 {
		return app, nil, false // openType's own message says why
	}
	if letter == "c" && n == 1 {
		return app, nil, false // compareSubjectFor handles a single instance
	}
	if letter == "W" && !app.hasWatch() {
		return app, nil, false
	}
	depth := len(app.browser.stack)
	app, cmd := app.openType(d)
	if len(app.browser.stack) > depth {
		if n == 1 {
			app.statusMsg = dimStyle.Render("press " + letter + " here")
		} else {
			app.statusMsg = dimStyle.Render("pick one, then press " + letter)
		}
	}
	return app, cmd, true
}

func (app App) comparePress() (App, tea.Cmd) {
	if a, cmd, ok := app.keyOnType("c"); ok {
		return a, cmd
	}
	return app.openCompare()
}

func (app App) watchPress() (App, tea.Cmd) {
	if a, cmd, ok := app.keyOnType("W"); ok {
		return a, cmd
	}
	if p, ok := app.browser.top(); ok && p.kind == paneTypes { // not a resource row with instances
		if !app.hasWatch() {
			app.statusMsg = warnStyle.Render("watch needs the gRPC source")
			return app, nil
		}
		if note, ok := app.unboundKeyNote("W"); ok {
			app.statusMsg = dimStyle.Render(note)
		}
		return app, nil
	}
	return app.toggleWatch()
}

// --- movement ---

func (app App) browserMove(delta int) App {
	p, ok := app.browser.top()
	if !ok {
		return app
	}
	if p.kind == paneDescribe && (delta == 1 || delta == -1) {
		if a, ok := app.describeMove(delta); ok {
			return a
		}
	}
	n := app.paneLen(p)
	rows := app.paneInnerRows(p.kind)
	app.browser = app.browser.withTop(func(p *pane) {
		if p.kind == paneYAML || p.kind == paneDescribe || p.kind == paneDiff {
			p.scroll = clamp(p.scroll+delta, 0, max(0, n-rows))
			return
		}
		p.cur = clamp(p.cur+delta, 0, max(0, n-1))
		if p.kind == paneTypes { // scroll is in visual lines: headers take space
			vis := app.browser.typeVisual(p.category, p.filter)
			if p.cur == 0 {
				p.scroll = 0 // keep the first section header in view
			} else {
				p.scroll = clampScrollStart(p.scroll, visualIndex(vis, p.cur), len(vis), rows)
			}
			return
		}
		p.scroll = clampScrollStart(p.scroll, p.cur, n, rows)
	})
	return app
}

func (app App) clampYAMLScroll() App {
	p, ok := app.browser.top()
	if !ok || p.kind != paneYAML {
		return app
	}
	return app.browserMove(0)
}

func (app App) toggleBrowserWrap() App {
	app.browser.wrap = !app.browser.wrap
	return app.clampYAMLScroll()
}

// --- back / pop ---

func (app App) browserBack() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok {
		return app.exitBrowser(), nil
	}
	if p.kind == paneYAML && app.browser.find != "" {
		app.browser = app.browser.clearFind()
		return app, nil
	}
	if p.kind == paneRelated && len(p.rel.marks) > 0 {
		return app.relClearMarks(), nil
	}
	if p.kind != paneYAML && p.filter != "" {
		app.browser = app.browser.withTop(func(p *pane) { p.filter, p.cur, p.scroll = "", 0, 0 })
		return app, nil
	}
	return app.popPane(), nil
}

func (b browser) clearFind() browser {
	b.find, b.findHits, b.findIdx = "", nil, 0
	return b
}

func (app App) popPane() App {
	if len(app.browser.stack) <= 1 {
		return app.exitBrowser()
	}
	p, _ := app.browser.top()
	app.browser = app.browser.pop()
	if p.kind == paneYAML {
		app.browser.fullscreen = false
		app.browser = app.browser.clearFind()
	}
	app.statusMsg = ""
	return app.syncBrowserState()
}

// exitBrowser returns to the node list; nodeCur is left alone so the node
// stays selected.
func (app App) exitBrowser() App {
	app.browser = browser{}
	app.selNode = nil
	app.statusMsg = ""
	app.state = StateNodeList
	return app
}

// --- prompts (`/` filter, YAML find) ---

func (app App) openPrompt(kind promptKind) (App, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 100
	p, _ := app.browser.top()
	if kind == promptFilter {
		ti.Placeholder = "filter…"
		ti.SetValue(p.filter)
	} else {
		ti.Placeholder = "search…"
		ti.SetValue(app.browser.find)
	}
	ti.CursorEnd()
	app.browser.input = ti
	app.browser.promptPrev = p.filter
	app.browser.prompting = true
	app.browser.promptKind = kind
	return app, app.browser.input.Focus()
}

func (app App) handleBrowserPrompt(msg tea.KeyMsg) (App, tea.Cmd) {
	if top, ok := app.browser.top(); ok && top.kind == paneAliases {
		if a, cmd, handled := app.handlePalettePromptKey(msg); handled {
			return a, cmd
		}
	}
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc":
		app.browser.prompting = false
		app.browser.input.Blur()
		if app.browser.promptKind == promptFilter { // cancel: undo the live filter
			prev := app.browser.promptPrev
			app.browser = app.browser.withTop(func(p *pane) { p.filter, p.cur, p.scroll = prev, 0, 0 })
		}
		return app, nil
	case "enter":
		val := app.browser.input.Value()
		kind := app.browser.promptKind
		app.browser.prompting = false
		app.browser.input.Blur()
		if kind == promptFilter {
			return app, nil // already applied live
		}
		return app.applyFind(val), nil
	case "up", "ctrl+p":
		if app.browser.promptKind == promptFilter {
			return app.browserMove(-1), nil
		}
	case "down", "ctrl+n":
		if app.browser.promptKind == promptFilter {
			return app.browserMove(1), nil
		}
	}
	var cmd tea.Cmd
	app.browser.input, cmd = app.browser.input.Update(msg)
	if app.browser.promptKind == promptFilter { // fuzzy filter follows every keystroke
		val := app.browser.input.Value()
		app.browser = app.browser.withTop(func(p *pane) { p.filter, p.cur, p.scroll = val, 0, 0 })
	}
	return app, cmd
}

// browserFilterText is the active filter/search shown in the status line.
func (app App) browserFilterText() string {
	p, ok := app.browser.top()
	if !ok {
		return ""
	}
	if p.kind == paneYAML {
		return app.browser.find
	}
	return p.filter
}

// --- YAML search ---

func (app App) applyFind(term string) App {
	p, ok := app.browser.top()
	if !ok || p.kind != paneYAML {
		return app
	}
	if term == "" {
		app.browser = app.browser.clearFind()
		return app
	}
	app.browser.find = term
	app.browser.findHits = yamlFindHits(p.yaml, term)
	app.browser.findIdx = 0
	if len(app.browser.findHits) == 0 {
		app.statusMsg = warnStyle.Render("no match for " + term)
		return app
	}
	// start at the first hit at or below the visible top
	vl := yamlVisual(p.yaml, app.yamlInnerWidth(), app.browser.wrap)
	topLogical := 0
	if p.scroll < len(vl) {
		topLogical = vl[p.scroll].logical
	}
	for i, h := range app.browser.findHits {
		if h >= topLogical {
			app.browser.findIdx = i
			break
		}
	}
	return app.scrollToHit()
}

func (app App) findStep(dir int) App {
	n := len(app.browser.findHits)
	if n == 0 {
		return app
	}
	app.browser.findIdx = ((app.browser.findIdx+dir)%n + n) % n
	return app.scrollToHit()
}

func (app App) scrollToHit() App {
	p, ok := app.browser.top()
	if !ok || len(app.browser.findHits) == 0 {
		return app
	}
	hit := app.browser.findHits[app.browser.findIdx]
	vl := yamlVisual(p.yaml, app.yamlInnerWidth(), app.browser.wrap)
	rows := app.paneInnerRows(paneYAML)
	target := 0
	for i, l := range vl {
		if l.logical == hit {
			target = i
			break
		}
	}
	app.browser = app.browser.withTop(func(p *pane) {
		p.scroll = clamp(target, 0, max(0, len(vl)-rows))
	})
	app.statusMsg = infoStyle.Render(fmt.Sprintf("match %d/%d", app.browser.findIdx+1, len(app.browser.findHits)))
	return app
}

// --- enter / open ---

func (app App) openBrowser(n talos.Node) (App, tea.Cmd) {
	app.selNode = &n
	app.prev = StateNodeList
	app.browser = browser{
		node:  n,
		stack: []pane{{kind: paneCategories, title: "Categories"}},
	}
	app.statusMsg = ""
	app = app.syncBrowserState()
	app, _ = app.useCachedConfig()
	if defs, ok := app.resourceDefs[n.IP]; ok {
		app.browser.defs = defs
		app = app.showTip()
		return app.countAllTypes()
	}
	app.browser.defsLoading = true
	app.statusMsg = "Loading resource definitions..."
	return app, app.loadResourceDefs()
}

func (app App) browserEnter() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok {
		return app, nil
	}
	b := app.browser
	switch p.kind {
	case paneCategories:
		rows := b.categoryRows(p.filter)
		if p.cur >= len(rows) {
			return app, nil
		}
		row := rows[p.cur]
		types := b.typesIn(row.key)
		app.browser = b.push(pane{kind: paneTypes, title: row.label, category: row.key})
		cmd := app.loadCounts(types) // skips types already counted or in flight
		app.browser = app.browser.markCountsLoading(types)
		app = app.syncBrowserState().showTip()
		var cfgCmd tea.Cmd
		app, cfgCmd = app.ensureConfig()
		return app, tea.Batch(cmd, cfgCmd)

	case paneTypes:
		rows := b.typeEntries(p.category, p.filter)
		if p.cur >= len(rows) {
			return app, nil
		}
		if rows[p.cur].config {
			return app.openConfigKind(rows[p.cur].ck)
		}
		return app.openType(rows[p.cur].def)

	case paneInstances:
		items := filterInstances(p.items, p.filter)
		if p.cur >= len(items) {
			return app, nil
		}
		if p.cfgKind != "" {
			return app.openConfigYAML(p.cfgKind, items[p.cur].ID)
		}
		return app.openYAML(p.def, items[p.cur])
	}
	return app, nil
}

// openType implements Enter on a type row.
func (app App) openType(d talos.ResourceDef) (App, tea.Cmd) {
	b := app.browser
	n, counted := b.counts[d.Type]
	switch {
	case !counted:
		app.statusMsg = dimStyle.Render("still counting " + d.DisplayType + "…")
		return app, nil
	case n == countLocked:
		app.statusMsg = warnStyle.Render("requires os:admin")
		return app, nil
	case n == countError:
		app.statusMsg = errStyle.Render("could not list " + d.DisplayType)
		return app, nil
	case n == 0:
		app.statusMsg = dimStyle.Render("no " + d.DisplayType + " on this node")
		return app, nil
	}
	if only, ok := b.singles[d.Type]; ok && n == 1 {
		// One instance: push the one-row list and the YAML together so Esc
		// from YAML lands on a list, same depth as the multi-instance path.
		app.browser = b.push(pane{kind: paneInstances, title: d.DisplayType, def: d, items: []talos.ResourceMeta{only}})
		return app.openYAML(d, only)
	}
	app.browser = b.push(pane{kind: paneInstances, title: d.DisplayType, def: d, loading: true})
	app = app.syncBrowserState()
	return app, app.loadInstances(d)
}

func (app App) openYAML(d talos.ResourceDef, m talos.ResourceMeta) (App, tea.Cmd) {
	app.browser = app.browser.clearFind().push(pane{kind: paneYAML, title: m.ID, def: d, meta: m, loading: true})
	app = app.syncBrowserState()
	return app, app.loadYAML(d, m)
}

// --- reload ---

func (app App) browserReload() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok {
		return app, nil
	}
	b := app.browser
	switch p.kind {
	case paneCategories:
		defs := make(map[string][]talos.ResourceDef, len(app.resourceDefs))
		for k, v := range app.resourceDefs {
			if k != b.node.IP {
				defs[k] = v
			}
		}
		app.resourceDefs = defs
		b.defs, b.counts, b.singles, b.loading = nil, nil, nil, nil
		b.defsLoading, b.defsErr = true, ""
		b.docs, b.cfgState, b.cfgErr = nil, cfgNone, ""
		app.browser = b
		app = app.dropConfigCache()
		app.statusMsg = "Reloading resource definitions..."
		return app, app.loadResourceDefs()

	case paneTypes:
		types := b.typesIn(p.category)
		counts := make(map[string]int, len(b.counts))
		for k, v := range b.counts {
			counts[k] = v
		}
		for _, d := range types {
			delete(counts, d.Type)
			b = b.setSingle(d.Type, talos.ResourceMeta{}, false)
		}
		b.counts = counts
		loading := make(map[string]bool, len(b.loading))
		for k, v := range b.loading {
			loading[k] = v
		}
		for _, d := range types {
			delete(loading, d.Type)
		}
		b.loading = loading
		app.browser = b
		cmd := app.loadCounts(types) // skips types already counted or in flight
		app.browser = app.browser.markCountsLoading(types)
		return app.reloadConfig(cmd)

	case paneInstances:
		if p.cfgKind != "" {
			return app.reloadConfig(nil)
		}
		app.stopWatch() // syncWatch restarts it, bootstrapping afresh
		app.browser = b.withTop(func(p *pane) { p.loading, p.err = true, "" })
		return app, app.loadInstances(p.def)

	case paneDescribe:
		return app.reloadDeps()
	case paneDiff:
		return app, nil
	case paneRelated:
		return app.reloadRelated()
	case paneNetwork:
		return app.reloadNetwork()
	case paneCompare:
		return app.reloadCompare()
	case paneAliases:
		return app.paletteCounts()
	case paneYAML:
		if p.cfgKind != "" {
			return app.reloadConfig(nil)
		}
		app.stopWatch()
		app.browser = b.withTop(func(p *pane) { p.loading, p.err = true, "" })
		return app, app.loadYAML(p.def, p.meta)
	}
	return app, nil
}

// dropConfigCache forgets the current node's cached config documents.
func (app App) dropConfigCache() App {
	cache := make(map[string]cfgCacheEntry, len(app.configDocs))
	for k, v := range app.configDocs {
		if k != app.browser.node.IP {
			cache[k] = v
		}
	}
	app.configDocs = cache
	return app
}

// reloadConfig refetches the node's config documents; extra is batched in.
func (app App) reloadConfig(extra tea.Cmd) (App, tea.Cmd) {
	app = app.dropConfigCache()
	app.browser.cfgState, app.browser.cfgErr = cfgLoading, ""
	return app, tea.Batch(extra, app.loadConfigDocs())
}

// relatedActions is the key table of the related view.
func relatedActions() []keyAction {
	dir := func(dx, dy int) func(App) (App, tea.Cmd) {
		return func(app App) (App, tea.Cmd) { return app.relDir(dx, dy) }
	}
	return []keyAction{
		{keys: []string{"left", "h"}, label: "←→", desc: "Move left", visible: true, fn: dir(-1, 0)},
		{keys: []string{"right", "l"}, desc: "Move right", fn: dir(1, 0)},
		{keys: []string{"up", "k"}, label: "↑↓", desc: "Move up", visible: true, fn: dir(0, -1)},
		{keys: []string{"down", "j"}, desc: "Move down", fn: dir(0, 1)},
		{keys: []string{"g", "home"}, desc: "Table: first row", fn: func(app App) (App, tea.Cmd) { return app.relEdge(true) }},
		{keys: []string{"G", "end"}, desc: "Table: last row", fn: func(app App) (App, tea.Cmd) { return app.relEdge(false) }},
		{keys: []string{"tab"}, desc: "Switch between pipeline and table", visible: true, fn: (App).relFocusSwitch},
		{keys: []string{"enter"}, label: "↵", desc: "Pipeline: open the type · table: open the YAML", visible: true, fn: (App).relEnter},
		{keys: []string{" ", "space"}, label: "space", desc: "Table: mark a cell (two at most)", visible: true, fn: (App).relMarkToggle},
		{keys: []string{"c"}, desc: "Table: diff the two marked cells", visible: true, fn: (App).relDiff},
		{keys: []string{"ctrl+a"}, label: "^a", desc: "All types (aliases palette)", visible: true, fn: (App).openPalette},
		{keys: []string{":"}, desc: "Command mode (:nodes :net :addr :q)", fn: (App).openCommandPrompt},
		{keys: []string{"esc", "q"}, label: "Esc/q", desc: "Back (clears marks first)", visible: true, fn: (App).browserBack},
		{keys: []string{"ctrl+r"}, label: "^r", desc: "Reload", visible: true, fn: (App).browserReload},
		{keys: []string{"ctrl+c"}, desc: "Quit", fn: func(app App) (App, tea.Cmd) {
			app.cleanup()
			return app, tea.Quit
		}},
	}
}
