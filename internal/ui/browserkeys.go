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
	as := []keyAction{
		{keys: []string{"up", "k"}, label: "↑↓", desc: "Navigate", visible: true, fn: moveAction(-1)},
		{keys: []string{"down", "j"}, desc: "Move down", fn: moveAction(1)},
		{keys: []string{"g", "home"}, label: "g/G", desc: "Top / bottom", visible: kind == paneYAML, fn: moveAction(-bigMove)},
		{keys: []string{"G", "end"}, desc: "Bottom", fn: moveAction(bigMove)},
		{keys: []string{"ctrl+f", "pgdown"}, desc: "Page down", fn: pageAction(1)},
		{keys: []string{"ctrl+b", "pgup"}, desc: "Page up", fn: pageAction(-1)},
	}
	switch kind {
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
	return app, nil
}

// --- movement ---

func (app App) browserMove(delta int) App {
	p, ok := app.browser.top()
	if !ok {
		return app
	}
	n := app.paneLen(p)
	rows := app.paneInnerRows(p.kind)
	app.browser = app.browser.withTop(func(p *pane) {
		if p.kind == paneYAML {
			p.scroll = clamp(p.scroll+delta, 0, max(0, n-rows))
			return
		}
		p.cur = clamp(p.cur+delta, 0, max(0, n-1))
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
	app.browser.prompting = true
	app.browser.promptKind = kind
	return app, app.browser.input.Focus()
}

func (app App) handleBrowserPrompt(msg tea.KeyMsg) (App, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc":
		app.browser.prompting = false
		app.browser.input.Blur()
		return app, nil
	case "enter":
		val := app.browser.input.Value()
		kind := app.browser.promptKind
		app.browser.prompting = false
		app.browser.input.Blur()
		if kind == promptFilter {
			app.browser = app.browser.withTop(func(p *pane) { p.filter, p.cur, p.scroll = val, 0, 0 })
			return app, nil
		}
		return app.applyFind(val), nil
	}
	var cmd tea.Cmd
	app.browser.input, cmd = app.browser.input.Update(msg)
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
	if defs, ok := app.resourceDefs[n.IP]; ok {
		app.browser.defs = defs
		return app, nil
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
		app.browser = b.push(pane{kind: paneTypes, title: row.label, category: row.key})
		app = app.syncBrowserState()
		return app, app.loadCounts(b.typesIn(row.key))

	case paneTypes:
		rows := b.typeRows(p.category, p.filter)
		if p.cur >= len(rows) {
			return app, nil
		}
		return app.openType(rows[p.cur])

	case paneInstances:
		items := filterInstances(p.items, p.filter)
		if p.cur >= len(items) {
			return app, nil
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
		app.browser = b
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
		app.browser = b
		return app, app.loadCounts(types)

	case paneInstances:
		app.browser = b.withTop(func(p *pane) { p.loading, p.err = true, "" })
		return app, app.loadInstances(p.def)

	case paneYAML:
		app.browser = b.withTop(func(p *pane) { p.loading, p.err = true, "" })
		return app, app.loadYAML(p.def, p.meta)
	}
	return app, nil
}
