package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/talos"
)

// Related view (`p`): the pipeline of a type along the controller graph (top)
// and the family of types that share its stem, joined by ID (bottom). It is a
// whole-width pane on the browser stack.

const (
	relFocusPipeline = iota
	relFocusTable
)

// relMark identifies a marked table cell by row ID and column title, so marks
// survive a reload that reorders rows.
type relMark struct{ row, col string }

type relatedView struct {
	subject relSubject
	focus   int
	boxTyp  string // type of the selected pipeline box; "" = the subject
	row     int    // table cursor
	col     int
	lists   map[string]relList
	marks   []relMark
	hi      map[string]bool // types to highlight (J: the inputs of the owner controller)
	hiNote  string

	// two-cell diff in flight
	dseq  uint64
	dname [2]string
	dyaml [2]string
	dgot  [2]bool
}

type relListMsg struct {
	node, ns, typ string
	items         []talos.ResourceMeta
	err           error
}

type relYAMLMsg struct {
	node string
	seq  uint64
	slot int
	yaml string
	err  error
}

// --- state helpers ---

func (app App) relPane() (pane, bool) {
	p, ok := app.browser.top()
	if !ok || p.kind != paneRelated {
		return pane{}, false
	}
	return p, true
}

func (app App) setRel(f func(rv *relatedView)) App {
	app.browser = app.browser.withTop(func(p *pane) {
		if p.kind == paneRelated {
			f(&p.rel)
		}
	})
	return app
}

func (app App) hasRelated() bool {
	for _, p := range app.browser.stack {
		if p.kind == paneRelated {
			return true
		}
	}
	return false
}

func (app App) relTable(rv relatedView) relTable {
	b := app.browser
	return buildRelTable(b.relFamily(rv.subject.stem()), rv.lists, b.docs, b.cfgState)
}

// --- opening ---

// relSubjectFromTop is what `p` was pressed on.
func (app App) relSubjectFromTop() (relSubject, bool) {
	p, ok := app.browser.top()
	if !ok {
		return relSubject{}, false
	}
	b := app.browser
	switch p.kind {
	case paneTypes:
		rows := b.typeEntries(p.category, p.filter)
		if p.cur >= len(rows) {
			return relSubject{}, false
		}
		if rows[p.cur].config {
			return subjectOfKind(rows[p.cur].ck.Kind), true
		}
		return subjectOfDef(rows[p.cur].def), true
	case paneInstances, paneYAML:
		if p.cfgKind != "" {
			return subjectOfKind(p.cfgKind), true
		}
		return subjectOfDef(p.def), true
	case paneDescribe:
		if p.sub.cfg {
			return subjectOfKind(p.sub.ck.Kind), true
		}
		return subjectOfDef(p.sub.def), true
	}
	return relSubject{}, false
}

// openRelated implements `p`.
func (app App) openRelated() (App, tea.Cmd) {
	s, ok := app.relSubjectFromTop()
	if !ok {
		return app, nil
	}
	return app.pushRelated(s, nil)
}

func (app App) pushRelated(s relSubject, hi map[string]bool) (App, tea.Cmd) {
	rv := relatedView{subject: s, lists: map[string]relList{}, hi: hi}
	app.browser = app.browser.clearFind().push(pane{kind: paneRelated, title: "Related to " + s.display, rel: rv})
	app = app.syncBrowserState()
	app.statusMsg = ""
	app, depCmd := app.ensureDeps()
	app, cfgCmd := app.ensureConfig()
	app, cntCmd := app.relatedCounts()
	return app, tea.Batch(depCmd, cfgCmd, cntCmd, app.loadRelatedLists(s))
}

// relatedTypes lists the definitions whose counts the view shows: the family
// and every pipeline box the node knows.
func (app App) relatedTypes(s relSubject) []talos.ResourceDef {
	b := app.browser
	var out []talos.ResourceDef
	seen := map[string]bool{}
	add := func(d talos.ResourceDef) {
		if !seen[d.Type] {
			seen[d.Type] = true
			out = append(out, d)
		}
	}
	for _, m := range b.relFamily(s.stem()) {
		if !m.config {
			add(m.def)
		}
	}
	stages, _ := app.relPipeline(s)
	for _, st := range stages {
		for _, bx := range st.boxes {
			if d, ok := b.lookupType(bx.typ); ok && bx.selectable() {
				add(d)
			}
		}
	}
	return out
}

// relatedCountsFor counts what the view shows (counts may already be cached).
func (app App) relatedCountsFor(sub relSubject) (App, tea.Cmd) {
	types := app.relatedTypes(sub)
	cmd := app.loadCounts(types)
	app.browser = app.browser.markCountsLoading(types)
	return app, cmd
}

func (app App) relatedCounts() (App, tea.Cmd) {
	p, ok := app.relPane()
	if !ok {
		return app, nil
	}
	return app.relatedCountsFor(p.rel.subject)
}

// loadRelatedLists lists every type of the family (and the layered namespace
// of merged network specs) through the shared semaphore.
func (app App) loadRelatedLists(s relSubject) tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	sem := app.resSem
	if sem == nil {
		sem = make(chan struct{}, 8)
	}
	var cmds []tea.Cmd
	for _, r := range relRequests(app.browser.relFamily(s.stem())) {
		r := r
		cmds = append(cmds, func() tea.Msg {
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
			defer cancel()
			items, err := src.List(ctx, node, r.ns, r.typ)
			return relListMsg{node: node, ns: r.ns, typ: r.typ, items: items, err: err}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (app App) handleRelList(msg relListMsg) App {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app
	}
	l := relList{items: msg.items}
	if msg.err != nil {
		l = relList{locked: talos.IsPermissionDenied(msg.err), failed: !talos.IsPermissionDenied(msg.err)}
	}
	app.browser = app.browser.withPane(func(p pane) bool { return p.kind == paneRelated }, func(p *pane) {
		m := make(map[string]relList, len(p.rel.lists)+1)
		for k, v := range p.rel.lists {
			m[k] = v
		}
		m[relKey(msg.ns, msg.typ)] = l
		p.rel.lists = m
	})
	return app
}

// afterDeps starts the counts for the pipeline once the graph has arrived.
func (app App) afterDeps() (App, tea.Cmd) {
	for i := len(app.browser.stack) - 1; i >= 0; i-- {
		if p := app.browser.stack[i]; p.kind == paneRelated {
			return app.relatedCountsFor(p.rel.subject)
		}
	}
	return app, nil
}

func (app App) reloadRelated() (App, tea.Cmd) {
	p, ok := app.relPane()
	if !ok {
		return app, nil
	}
	app.browser = app.browser.withTop(func(p *pane) { p.rel.lists = map[string]relList{} })
	app, depCmd := app.reloadDeps()
	app, cfgCmd := app.reloadConfig(nil)
	return app, tea.Batch(depCmd, cfgCmd, app.loadRelatedLists(p.rel.subject))
}

// --- keys ---

func (app App) relFocusSwitch() (App, tea.Cmd) {
	app = app.setRel(func(rv *relatedView) { rv.focus = 1 - rv.focus })
	return app, nil
}

// relDir moves the cursor of the focused section. dx/dy are screen directions.
func (app App) relDir(dx, dy int) (App, tea.Cmd) {
	p, ok := app.relPane()
	if !ok {
		return app, nil
	}
	if p.rel.focus == relFocusTable {
		return app.relMoveCell(dy, dx), nil
	}
	return app.relMoveBox(dx, dy), nil
}

func relSelectable(st relStage) int {
	n := 0
	for _, b := range st.boxes {
		if b.selectable() {
			n++
		}
	}
	return n
}

// relLocate finds the selected box (stage, index). The subject is the default.
func relLocate(stages []relStage, typ string) (int, int, bool) {
	for si, st := range stages {
		for bi, b := range st.boxes {
			if b.typ == typ && b.selectable() {
				return si, bi, true
			}
		}
	}
	return 0, 0, false
}

func (app App) relSelectedTyp(p pane) string {
	if p.rel.boxTyp != "" {
		return p.rel.boxTyp
	}
	return p.rel.subject.typ()
}

// relMoveBox moves between pipeline boxes spatially: in the horizontal layout
// left/right change stage and up/down change box; in the stacked layout the
// axes swap.
func (app App) relMoveBox(dx, dy int) App {
	p, ok := app.relPane()
	if !ok {
		return app
	}
	stages, _ := app.relPipeline(p.rel.subject)
	if len(stages) == 0 {
		return app
	}
	si, bi, found := relLocate(stages, app.relSelectedTyp(p))
	if !found {
		si, bi = 0, 0
	}
	horizontal, _, _ := relLayout(app.width-2, stages)
	dStage, dBox := dx, dy
	if !horizontal {
		dStage, dBox = dy, dx
	}
	si = clamp(si+dStage, 0, len(stages)-1)
	bi = clamp(bi+dBox, 0, max(0, relSelectable(stages[si])-1))
	typ := stages[si].boxes[bi].typ
	return app.setRel(func(rv *relatedView) { rv.boxTyp = typ })
}

func (app App) relMoveCell(dr, dc int) App {
	p, ok := app.relPane()
	if !ok {
		return app
	}
	t := app.relTable(p.rel)
	if len(t.rows) == 0 || len(t.cols) == 0 {
		return app
	}
	return app.setRel(func(rv *relatedView) {
		rv.row = clamp(rv.row+dr, 0, len(t.rows)-1)
		rv.col = clamp(rv.col+dc, 0, len(t.cols)-1)
	})
}

func (app App) relEdge(top bool) (App, tea.Cmd) {
	p, ok := app.relPane()
	if !ok || p.rel.focus != relFocusTable {
		return app, nil
	}
	t := app.relTable(p.rel)
	if len(t.rows) == 0 {
		return app, nil
	}
	return app.setRel(func(rv *relatedView) {
		if top {
			rv.row = 0
		} else {
			rv.row = len(t.rows) - 1
		}
	}), nil
}

func (app App) relEnter() (App, tea.Cmd) {
	p, ok := app.relPane()
	if !ok {
		return app, nil
	}
	if p.rel.focus == relFocusPipeline {
		return app.relOpenBox(p)
	}
	return app.relOpenCell(p)
}

// relOpenBox opens the instances of the selected pipeline box on top of the
// related view, so Esc returns here.
func (app App) relOpenBox(p pane) (App, tea.Cmd) {
	typ := app.relSelectedTyp(p)
	if typ == machineConfigType {
		app.statusMsg = dimStyle.Render("the machine config is browsed as config kinds: Esc, then open a CONFIG row")
		return app, nil
	}
	d, ok := app.browser.lookupType(typ)
	if !ok {
		app.statusMsg = dimStyle.Render(displayFromType(typ) + " is not a resource on this node")
		return app, nil
	}
	return app.openType(d)
}

func (app App) relCellAt(p pane) (relTable, relCell, bool) {
	t := app.relTable(p.rel)
	if p.rel.row >= len(t.rows) || p.rel.col >= len(t.cols) {
		return t, relCell{}, false
	}
	return t, t.cells[p.rel.row][p.rel.col], true
}

func (app App) relOpenCell(p pane) (App, tea.Cmd) {
	t, cell, ok := app.relCellAt(p)
	if !ok {
		return app, nil
	}
	col := t.cols[p.rel.col]
	switch cell.state {
	case cellLocked:
		app.statusMsg = app.lockMessage()
		return app, nil
	case cellLoading:
		app.statusMsg = dimStyle.Render("still loading…")
		return app, nil
	case cellAbsent, cellError:
		app.statusMsg = dimStyle.Render(col.title + " has no " + rowLabel(t.rows[p.rel.row]))
		return app, nil
	}
	if col.config {
		return app.openConfigYAML(col.kind, app.relDocLabel(col.kind, cell.doc))
	}
	return app.openYAML(col.def, cell.meta)
}

// relDocLabel names document i (an index into the node's document list) the
// way the config panes do.
func (app App) relDocLabel(kind string, i int) string {
	docs := app.browser.docsOfKind(kind)
	labels := docLabels(docs)
	n := 0
	for j, d := range app.browser.docs {
		if d.Kind != kind {
			continue
		}
		if j == i && n < len(labels) {
			return labels[n]
		}
		n++
	}
	return ""
}

func (app App) relMarkToggle() (App, tea.Cmd) {
	p, ok := app.relPane()
	if !ok {
		return app, nil
	}
	if p.rel.focus != relFocusTable {
		app.statusMsg = dimStyle.Render("tab moves to the family table, where space marks a cell")
		return app, nil
	}
	t, cell, ok := app.relCellAt(p)
	if !ok {
		return app, nil
	}
	if cell.state != cellPresent {
		app.statusMsg = dimStyle.Render("only a present cell (●) can be marked")
		return app, nil
	}
	m := relMark{row: t.rows[p.rel.row], col: t.cols[p.rel.col].title}
	var marks []relMark
	removed := false
	for _, x := range p.rel.marks {
		if x == m {
			removed = true
			continue
		}
		marks = append(marks, x)
	}
	if !removed {
		if len(marks) >= 2 {
			marks = marks[1:] // keep the two most recent
		}
		marks = append(marks, m)
	}
	app = app.setRel(func(rv *relatedView) { rv.marks = marks })
	switch len(marks) {
	case 2:
		app.statusMsg = infoStyle.Render("2 marked: press c to diff them")
	default:
		app.statusMsg = dimStyle.Render(fmt.Sprintf("%d/2 marked", len(marks)))
	}
	return app, nil
}

func (app App) relClearMarks() App {
	return app.setRel(func(rv *relatedView) { rv.marks = nil })
}

// relDiff implements `c`: fetch the two marked cells and diff them.
func (app App) relDiff() (App, tea.Cmd) {
	p, ok := app.relPane()
	if !ok {
		return app, nil
	}
	if p.rel.focus != relFocusTable {
		app.statusMsg = dimStyle.Render("tab moves to the family table: mark two cells with space, then c")
		return app, nil
	}
	if len(p.rel.marks) != 2 {
		app.statusMsg = warnStyle.Render(fmt.Sprintf("mark two cells with space first (%d/2)", len(p.rel.marks)))
		return app, nil
	}
	t := app.relTable(p.rel)
	app.compareSeq++
	seq := app.compareSeq
	var names [2]string
	var yamls [2]string
	var got [2]bool
	var cmds []tea.Cmd
	for slot, m := range p.rel.marks {
		r, c := -1, -1
		for i, k := range t.rows {
			if k == m.row {
				r = i
			}
		}
		for i, col := range t.cols {
			if col.title == m.col {
				c = i
			}
		}
		if r < 0 || c < 0 || t.cells[r][c].state != cellPresent {
			app.statusMsg = warnStyle.Render("a marked cell is gone: mark again")
			return app.relClearMarks(), nil
		}
		col, cell := t.cols[c], t.cells[r][c]
		names[slot] = col.title + " " + rowLabel(m.row)
		if col.config {
			yamls[slot], got[slot] = app.browser.docs[cell.doc].YAML, true
			continue
		}
		cmds = append(cmds, app.loadRelYAML(seq, slot, cell.meta, col.def))
	}
	app = app.setRel(func(rv *relatedView) {
		rv.dseq, rv.dname, rv.dyaml, rv.dgot = seq, names, yamls, got
	})
	app.statusMsg = dimStyle.Render("fetching both…")
	if len(cmds) == 0 {
		return app.relFinishDiff(), nil
	}
	return app, tea.Batch(cmds...)
}

func (app App) loadRelYAML(seq uint64, slot int, m talos.ResourceMeta, d talos.ResourceDef) tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	ns := m.Namespace
	if ns == "" {
		ns = d.DefaultNamespace
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
		defer cancel()
		y, err := src.GetYAML(ctx, node, ns, d.Type, m.ID)
		return relYAMLMsg{node: node, seq: seq, slot: slot, yaml: y, err: err}
	}
}

func (app App) handleRelYAML(msg relYAMLMsg) App {
	p, ok := app.relPane()
	if !ok || app.browser.node.IP != msg.node || p.rel.dseq != msg.seq {
		return app
	}
	if msg.err != nil {
		app.statusMsg = errStyle.Render("diff: " + shortReason(msg.err))
		return app.setRel(func(rv *relatedView) { rv.dseq = 0 })
	}
	app = app.setRel(func(rv *relatedView) {
		rv.dyaml[msg.slot], rv.dgot[msg.slot] = msg.yaml, true
	})
	if p, _ := app.relPane(); p.rel.dgot[0] && p.rel.dgot[1] {
		return app.relFinishDiff()
	}
	return app
}

// relFinishDiff pushes the diff pane once both YAMLs are in.
func (app App) relFinishDiff() App {
	p, ok := app.relPane()
	if !ok {
		return app
	}
	a, b := normalizeRelatedYAML(p.rel.dyaml[0]), normalizeRelatedYAML(p.rel.dyaml[1])
	lines := unifiedDiff(p.rel.dname[0], p.rel.dname[1], a, b, 3)
	app = app.setRel(func(rv *relatedView) { rv.dseq = 0 })
	app.browser = app.browser.push(pane{
		kind: paneDiff, title: "Diff " + p.rel.dname[0] + " ↔ " + p.rel.dname[1],
		legend: "(left: first marked, right: second)", diff: lines, side: true,
	})
	app = app.syncBrowserState()
	app.statusMsg = ""
	if !diffChanged(lines) {
		app.statusMsg = okStyle.Render("identical (metadata ignored)")
	}
	return app
}

// --- layout ---

const (
	relBoxW     = 24
	relMinConn  = 6
	relMaxConn  = 24
	relNarrowAt = 120
)

// relLayout decides between the left-to-right pipeline and the stacked one,
// and the box and connector widths. iw is the pane's inner width.
func relLayout(iw int, stages []relStage) (horizontal bool, boxW, connW int) {
	n := len(stages)
	if n == 0 {
		return false, relBoxW, 0
	}
	if iw+2 >= relNarrowAt {
		for _, bw := range []int{relBoxW, 22, 20, 18, 16} {
			if n == 1 {
				return true, bw, 0
			}
			if cw := (iw - n*bw) / (n - 1); cw >= relMinConn {
				return true, bw, min(cw, relMaxConn)
			}
		}
	}
	return false, relBoxW, 0
}

// --- pipeline rendering ---

func clipANSI(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "")
}

// shortCtrl shortens "network.LinkSpecController" to fit w cells.
func shortCtrl(c string, w int) string {
	if i := strings.LastIndex(c, "."); i >= 0 {
		c = c[i+1:]
	}
	if lipgloss.Width(c) > w {
		c = strings.TrimSuffix(c, "Controller")
	}
	if lipgloss.Width(c) > w {
		c = cutWidth(c, max(0, w-1)) + "…"
	}
	return c
}

// relChipView is the one-line form of a box, used when the pipeline is stacked
// (narrow terminals have no height for bordered boxes).
func (app App) relChipView(rb relBox, w int, selected, hi bool) string {
	if !rb.selectable() {
		return dimStyle.Render(fit(" "+rb.display, w))
	}
	cnt := "n/a"
	if d, ok := app.browser.lookupType(rb.typ); ok {
		cnt, _ = app.browser.typeCell(d)
	}
	room := max(1, w-lipgloss.Width(cnt)-3)
	text := fit(" "+fit(cutEllipsis(rb.display, room), room)+" "+cnt+" ", w)
	st := lipgloss.NewStyle().Foreground(roleColor(rb.role))
	switch {
	case hi:
		st = lipgloss.NewStyle().Background(colorMarkAccent).Foreground(colorOnAccent).Bold(true)
	case selected:
		st = lipgloss.NewStyle().Background(roleColor(rb.role)).Foreground(colorOnAccent).Bold(true)
	case !rb.known && rb.typ != machineConfigType:
		st = dimStyle
	}
	return st.Render(text)
}

func (app App) relBoxView(rb relBox, w int, selected, hi bool) string {
	inner := w - 2
	border := lipgloss.RoundedBorder()
	col := roleColor(rb.role)
	st := lipgloss.NewStyle().BorderForeground(col)
	if selected {
		border = lipgloss.ThickBorder()
	}
	if hi {
		border = lipgloss.DoubleBorder()
		st = st.BorderForeground(colorMarkAccent)
	}
	if !rb.selectable() {
		return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorderQuiet).
			Render(dimStyle.Render(fit(" "+rb.display, inner)))
	}

	cnt, dim := "n/a", true
	if d, ok := app.browser.lookupType(rb.typ); ok {
		cnt, dim = app.browser.typeCell(d)
	}
	badge := dimStyle.Render(cnt)
	switch {
	case cnt == padlock():
		badge = lipgloss.NewStyle().Foreground(colorLockAccent).Render(cnt)
	case !dim && inner >= 18:
		badge = chip(cnt, col)
	case !dim:
		badge = lipgloss.NewStyle().Background(col).Foreground(colorOnAccent).Bold(true).Render(cnt)
	}
	name := rb.display
	nameStyle := lipgloss.NewStyle().Foreground(col)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}
	if !rb.known && rb.typ != machineConfigType {
		nameStyle = dimStyle
	}
	room := max(1, inner-lipgloss.Width(badge)-2)
	line := " " + nameStyle.Render(fit(cutEllipsis(name, room), room)) + " " + badge
	lines := []string{clipANSI(padRight(line, inner), inner)}
	if len(rb.cfgKinds) > 0 {
		lines = append(lines, dimStyle.Render(fit(" ↳ "+strings.Join(rb.cfgKinds, ", "), inner)))
	}
	return st.Border(border).Render(strings.Join(lines, "\n"))
}

// cutEllipsis cuts plain text to w cells, ending in … when it had to cut.
func cutEllipsis(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 1 {
		return cutWidth(s, w)
	}
	return cutWidth(s, w-1) + "…"
}

func blockLines(s string) []string { return strings.Split(s, "\n") }

func blanks(n, w int) []string {
	out := make([]string, max(0, n))
	for i := range out {
		out[i] = strings.Repeat(" ", w)
	}
	return out
}

// renderPipeline draws the stages and returns the line range of the selected box.
func (app App) renderPipeline(p pane, stages []relStage, iw int) (lines []string, selTop, selBot int) {
	selTyp := app.relSelectedTyp(p)
	horizontal, boxW, connW := relLayout(iw, stages)
	isHi := func(b relBox) bool { return p.rel.hi[b.typ] }

	if horizontal {
		type col struct {
			lines    []string
			top, bot int // selected box range inside the column
			sel      bool
		}
		cols := make([]col, len(stages))
		height := 0
		for i, st := range stages {
			var ls []string
			c := col{}
			for _, b := range st.boxes {
				bl := blockLines(app.relBoxView(b, boxW, b.typ == selTyp && b.selectable(), isHi(b)))
				if b.typ == selTyp && b.selectable() {
					c.sel, c.top, c.bot = true, len(ls), len(ls)+len(bl)-1
				}
				ls = append(ls, bl...)
			}
			c.lines = ls
			cols[i] = c
			height = max(height, len(ls))
		}
		var parts []string
		for i, c := range cols {
			top := (height - len(c.lines)) / 2
			padded := append(append(blanks(top, boxW), c.lines...), blanks(height-len(c.lines)-top, boxW)...)
			if c.sel {
				selTop, selBot = top+c.top, top+c.bot
			}
			parts = append(parts, strings.Join(padded, "\n"))
			if i < len(cols)-1 {
				parts = append(parts, strings.Join(app.relConnector(stages[i].controllers, connW, height), "\n"))
			}
		}
		return blockLines(lipgloss.JoinHorizontal(lipgloss.Top, parts...)), selTop, selBot
	}

	// stacked: one row of chips per stage, a ▼ connector line between rows
	var blocks []string
	rowsSoFar := 0
	for i, st := range stages {
		n := len(st.boxes)
		bw := clamp((iw-(n-1))/max(1, n), 8, relBoxW)
		var cells []string
		rowSel := false
		for j, b := range st.boxes {
			if j > 0 {
				cells = append(cells, " ")
			}
			sel := b.typ == selTyp && b.selectable()
			rowSel = rowSel || sel
			cells = append(cells, app.relChipView(b, bw, sel, isHi(b)))
		}
		blocks = append(blocks, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		if rowSel {
			selTop, selBot = rowsSoFar, rowsSoFar
		}
		rowsSoFar++
		if i < len(stages)-1 {
			blocks = append(blocks, dimStyle.Render("  ▼ "+relCtrlLabel(st.controllers, iw-4)))
			rowsSoFar++
		}
	}
	return blockLines(lipgloss.JoinVertical(lipgloss.Left, blocks...)), selTop, selBot
}

func relCtrlLabel(ctrls []string, w int) string {
	if len(ctrls) == 0 {
		return ""
	}
	var shorts []string
	for i, c := range ctrls {
		if i == 2 {
			shorts = append(shorts, fmt.Sprintf("+%d", len(ctrls)-2))
			break
		}
		shorts = append(shorts, shortCtrl(c, max(8, w/2-2)))
	}
	return cutWidth(strings.Join(shorts, ", "), max(0, w))
}

// relConnector draws the arrow between two stages with the controllers as
// small dim labels above and below it.
func (app App) relConnector(ctrls []string, w, h int) []string {
	out := blanks(h, w)
	mid := h / 2
	out[mid] = " " + lipgloss.NewStyle().Foreground(colorGray).Render(strings.Repeat("─", max(0, w-3))+"▶") + " "
	label := func(i int) string {
		switch {
		case i >= len(ctrls):
			return strings.Repeat(" ", w)
		case i == 1 && len(ctrls) > 2:
			return " " + dimStyle.Render(fit(fmt.Sprintf("+%d more", len(ctrls)-1), w-2)) + " "
		}
		return " " + dimStyle.Render(fit(shortCtrl(ctrls[i], w-2), w-2)) + " "
	}
	if mid-1 >= 0 {
		out[mid-1] = label(0)
	}
	if mid+1 < h {
		out[mid+1] = label(1)
	}
	return out
}

// --- table rendering ---

func relCellText(c relCell, marked bool) string {
	switch c.state {
	case cellPresent:
		if marked {
			return "◆"
		}
		return "●"
	case cellLocked:
		return padlock()
	case cellLoading:
		return "…"
	case cellError:
		return "err"
	}
	return "·"
}

func relHeaderParts(title string) (name, layer string) {
	n, l, ok := strings.Cut(title, "@")
	if ok {
		l = "@" + l
	}
	return n, l
}

// renderRelTable draws the family table in at most `rows` lines.
func (app App) renderRelTable(p pane, t relTable, iw, rows int, active bool) []string {
	switch {
	case len(t.cols) == 0:
		return []string{dimStyle.Render(fit("  (nothing to compare)", iw))}
	case len(t.rows) == 0:
		return []string{dimStyle.Render(fit("  no instance of any family member on this node", iw))}
	}
	rv := p.rel
	focus := active && rv.focus == relFocusTable
	row := clamp(rv.row, 0, len(t.rows)-1)
	cur := clamp(rv.col, 0, len(t.cols)-1)

	labelW := 4
	for _, k := range t.rows {
		labelW = max(labelW, lipgloss.Width(rowLabel(k)))
	}
	labelW = min(labelW, max(8, iw/3))
	widths := make([]int, len(t.cols))
	for i, c := range t.cols {
		n, l := relHeaderParts(c.title)
		widths[i] = max(6, lipgloss.Width(padlock()), lipgloss.Width(n), lipgloss.Width(l)) + 2
	}
	budget := iw - (labelW + 2) - 1
	// the window of columns that keeps the cursor column on screen
	c0 := 0
	span := func(a, b int) int { // columns a..b inclusive, with their borders
		s := 0
		for i := a; i <= b; i++ {
			s += widths[i] + 1
		}
		return s
	}
	for c0 < cur && span(c0, cur) > budget {
		c0++
	}
	c1 := c0
	for c1+1 < len(t.cols) && span(c0, c1+1) <= budget {
		c1++
	}
	for i := c0; i <= c1; i++ { // squeeze when even one column does not fit
		widths[i] = min(widths[i], max(4, budget-1))
	}

	layered := false
	for i := c0; i <= c1; i++ {
		if strings.Contains(t.cols[i].title, "@") {
			layered = true
		}
	}
	headerLines := 2 // names and the rule
	if layered {
		headerLines = 3 // names, layers and the rule
	}
	vis := max(1, rows-headerLines)
	start := clampScrollStart(0, row, len(t.rows), vis)

	isMarked := func(r, c int) bool {
		for _, m := range rv.marks {
			if m.row == t.rows[r] && m.col == t.cols[c].title {
				return true
			}
		}
		return false
	}

	corner := "ID"
	if c0 > 0 {
		corner = "‹ " + corner
	}
	if c1 < len(t.cols)-1 {
		corner += " ›"
	}
	heads := []string{corner}
	layerRow := []string{""}
	for i := c0; i <= c1; i++ {
		n, l := relHeaderParts(t.cols[i].title)
		heads = append(heads, n)
		layerRow = append(layerRow, l)
	}
	var data [][]string
	if layered {
		data = append(data, layerRow)
	}
	for r := start; r < len(t.rows) && r < start+vis; r++ {
		line := []string{rowLabel(t.rows[r])}
		for c := c0; c <= c1; c++ {
			line = append(line, relCellText(t.cells[r][c], isMarked(r, c)))
		}
		data = append(data, line)
	}

	tb := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(colorBorderQuiet)).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(true).BorderRow(false).
		Headers(heads...).
		Rows(data...).
		StyleFunc(func(r, c int) lipgloss.Style {
			w := labelW + 2
			if c > 0 {
				w = widths[c0+c-1]
			}
			st := lipgloss.NewStyle().Padding(0, 1).Width(w).MaxWidth(w)
			if r == table.HeaderRow {
				if c == 0 {
					return st.Foreground(colorCyan).Bold(true)
				}
				return st.Foreground(roleColor(t.cols[c0+c-1].role)).Bold(true).Align(lipgloss.Center)
			}
			if layered {
				if r == 0 { // the layer line under the names
					return st.Foreground(colorGray).Align(lipgloss.Center)
				}
				r--
			}
			ar := start + r
			if c == 0 {
				if focus && ar == row {
					return st.Background(colorBgSel).Bold(true)
				}
				return st
			}
			ac := c0 + c - 1
			cell := t.cells[ar][ac]
			st = st.Align(lipgloss.Center)
			switch {
			case isMarked(ar, ac):
				st = st.Foreground(colorMarkAccent).Bold(true)
			case cell.state == cellPresent:
				st = st.Foreground(roleColor(t.cols[ac].role))
			case cell.state == cellLocked || cell.state == cellError:
				st = st.Foreground(colorLockAccent)
			default:
				st = st.Foreground(colorGray)
			}
			if focus && ar == row && ac == cur {
				st = st.Background(colorBgSel).Bold(true)
			} else if focus && ar == row {
				st = st.Background(lipgloss.AdaptiveColor{Light: "#eaeef2", Dark: "#161b22"})
			}
			return st
		})
	out := blockLines(tb.String())
	// rule under the header (the table's own rule would sit between the two header lines)
	rule := strings.Repeat("─", labelW+2)
	for i := c0; i <= c1; i++ {
		rule += "┼" + strings.Repeat("─", widths[i])
	}
	ruleAt := 1
	if layered {
		ruleAt = 2
	}
	if ruleAt <= len(out) {
		out = append(out[:ruleAt:ruleAt], append([]string{lipgloss.NewStyle().Foreground(colorBorderQuiet).Render(rule)}, out[ruleAt:]...)...)
	}
	for i := range out {
		out[i] = clipANSI(out[i], iw)
	}
	return out
}

// --- the pane ---

func (app App) relatedLines(p pane, iw, inner int, active bool) []string {
	rv := p.rel
	stages, note := app.relPipeline(rv.subject)
	t := app.relTable(rv)

	head := func(text string, on bool) string {
		if on && active {
			return titleStyle.Render(fit(text, iw))
		}
		return dimStyle.Render(fit(text, iw))
	}
	stem := rv.subject.stem()
	pipeHead := fmt.Sprintf("PIPELINE  %s: who writes it, who reads it", rv.subject.display)
	famHead := fmt.Sprintf("FAMILY  %s*: one row per ID, one column per layer", stem)
	if rv.hiNote != "" {
		pipeHead = "PIPELINE  " + rv.hiNote
	}

	avail := max(0, inner-2) // minus the two section headers
	var pipe []string
	selTop, selBot := 0, 0
	if note != "" {
		pipe = []string{dimStyle.Render(fit("  "+note, iw))}
	} else if len(stages) > 0 {
		pipe, selTop, selBot = app.renderPipeline(p, stages, iw)
	}

	tableNeed := 3 + max(1, min(len(t.rows), 3))
	pH := min(len(pipe), max(4, avail-tableNeed))
	pH = min(pH, avail)
	tableRows := max(0, avail-pH)

	// window the pipeline so the selected box stays visible
	top := 0
	if selBot >= pH { // centre the selected box so both neighbours show
		top = selTop - (pH-(selBot-selTop+1))/2
	}
	top = clamp(top, 0, max(0, len(pipe)-pH))
	if pH < len(pipe) {
		pipe = pipe[top : top+pH]
	}

	out := []string{head(pipeHead, rv.focus == relFocusPipeline)}
	for _, l := range pipe {
		out = append(out, clipANSI(l, iw))
	}
	out = append(out, head(famHead, rv.focus == relFocusTable))
	if tableRows > 0 {
		out = append(out, app.renderRelTable(p, t, iw, tableRows, active)...)
	}
	if len(out) > inner {
		out = out[:inner]
	}
	return out
}

// relatedNextStep is the dim line under the related view.
func (app App) relatedNextStep(p pane) []string {
	if p.rel.focus == relFocusPipeline {
		parts := []string{"←→↑↓ pick a type", "↵ open it", "tab family table"}
		if p.rel.hiNote != "" {
			parts = append([]string{p.rel.hiNote}, parts...)
		}
		return parts
	}
	n := len(p.rel.marks)
	return []string{"←→↑↓ pick a cell", "↵ YAML", fmt.Sprintf("space mark (%d/2)", n), "c diff the two", "tab pipeline"}
}
