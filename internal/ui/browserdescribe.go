package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss/tree"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/catalog"
	"github.com/florianspk/t9s/internal/talos"
)

// Describe pane (k9s `d`): what a config kind or resource type is, what it is
// on Ubuntu, and which controllers write and read it. The rows are rebuilt on
// every render from the pane's subject plus the cached dependency graph, so a
// graph that arrives late fills in without reopening the pane.

// descSubject is what the describe pane is about.
type descSubject struct {
	cfg     bool
	ck      catalog.ConfigKind
	def     talos.ResourceDef
	meta    talos.ResourceMeta // the selected instance, when there is one
	hasMeta bool
}

// drow is one logical row of the describe pane.
type drow struct {
	label string // section label, set on the first row of a section
	text  string
	head  bool   // title row: full width, no label column
	raw   bool   // tree line: cut instead of word-wrapped
	typ   string // full resource type this row names (jump target)
	sel   bool   // selectable: names a type that exists on the node
	dim   bool
}

// dline is one rendered (wrapped) line of the describe pane.
type dline struct {
	label, text string
	head        bool
	sel         bool // first line of a selectable row
	selIdx      int  // ordinal among selectable rows
	dim         bool
}

const (
	descLabelW  = 14
	maxDescKids = 6 // controllers and children shown before "… N more"
)

// findConfigKind looks a kind up in the catalogue (plus the legacy document).
func findConfigKind(kind string) (catalog.ConfigKind, bool) {
	if kind == legacyKind.Kind {
		return legacyKind, true
	}
	for _, k := range catalog.ConfigKinds() {
		if k.Kind == kind {
			return k, true
		}
	}
	return catalog.ConfigKind{Kind: kind}, false
}

// --- row building ---

func (app App) describeRows(p pane) []drow {
	if p.sub.cfg {
		return app.configDescribeRows(p.sub)
	}
	return app.resourceDescribeRows(p.sub)
}

func labelled(rows []drow, label, text string) []drow {
	if strings.TrimSpace(text) == "" {
		return rows
	}
	return append(rows, drow{label: label, text: text})
}

func (app App) configDescribeRows(s descSubject) []drow {
	b := app.browser
	ck := s.ck
	cat := catalog.ConfigCategoryFor(ck.Group)
	rows := []drow{{head: true, text: fmt.Sprintf("%s (machine config document · group %s · %s)",
		ck.Kind, ck.Group, catalog.Label(cat))}}
	rows = labelled(rows, "WHAT", ck.Desc)
	rows = labelled(rows, "ON UBUNTU", ck.Ubuntu)
	rows = labelled(rows, "SINCE", ck.Since)
	switch b.cfgState {
	case cfgLoaded:
		rows = labelled(rows, "IN THIS NODE", fmt.Sprintf("%d document(s)", len(b.docsOfKind(ck.Kind))))
	case cfgDenied:
		rows = labelled(rows, "IN THIS NODE", padlock()+" "+app.lockReason())
	}
	g, note := app.depGraph()
	if note != "" {
		return append(rows, drow{text: note, dim: true})
	}
	// Every controller that reads the machine config is listed; keep those of
	// the kind's own group first (controller names start with their package).
	consumers := g.Consumers(machineConfigType)
	prefix := ck.Group
	if prefix == "kubernetes" {
		prefix = "k8s"
	}
	var own []string
	for _, c := range consumers {
		if strings.HasPrefix(c, prefix+".") {
			own = append(own, c)
		}
	}
	if len(own) > 0 {
		consumers = own
	}
	return app.controllerTrees(rows, "FEEDS", g, consumers, false)
}

// machineConfigType is the resource type controllers read the config from.
const machineConfigType = "MachineConfigs.config.talos.dev"

func (app App) resourceDescribeRows(s descSubject) []drow {
	b := app.browser
	d := s.def
	meta := []string{d.Type, "ns " + d.DefaultNamespace}
	if len(d.Aliases) > 0 {
		meta = append(meta, "aliases "+strings.Join(d.Aliases, ", "))
	}
	if d.Sensitive {
		meta = append(meta, "sensitive: needs os:admin")
	}
	rows := []drow{{head: true, text: fmt.Sprintf("%s (%s)", d.DisplayType, strings.Join(meta, " · "))}}
	rows = labelled(rows, "CATEGORY", catalog.Label(catalog.CategoryFor(d)))
	if n, ok := b.counts[d.Type]; ok && n >= 0 {
		rows = labelled(rows, "ON THIS NODE", fmt.Sprintf("%d instance(s)", n))
	}
	if note, ok := catalog.NoteFor(d.DisplayType); ok {
		rows = labelled(rows, "WHAT", note.What)
		rows = labelled(rows, "ON UBUNTU", note.Ubuntu)
		rows = labelled(rows, "LOOK HERE", strings.Join(note.LookWhen, " · "))
	}
	if s.hasMeta && s.meta.Owner != "" {
		rows = labelled(rows, "WRITTEN BY", fmt.Sprintf("%s (owner of %s)", s.meta.Owner, s.meta.ID))
	}
	g, note := app.depGraph()
	if note != "" {
		return append(rows, drow{text: note, dim: true})
	}
	rows = app.controllerTrees(rows, "FED BY", g, g.Producers(d.Type), true)
	return app.controllerTrees(rows, "FEEDS", g, g.Consumers(d.Type), false)
}

// depGraph returns the browser node's cached graph, or a dim note saying why
// there is none.
func (app App) depGraph() (talos.DepGraph, string) {
	e, ok := app.deps[app.browser.node.IP]
	switch {
	case !ok || e.loading:
		return talos.DepGraph{}, "loading relationships…"
	case errors.Is(e.err, talos.ErrNeedsGRPC):
		return talos.DepGraph{}, "relationships need the gRPC source (--source=grpc)"
	case e.err != nil:
		return talos.DepGraph{}, "relationships unavailable: " + shortReason(e.err)
	}
	return e.g, ""
}

// controllerTrees appends one small tree per controller: the controller, then
// the types it reads (inputs) or writes (outputs).
func (app App) controllerTrees(rows []drow, label string, g talos.DepGraph, ctrls []string, inputs bool) []drow {
	// Arrows show the direction of flow: inputs point at the controller that
	// reads them, outputs point away to what it writes.
	arrow := "▶"
	if inputs {
		arrow = "◀"
	}
	for i, c := range ctrls {
		if i == maxDescKids {
			rows = append(rows, drow{text: fmt.Sprintf("… %d more controllers", len(ctrls)-maxDescKids), dim: true})
			break
		}
		edges := g.Outputs(c)
		if inputs {
			edges = g.Inputs(c)
		}
		t := tree.Root(c).Enumerator(func(ch tree.Children, i int) string {
			if i == ch.Length()-1 {
				return "└─" + arrow
			}
			return "├─" + arrow
		}).Indenter(func(tree.Children, int) string { return "   " })
		var kids []drow
		for j, e := range edges {
			if j == maxDescKids {
				kids = append(kids, drow{dim: true, raw: true, text: fmt.Sprintf("… %d more", len(edges)-maxDescKids)})
				t.Child(kids[len(kids)-1].text)
				break
			}
			_, exists := app.browser.lookupType(e.Type)
			kids = append(kids, drow{raw: true, typ: e.Type, sel: exists, dim: !exists, text: e.Type})
			t.Child(kids[len(kids)-1].text)
		}
		lines := strings.Split(t.String(), "\n")
		for j, ln := range lines {
			r := drow{text: ln, raw: true}
			if j > 0 && j-1 < len(kids) {
				k := kids[j-1]
				// the tree prefixed the child text; keep our flags on the line
				r.typ, r.sel, r.dim = k.typ, k.sel, k.dim
			}
			if j == 0 && i == 0 {
				r.label = label
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// lookupType finds a resource type of the node by its full type string.
func (b browser) lookupType(typ string) (talos.ResourceDef, bool) {
	for _, d := range b.defs {
		if d.Type == typ {
			return d, true
		}
	}
	return talos.ResourceDef{}, false
}

// --- layout ---

// describeVisual wraps the rows to w cells and numbers the selectable ones.
func describeVisual(rows []drow, w int) []dline {
	var out []dline
	sel := 0
	for _, r := range rows {
		switch {
		case r.head:
			for _, c := range wordWrap(r.text, max(1, w)) {
				out = append(out, dline{text: c, head: true})
			}
			continue
		case r.text == "":
			out = append(out, dline{label: r.label})
			continue
		}
		tw := max(1, w-descLabelW)
		var chunks []string
		if r.raw {
			chunks = wrapChunks(r.text, tw)
		} else {
			chunks = wordWrap(r.text, tw)
		}
		for i, c := range chunks {
			l := dline{text: c, dim: r.dim}
			if i == 0 {
				l.label = r.label
				if r.sel {
					l.sel, l.selIdx = true, sel
				}
			} else if r.raw {
				l.text = "   " + c // continuation of a long tree line
			}
			out = append(out, l)
		}
		if r.sel {
			sel++
		}
	}
	return out
}

// selectableLines are the visual line indexes of the selectable rows.
func selectableLines(vl []dline) []int {
	var out []int
	for i, l := range vl {
		if l.sel {
			out = append(out, i)
		}
	}
	return out
}

func (app App) describeLines(p pane, iw, inner int) []string {
	vl := describeVisual(app.describeRows(p), iw)
	start := clamp(p.scroll, 0, max(0, len(vl)-inner))
	var out []string
	for i := start; i < len(vl) && i < start+inner; i++ {
		l := vl[i]
		switch {
		case l.head:
			out = append(out, titleStyle.Render(fit(l.text, iw)))
			continue
		}
		lab := yamlKeyStyle.Render(fit(l.label, descLabelW))
		body := fit(l.text, max(0, iw-descLabelW))
		switch {
		case l.sel && l.selIdx == p.cur:
			body = selectedStyle.Render(body)
		case l.dim:
			body = dimStyle.Render(body)
		default:
			body = colorTreeText(body)
		}
		out = append(out, lab+body)
	}
	if len(out) == 0 {
		return []string{dimStyle.Render(fit("  (nothing to describe)", iw))}
	}
	return out
}

func (app App) paneDescribeLen(p pane) int {
	return len(describeVisual(app.describeRows(p), app.yamlInnerWidth()))
}

// describeMove moves the selection one selectable row (j/k). It reports false
// when the pane has nothing selectable, so the caller scrolls instead. Moving
// past the first or last row scrolls one line.
func (app App) describeMove(delta int) (App, bool) {
	p, ok := app.browser.top()
	if !ok {
		return app, false
	}
	vl := describeVisual(app.describeRows(p), app.yamlInnerWidth())
	sels := selectableLines(vl)
	if len(sels) == 0 {
		return app, false
	}
	rows := app.paneInnerRows(paneDescribe)
	app.browser = app.browser.withTop(func(p *pane) {
		cur := p.cur + delta
		switch {
		case cur < 0:
			cur = 0
			p.scroll = max(0, p.scroll-1)
		case cur >= len(sels):
			cur = len(sels) - 1
			p.scroll = clamp(p.scroll+1, 0, max(0, len(vl)-rows))
		}
		p.cur = cur
		line := sels[cur]
		switch {
		case line < p.scroll:
			p.scroll = line
		case line >= p.scroll+rows:
			p.scroll = line - rows + 1
		}
		p.scroll = clamp(p.scroll, 0, max(0, len(vl)-rows))
	})
	return app, true
}

// describeJump implements Enter in the describe pane: jump to the selected
// type as the palette would, so Esc behaves the same.
func (app App) describeJump() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok || p.kind != paneDescribe {
		return app, nil
	}
	var typ string
	for _, r := range app.describeRows(p) {
		if r.sel {
			if p.cur == 0 {
				typ = r.typ
				break
			}
			p.cur--
		}
	}
	if typ == "" {
		return app, nil
	}
	e, ok := app.browser.lookupExact(typ)
	if !ok {
		return app, nil
	}
	return app.jumpTo(e)
}

// --- opening ---

// openDescribe implements `d` on a types or instances pane.
func (app App) openDescribe() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok {
		return app, nil
	}
	b := app.browser
	var title string
	var sub descSubject
	switch p.kind {
	case paneTypes:
		rows := b.typeEntries(p.category, p.filter)
		if p.cur >= len(rows) {
			return app, nil
		}
		e := rows[p.cur]
		title = e.name()
		if e.config {
			sub = descSubject{cfg: true, ck: e.ck}
		} else {
			sub = descSubject{def: e.def}
			if only, ok := b.singles[e.def.Type]; ok && b.counts[e.def.Type] == 1 {
				sub.meta, sub.hasMeta = only, true
			}
		}
	case paneInstances:
		title = p.title
		if p.cfgKind != "" {
			ck, _ := findConfigKind(p.cfgKind)
			sub = descSubject{cfg: true, ck: ck}
		} else {
			sub = descSubject{def: p.def}
			if items := filterInstances(p.items, p.filter); p.cur < len(items) {
				sub.meta, sub.hasMeta = items[p.cur], true
			}
		}
	default:
		return app, nil
	}
	app.browser = b.push(pane{kind: paneDescribe, title: "Describe " + title, sub: sub})
	app = app.syncBrowserState()
	return app.ensureDeps()
}

// describeToYAML implements `y` in the describe pane: close it and open the
// row underneath, as Enter would.
func (app App) describeToYAML() (App, tea.Cmd) {
	app = app.popPane()
	return app.browserEnter()
}
