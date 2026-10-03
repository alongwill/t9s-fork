package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/catalog"
	"github.com/florianspk/t9s/internal/talos"
)

// Palette (k9s aliases view, ctrl+a): every config kind and resource type of
// the node in one filterable list. It is a pane on the browser stack, so
// closing it restores the previous stack untouched.

type paletteEntry struct {
	name    string // DisplayType or config Kind
	typ     string // full resource type, or the config Kind
	catKey  string
	aliases []string
	config  bool
	ck      catalog.ConfigKind
	def     talos.ResourceDef
}

func (e paletteEntry) kindLabel() string {
	if e.config {
		return "config"
	}
	return "resource"
}

// exact reports a case-insensitive whole-word match on the name, the full
// type or an alias (k9s: `svc` is exact, not fuzzy).
func (e paletteEntry) exact(term string) bool {
	if term == "" {
		return false
	}
	if strings.EqualFold(e.name, term) || strings.EqualFold(e.typ, term) {
		return true
	}
	for _, a := range e.aliases {
		if strings.EqualFold(a, term) {
			return true
		}
	}
	return false
}

func (e paletteEntry) fields() []string {
	f := append([]string{e.name, e.typ}, e.aliases...)
	return append(f, catalog.Label(e.catKey))
}

// configCategory is the category key a config kind is listed under.
func configCategory(ck catalog.ConfigKind) string { return catalog.ConfigCategoryFor(ck.Group) }

// paletteEntries lists everything the node knows, by category: config kinds
// first, then resource types. Config kinds follow the types pane, so they are
// absent while the machine config is unreadable.
func (b browser) paletteEntries() []paletteEntry {
	var out []paletteEntry
	for _, c := range catalog.Categories {
		kinds := b.configKindsIn(c.Key)
		sort.SliceStable(kinds, func(i, j int) bool {
			return strings.ToLower(kinds[i].Kind) < strings.ToLower(kinds[j].Kind)
		})
		for _, k := range kinds {
			out = append(out, paletteEntry{name: k.Kind, typ: k.Kind, catKey: c.Key, config: true, ck: k})
		}
		for _, d := range b.typesIn(c.Key) {
			out = append(out, paletteEntry{name: d.DisplayType, typ: d.Type, catKey: c.Key, aliases: d.Aliases, def: d})
		}
	}
	return out
}

// paletteRows filters the palette: exact name/alias matches first, then the
// fuzzy ranking of the rest.
func (b browser) paletteRows(filter string) []paletteEntry {
	all := b.paletteEntries()
	if filter == "" || strings.HasPrefix(filter, "!") {
		return rankFilter(all, filter, paletteEntry.fields)
	}
	var exact, rest []paletteEntry
	for _, e := range all {
		if e.exact(filter) {
			exact = append(exact, e)
		} else {
			rest = append(rest, e)
		}
	}
	return append(exact, rankFilter(rest, filter, paletteEntry.fields)...)
}

// lookupExact finds the first entry whose name, full type, alias or config
// kind equals term (case-insensitive).
func (b browser) lookupExact(term string) (paletteEntry, bool) {
	for _, e := range b.paletteEntries() {
		if e.exact(term) {
			return e, true
		}
	}
	return paletteEntry{}, false
}

func (b browser) paletteCell(e paletteEntry) (string, bool) {
	if e.config {
		return b.configCell(e.ck.Kind)
	}
	return b.typeCell(e.def)
}

// --- opening / closing / jumping ---

func (app App) hasPalette() bool {
	for _, p := range app.browser.stack {
		if p.kind == paneAliases {
			return true
		}
	}
	return false
}

// openPalette pushes the palette with its filter prompt open and starts
// counting every type not yet counted.
func (app App) openPalette() (App, tea.Cmd) {
	if top, ok := app.browser.top(); !ok || top.kind == paneAliases {
		return app, nil
	}
	app.browser = app.browser.push(pane{kind: paneAliases, title: "Aliases"})
	app = app.syncBrowserState()
	app, focus := app.openPrompt(promptFilter)
	app, cmds := app.paletteCounts()
	return app, tea.Batch(focus, cmds)
}

// paletteCounts starts the background counts (semaphore-capped) and the
// machine-config load. Safe to call again: already counted/in-flight types are
// skipped.
func (app App) paletteCounts() (App, tea.Cmd) {
	cmd := app.loadCounts(app.browser.defs)
	app.browser = app.browser.markCountsLoading(app.browser.defs)
	app, cfgCmd := app.ensureConfig()
	return app, tea.Batch(cmd, cfgCmd)
}

// closePalette pops the palette and its prompt.
func (app App) closePalette() App {
	app.browser.prompting = false
	app.browser.input.Blur()
	return app.popPane()
}

// paletteSelect implements Enter in the palette.
func (app App) paletteSelect() (App, tea.Cmd) {
	p, _ := app.browser.top()
	rows := app.browser.paletteRows(p.filter)
	if p.cur >= len(rows) {
		return app, nil
	}
	app.browser.prompting = false
	app.browser.input.Blur()
	return app.jumpTo(rows[p.cur])
}

// jumpTo replaces the stack with [categories, types(category)], cursors on the
// entry, and presses Enter on it. Esc afterwards lands in its category.
func (app App) jumpTo(e paletteEntry) (App, tea.Cmd) {
	b := app.browser
	cats := b.categoryRows("")
	catIdx := 0
	for i, r := range cats {
		if r.key == e.catKey {
			catIdx = i
		}
	}
	ents := b.typeEntries(e.catKey, "")
	idx := 0
	for i, t := range ents {
		if t.config == e.config && t.name() == e.name {
			idx = i
		}
	}
	vis := b.typeVisual(e.catKey, "")
	typeScroll := 0
	if idx > 0 {
		typeScroll = clampScrollStart(0, visualIndex(vis, idx), len(vis), app.paneInnerRows(paneTypes))
	}
	b.stack = []pane{
		{kind: paneCategories, title: "Categories", cur: catIdx,
			scroll: clampScrollStart(0, catIdx, len(cats), app.paneInnerRows(paneCategories))},
		{kind: paneTypes, title: catalog.Label(e.catKey), category: e.catKey, cur: idx, scroll: typeScroll},
	}
	b = b.clearFind()
	b.fullscreen, b.prompting = false, false
	app.browser = b
	app.statusMsg = ""
	app = app.syncBrowserState()

	cmd := app.loadCounts(b.typesIn(e.catKey))
	app.browser = app.browser.markCountsLoading(b.typesIn(e.catKey))
	app, cfgCmd := app.ensureConfig()
	app, enterCmd := app.browserEnter()
	return app, tea.Batch(cmd, cfgCmd, enterCmd)
}

// --- prompt handling for the palette ---

// handlePalettePromptKey handles the keys the palette prompt owns; handled is
// false for everything else (typing, arrows, ctrl+c go the generic way).
func (app App) handlePalettePromptKey(msg tea.KeyMsg) (App, tea.Cmd, bool) {
	switch msg.String() {
	case "esc":
		if p, _ := app.browser.top(); p.filter != "" {
			app.browser.input.SetValue("")
			app.browser = app.browser.withTop(func(p *pane) { p.filter, p.cur, p.scroll = "", 0, 0 })
			return app, nil, true
		}
		return app.closePalette(), nil, true
	case "enter":
		a, cmd := app.paletteSelect()
		return a, cmd, true
	}
	return app, nil, false
}

// --- rendering ---

func (app App) paletteLines(p pane, iw, inner int) []string {
	b := app.browser
	rows := b.paletteRows(p.filter)
	avail := max(0, iw-2)
	const countW = 6
	kindW, catW, aliasW := 8, 16, 18
	showKind, showCat, showAlias := true, true, true
	width := func() int {
		n := countW + 1
		if showKind {
			n += kindW + 1
		}
		if showCat {
			n += catW + 1
		}
		if showAlias {
			n += aliasW + 1
		}
		return n
	}
	for _, drop := range []*bool{&showCat, &showAlias, &showKind} {
		if avail-width() >= 20 {
			break
		}
		*drop = false
	}
	nameW := max(1, avail-width())

	header := "  " + fit("NAME", nameW)
	if showAlias {
		header += " " + fit("ALIASES", aliasW)
	}
	if showCat {
		header += " " + fit("CATEGORY", catW)
	}
	if showKind {
		header += " " + fit("KIND", kindW)
	}
	header += " " + padLeft("COUNT", countW)
	out := []string{colHeaderStyle.Render(fit(header, iw))}
	body := max(0, inner-1)

	switch {
	case b.defsLoading && len(rows) == 0:
		return append(out, messageLines(iw, body, dimStyle, "loading resource definitions…")...)
	case len(rows) == 0:
		return append(out, messageLines(iw, body, dimStyle, "(no match)")...)
	}
	start := clampScrollStart(p.scroll, p.cur, len(rows), body)
	for i := start; i < len(rows) && i < start+body; i++ {
		e := rows[i]
		cnt, dim := b.paletteCell(e)
		text := marker(i == p.cur) + fit(e.name, nameW)
		if showAlias {
			text += " " + fit(strings.Join(e.aliases, ","), aliasW)
		}
		if showCat {
			text += " " + fit(catalog.Label(e.catKey), catW)
		}
		if showKind {
			text += " " + fit(e.kindLabel(), kindW)
		}
		text += " " + padLeft(cnt, countW)
		out = append(out, rowStyle(i == p.cur, true, dim).Render(fit(text, iw)))
	}
	return out
}

func (app App) palettePaneTitle(p pane) string {
	return fmt.Sprintf("%s (%d)", p.title, len(app.browser.paletteRows(p.filter)))
}
