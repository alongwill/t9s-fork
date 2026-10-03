package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/netmodel"
	"github.com/florianspk/t9s/internal/talos"
)

// Network view (`N`, `:netview`, `n` in the Networking category): one node's
// network from the physical NIC up, as a tree with a detail pane. The picture
// comes from netmodel; this file holds the pane state, loading and keys.
// netviewrender.go draws it. It is a whole-width pane on the browser stack, so
// jumps (YAML, describe, related, config document) return here with Esc.

const netFetchTimeout = 60 * time.Second

type netView struct {
	seq     uint64
	ready   bool // a fetch finished
	loading bool // a fetch is running
	res     netmodel.FetchResult
	model   netmodel.Model
	docs    bool // the model was built with the machine config documents

	sel       string // key of the selected node
	scroll    int
	collapsed map[string]bool
	pick      int // which document `c` jumps to when a link has several
}

type netFetchMsg struct {
	node string
	seq  uint64
	res  netmodel.FetchResult
}

// --- tree nodes ---

type netKind int

const (
	netLink netKind = iota
	netAddr
	netRoute
	netOp
	netWarn
	netRef    // a link built on this one that hangs under another parent
	netOrphan // a route that leaves through a link the node does not have
	netNote   // what could not be read
)

// netNode is one row of the tree. Nodes are immutable values built from the model.
type netNode struct {
	kind netKind
	key  string // unique, stable across reloads
	link string // owning link (netLink: itself; netRef: the referenced link)
	idx  int    // index into the link's addresses / routes / operators, the warnings or the orphan routes
	kids []netNode
}

// netTree builds the forest: root links with everything under them, then the
// warnings, the routes with no link and the note about unreadable types.
func netTree(m netmodel.Model, res netmodel.FetchResult) []netNode {
	byName := make(map[string]netmodel.Link, len(m.Links))
	for _, l := range m.Links {
		byName[l.Name] = l
	}
	seen := map[string]bool{}
	var build func(name string) netNode
	build = func(name string) netNode {
		l := byName[name]
		seen[name] = true
		n := netNode{kind: netLink, key: "link:" + name, link: name}
		for i, a := range l.Addresses {
			n.kids = append(n.kids, netNode{kind: netAddr, key: fmt.Sprintf("addr:%s|%s", name, a.Prefix), link: name, idx: i})
		}
		for i, r := range l.Routes {
			n.kids = append(n.kids, netNode{kind: netRoute, key: fmt.Sprintf("route:%s|%s|%s|%d", name, r.Dst, r.Gateway, i), link: name, idx: i})
		}
		for i, op := range l.Operators {
			if !opRealised(l, op) {
				n.kids = append(n.kids, netNode{kind: netOp, key: fmt.Sprintf("op:%s|%s|%s", name, op.Kind, op.VIP), link: name, idx: i})
			}
		}
		for _, c := range m.Children[name] {
			if !seen[c] {
				n.kids = append(n.kids, build(c))
			}
		}
		for _, c := range m.Also[name] {
			n.kids = append(n.kids, netNode{kind: netRef, key: "ref:" + name + ">" + c, link: c})
		}
		return n
	}
	var out []netNode
	for _, r := range m.Roots {
		if !seen[r] {
			out = append(out, build(r))
		}
	}
	for i, w := range m.Warnings {
		out = append(out, netNode{kind: netWarn, key: fmt.Sprintf("warn:%d|%s|%s", i, w.Config.Kind, w.Name), idx: i})
	}
	for i, r := range m.OrphanRoutes {
		out = append(out, netNode{kind: netOrphan, key: fmt.Sprintf("orphan:%s|%s|%s", r.Dst, r.Gateway, r.OutLink), idx: i})
	}
	if len(res.Denied)+len(res.Failed) > 0 {
		out = append(out, netNode{kind: netNote, key: "note:unreadable"})
	}
	return out
}

// opRealised reports whether the operator already produced an address.
func opRealised(l netmodel.Link, op netmodel.Operator) bool {
	for _, a := range l.Addresses {
		switch {
		case op.Kind == "vip" && strings.HasPrefix(a.Prefix, op.VIP+"/"):
			return true
		case op.Kind == "dhcp4" && a.Source == netmodel.SrcDHCP4, op.Kind == "dhcp6" && a.Source == netmodel.SrcDHCP6:
			return true
		}
	}
	return op.Kind == "lldp" // lldp announces, it never assigns
}

// netFlat is a visible row: the node and where it sits.
type netFlat struct {
	node       netNode
	expandable bool
	collapsed  bool
}

// netFlatten lists the visible rows in the order the tree is drawn.
func netFlatten(nodes []netNode, collapsed map[string]bool) []netFlat {
	var out []netFlat
	var walk func(ns []netNode)
	walk = func(ns []netNode) {
		for _, n := range ns {
			f := netFlat{node: n, expandable: len(n.kids) > 0, collapsed: collapsed[n.key]}
			out = append(out, f)
			if f.expandable && !f.collapsed {
				walk(n.kids)
			}
		}
	}
	walk(nodes)
	return out
}

// --- state helpers ---

func (app App) netPane() (pane, bool) {
	p, ok := app.browser.top()
	if !ok || p.kind != paneNetwork {
		return pane{}, false
	}
	return p, true
}

func (app App) setNet(f func(nv *netView)) App {
	app.browser = app.browser.withTop(func(p *pane) {
		if p.kind == paneNetwork {
			f(&p.net)
		}
	})
	return app
}

// netBuild builds the model from a fetch, adding the config documents when the
// browser has them.
func (app App) netBuild(res netmodel.FetchResult) (netmodel.Model, bool) {
	in := res.In
	docs := app.browser.cfgState == cfgLoaded
	if docs {
		in.Docs = app.browser.docs
	}
	return netmodel.Build(in), docs
}

// netRebuild rebuilds the model of every network pane, after the machine
// config arrives.
func (app App) netRebuild() App {
	st := make([]pane, len(app.browser.stack))
	copy(st, app.browser.stack)
	for i := range st {
		if st[i].kind == paneNetwork && st[i].net.ready {
			st[i].net.model, st[i].net.docs = app.netBuild(st[i].net.res)
		}
	}
	app.browser.stack = st
	return app
}

// --- opening and loading ---

// openNetwork pushes the network view. root makes it the only pane (from the
// node list, so Esc returns straight to the node).
func (app App) openNetwork(root bool) (App, tea.Cmd) {
	if !isBrowserState(app.state) || len(app.browser.stack) == 0 {
		return app, nil
	}
	if p, ok := app.browser.top(); ok && p.kind == paneNetwork {
		return app, nil
	}
	nv := netView{seq: 1, loading: true, collapsed: map[string]bool{}}
	np := pane{kind: paneNetwork, title: "Network", net: nv}
	if root {
		app.browser.stack = []pane{np}
	} else {
		app.browser = app.browser.clearFind().push(np)
	}
	app.browser.fullscreen = false
	app.statusMsg = ""
	app = app.syncBrowserState()
	app, cfgCmd := app.ensureConfig()
	return app, tea.Batch(app.loadNetwork(nv.seq), cfgCmd)
}

// openNetworkFromList implements `N` on the node list.
func (app App) openNetworkFromList(n talos.Node) (App, tea.Cmd) {
	app, openCmd := app.openBrowser(n)
	app, cmd := app.openNetwork(true)
	return app, tea.Batch(openCmd, cmd)
}

func (app App) loadNetwork(seq uint64) tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	sem := app.resSem
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), netFetchTimeout)
		defer cancel()
		return netFetchMsg{node: node, seq: seq, res: netmodel.Fetch(ctx, src, node, sem)}
	}
}

func (app App) handleNetFetch(msg netFetchMsg) App {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app
	}
	model, docs := app.netBuild(msg.res)
	app.browser = app.browser.withPane(
		func(p pane) bool { return p.kind == paneNetwork && p.net.seq == msg.seq },
		func(p *pane) {
			p.net.loading, p.net.ready = false, true
			p.net.res, p.net.model, p.net.docs = msg.res, model, docs
			flat := netFlatten(netTree(model, msg.res), p.net.collapsed)
			if len(flat) > 0 && netIndexOf(flat, p.net.sel) < 0 {
				p.net.sel = flat[0].node.key
			}
		})
	return app
}

func (app App) reloadNetwork() (App, tea.Cmd) {
	p, ok := app.netPane()
	if !ok {
		return app, nil
	}
	seq := p.net.seq + 1
	app = app.setNet(func(nv *netView) { nv.seq, nv.loading = seq, true })
	app, cfgCmd := app.reloadConfig(nil)
	app.statusMsg = dimStyle.Render("reloading the network…")
	return app, tea.Batch(app.loadNetwork(seq), cfgCmd)
}

// --- selection ---

func netIndexOf(flat []netFlat, key string) int {
	for i, f := range flat {
		if f.node.key == key {
			return i
		}
	}
	return -1
}

// netRows is the tree of the top network pane, flattened.
func (app App) netRows(p pane) []netFlat {
	return netFlatten(netTree(p.net.model, p.net.res), p.net.collapsed)
}

// netSelected returns the selected row.
func (app App) netSelected(p pane) (netFlat, int, bool) {
	flat := app.netRows(p)
	i := netIndexOf(flat, p.net.sel)
	if i < 0 {
		if len(flat) == 0 {
			return netFlat{}, 0, false
		}
		i = 0
	}
	return flat[i], i, true
}

// netSelect moves the selection to row i and keeps it on screen.
func (app App) netSelect(flat []netFlat, i int) App {
	if len(flat) == 0 {
		return app
	}
	i = clamp(i, 0, len(flat)-1)
	rows := app.netTreeRows()
	return app.setNet(func(nv *netView) {
		nv.sel = flat[i].node.key
		nv.scroll = clampScrollStart(nv.scroll, i, len(flat), rows)
		nv.pick = 0
	})
}

func (app App) netMove(delta int) (App, tea.Cmd) {
	p, ok := app.netPane()
	if !ok {
		return app, nil
	}
	flat := app.netRows(p)
	_, i, ok := app.netSelected(p)
	if !ok {
		return app, nil
	}
	return app.netSelect(flat, i+delta), nil
}

func (app App) netEdge(top bool) (App, tea.Cmd) {
	if top {
		return app.netMove(-bigMove)
	}
	return app.netMove(bigMove)
}

func (app App) netPage(dir int) (App, tea.Cmd) {
	return app.netMove(dir * max(1, app.netTreeRows()-1))
}

// netCollapse implements h/←: collapse an expanded row, else go to the parent.
func (app App) netCollapse() (App, tea.Cmd) {
	p, ok := app.netPane()
	if !ok {
		return app, nil
	}
	f, i, ok := app.netSelected(p)
	if !ok {
		return app, nil
	}
	if f.expandable && !f.collapsed {
		return app.setNet(func(nv *netView) { nv.collapsed = withKey(nv.collapsed, f.node.key, true) }), nil
	}
	flat := app.netRows(p)
	if parent := netParentIndex(app.netTreeOf(p), flat, i); parent >= 0 {
		return app.netSelect(flat, parent), nil
	}
	app.statusMsg = dimStyle.Render("already at the top of the tree")
	return app, nil
}

// netExpand implements l/→: expand a collapsed row, else step to its first child.
func (app App) netExpand() (App, tea.Cmd) {
	p, ok := app.netPane()
	if !ok {
		return app, nil
	}
	f, i, ok := app.netSelected(p)
	if !ok {
		return app, nil
	}
	if f.expandable && f.collapsed {
		return app.setNet(func(nv *netView) { nv.collapsed = withKey(nv.collapsed, f.node.key, false) }), nil
	}
	if f.expandable {
		return app.netSelect(app.netRows(p), i+1), nil
	}
	app.statusMsg = dimStyle.Render("nothing below this row")
	return app, nil
}

func (app App) netTreeOf(p pane) []netNode { return netTree(p.net.model, p.net.res) }

// netParentIndex finds the visible parent row of row i (-1 for a top-level row).
func netParentIndex(nodes []netNode, flat []netFlat, i int) int {
	parent := map[string]string{}
	var walk func(ns []netNode, up string)
	walk = func(ns []netNode, up string) {
		for _, n := range ns {
			parent[n.key] = up
			walk(n.kids, n.key)
		}
	}
	walk(nodes, "")
	return netIndexOf(flat, parent[flat[i].node.key])
}

func withKey(m map[string]bool, key string, on bool) map[string]bool {
	out := make(map[string]bool, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	if on {
		out[key] = true
	} else {
		delete(out, key)
	}
	return out
}

// --- what a row stands for ---

// netItem is what the keys need to know about the selected row.
type netItem struct {
	title   string
	ref     *netmodel.Ref        // the resource Enter opens
	cfgs    []netmodel.ConfigRef // the documents that asked for it
	why     string               // why there is no document
	cfgKind string               // a warning row: the config kind it is about
}

func (app App) netItemOf(p pane, n netNode) netItem {
	m := p.net.model
	link, _ := m.Link(n.link)
	switch n.kind {
	case netLink, netRef:
		it := netItem{title: link.Name, ref: &link.Status, cfgs: link.Configs}
		it.why = netWhyLink(link)
		return it
	case netAddr:
		a := link.Addresses[n.idx]
		it := netItem{title: a.Prefix, ref: a.Status}
		if it.ref == nil {
			it.ref = a.Spec
		}
		if a.Config != nil {
			it.cfgs = []netmodel.ConfigRef{*a.Config}
		}
		it.why = netWhySource(a.Source)
		return it
	case netRoute:
		r := link.Routes[n.idx]
		it := netItem{title: r.Dst, ref: r.Status}
		if r.Config != nil {
			it.cfgs = []netmodel.ConfigRef{*r.Config}
		}
		it.why = netWhySource(r.Source)
		return it
	case netOp:
		op := link.Operators[n.idx]
		it := netItem{title: op.Kind + " on " + link.Name, ref: &op.Spec}
		if op.Config != nil {
			it.cfgs = []netmodel.ConfigRef{*op.Config}
		}
		it.why = "no config document asked for this operator"
		return it
	case netWarn:
		w := m.Warnings[n.idx]
		return netItem{title: w.Name, cfgs: []netmodel.ConfigRef{w.Config}, cfgKind: w.Config.Kind}
	case netOrphan:
		r := m.OrphanRoutes[n.idx]
		it := netItem{title: r.Dst, ref: r.Status}
		it.why = "this route is not from a document of this node's config"
		return it
	}
	return netItem{title: "unreadable types", why: "nothing to open"}
}

func netWhyLink(l netmodel.Link) string {
	if len(l.Configs) > 0 {
		return ""
	}
	if l.Physical {
		return l.Name + " is a physical NIC no document names: it gets Talos's defaults (or what DHCP, the platform or the kernel command line say)"
	}
	return "no document names " + l.Name + ": it was created by the platform, a CNI or another process"
}

func netWhySource(src string) string {
	switch src {
	case netmodel.SrcDHCP4, netmodel.SrcDHCP6:
		return "it came from DHCP: no document asked for the address itself"
	case netmodel.SrcPlatform:
		return "it came from the platform (cloud metadata), not from your config"
	case netmodel.SrcCmdline:
		return "it came from the kernel command line (talos.network.*)"
	case netmodel.SrcDefault:
		return "it is a Talos default"
	case netmodel.SrcKernel:
		return "no spec asked for it: the kernel or another process added it"
	case netmodel.SrcVIP:
		return "the VIP is held by an operator; no document names this address"
	}
	return "no config document made this"
}

// netDef finds the definition of a resource type, falling back to one built
// from the reference so Enter works before the definitions arrive.
func (app App) netDef(ref netmodel.Ref) (talos.ResourceDef, bool) {
	if d, ok := app.browser.lookupType(ref.Type); ok {
		return d, true
	}
	return talos.ResourceDef{Type: ref.Type, DisplayType: ref.Display(), DefaultNamespace: ref.Namespace}, false
}

func netMeta(ref netmodel.Ref) talos.ResourceMeta {
	return talos.ResourceMeta{Namespace: ref.Namespace, Type: ref.Type, ID: ref.ID}
}

// --- keys ---

func netNeedsReady(app App) (netFlat, pane, bool) {
	p, ok := app.netPane()
	if !ok {
		return netFlat{}, pane{}, false
	}
	f, _, ok := app.netSelected(p)
	return f, p, ok
}

// netEnter implements Enter: open the YAML of the row's resource, or its document.
func (app App) netEnter() (App, tea.Cmd) {
	f, p, ok := netNeedsReady(app)
	if !ok {
		app.statusMsg = dimStyle.Render("nothing selected yet")
		return app, nil
	}
	it := app.netItemOf(p, f.node)
	if it.ref == nil {
		if len(it.cfgs) > 0 {
			return app.netConfig()
		}
		app.statusMsg = dimStyle.Render("no resource behind this row")
		return app, nil
	}
	d, _ := app.netDef(*it.ref)
	return app.openYAML(d, netMeta(*it.ref))
}

// netConfig implements `c`: jump to the document that made the row. Pressing it
// again moves on to the next document when several name the same link.
func (app App) netConfig() (App, tea.Cmd) {
	f, p, ok := netNeedsReady(app)
	if !ok {
		return app, nil
	}
	it := app.netItemOf(p, f.node)
	if len(it.cfgs) == 0 {
		switch {
		case !p.net.docs && app.browser.cfgState == cfgDenied:
			app.statusMsg = warnStyle.Render("config documents need os:admin on this node")
		case !p.net.docs:
			app.statusMsg = dimStyle.Render("still loading the machine config…")
		default:
			app.statusMsg = dimStyle.Render(it.why)
		}
		return app, nil
	}
	c := it.cfgs[p.net.pick%len(it.cfgs)]
	app = app.setNet(func(nv *netView) { nv.pick++ })
	label := app.relDocLabel(c.Kind, c.DocIndex)
	if len(it.cfgs) > 1 {
		app.statusMsg = infoStyle.Render(fmt.Sprintf("document %d of %d; c again for the next", p.net.pick%len(it.cfgs)+1, len(it.cfgs)))
	}
	a, cmd := app.openConfigYAML(c.Kind, label)
	if top, _ := a.browser.top(); top.kind != paneYAML {
		a.statusMsg = warnStyle.Render("could not find the document in the machine config")
	}
	return a, cmd
}

// netSubject is the type (or config kind) `d` and `p` act on.
func (app App) netSubject() (relSubject, talos.ResourceMeta, bool, string) {
	f, p, ok := netNeedsReady(app)
	if !ok {
		return relSubject{}, talos.ResourceMeta{}, false, "nothing selected yet"
	}
	it := app.netItemOf(p, f.node)
	if it.cfgKind != "" {
		return subjectOfKind(it.cfgKind), talos.ResourceMeta{}, false, ""
	}
	if it.ref == nil {
		return relSubject{}, talos.ResourceMeta{}, false, "no resource behind this row"
	}
	d, known := app.netDef(*it.ref)
	if !known {
		return relSubject{}, talos.ResourceMeta{}, false, "still loading the resource definitions: try again in a moment"
	}
	return subjectOfDef(d), netMeta(*it.ref), true, ""
}

// netDescribe implements `d`.
func (app App) netDescribe() (App, tea.Cmd) {
	s, meta, hasMeta, why := app.netSubject()
	if why != "" {
		app.statusMsg = dimStyle.Render(why)
		return app, nil
	}
	sub := descSubject{def: s.def, meta: meta, hasMeta: hasMeta}
	title := s.display
	if s.config {
		ck, _ := findConfigKind(s.kind)
		sub = descSubject{cfg: true, ck: ck}
	}
	app.browser = app.browser.push(pane{kind: paneDescribe, title: "Describe " + title, sub: sub})
	app = app.syncBrowserState()
	return app.ensureDeps()
}

// netRelated implements `p`.
func (app App) netRelated() (App, tea.Cmd) {
	s, _, _, why := app.netSubject()
	if why != "" {
		app.statusMsg = dimStyle.Render(why)
		return app, nil
	}
	return app.pushRelated(s, nil)
}

// networkActions is the key table of the network view.
func networkActions() []keyAction {
	return []keyAction{
		{keys: []string{"up", "k"}, label: "↑↓", desc: "Move up", visible: true, fn: func(app App) (App, tea.Cmd) { return app.netMove(-1) }},
		{keys: []string{"down", "j"}, desc: "Move down", fn: func(app App) (App, tea.Cmd) { return app.netMove(1) }},
		{keys: []string{"left", "h"}, label: "←→", desc: "Collapse, or go to the parent", visible: true, fn: (App).netCollapse},
		{keys: []string{"right", "l"}, desc: "Expand, or step into the first child", fn: (App).netExpand},
		{keys: []string{"g", "home"}, desc: "Top", fn: func(app App) (App, tea.Cmd) { return app.netEdge(true) }},
		{keys: []string{"G", "end"}, desc: "Bottom", fn: func(app App) (App, tea.Cmd) { return app.netEdge(false) }},
		{keys: []string{"ctrl+f", "pgdown"}, desc: "Page down", fn: func(app App) (App, tea.Cmd) { return app.netPage(1) }},
		{keys: []string{"ctrl+b", "pgup"}, desc: "Page up", fn: func(app App) (App, tea.Cmd) { return app.netPage(-1) }},
		{keys: []string{"enter"}, label: "↵", desc: "YAML of the row's resource", visible: true, fn: (App).netEnter},
		{keys: []string{"d"}, desc: "Describe (what is this, on Ubuntu)", visible: true, fn: (App).netDescribe},
		{keys: []string{"p"}, desc: "Related resources (pipeline and family)", visible: true, fn: (App).netRelated},
		{keys: []string{"c"}, desc: "Jump to the config document that made it", visible: true, fn: (App).netConfig},
		{keys: []string{"o"}, desc: "Open the HTML stack diagram in the browser", visible: true, fn: (App).netOpenHTML},
		{keys: []string{"ctrl+a"}, label: "^a", desc: "All types (aliases palette)", fn: (App).openPalette},
		{keys: []string{":"}, desc: "Command mode (:nodes :net :netview :q)", fn: (App).openCommandPrompt},
		{keys: []string{"esc", "q"}, label: "Esc/q", desc: "Back", visible: true, fn: (App).browserBack},
		{keys: []string{"ctrl+r"}, label: "^r", desc: "Reload", visible: true, fn: (App).browserReload},
		{keys: []string{"ctrl+c"}, desc: "Quit", fn: func(app App) (App, tea.Cmd) {
			app.cleanup()
			return app, tea.Quit
		}},
	}
}

// netKey is `n` in the Networking category: open the network view.
func (app App) netKey() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok {
		return app, nil
	}
	if app.paneCategoryKey(p) != "networking" {
		app.statusMsg = dimStyle.Render("n opens the network view from the Networking category (or press N on the node list)")
		return app, nil
	}
	return app.openNetwork(false)
}

// paneCategoryKey is the category the pane is in ("" when it is not in one).
func (app App) paneCategoryKey(p pane) string {
	switch p.kind {
	case paneCategories:
		rows := app.browser.categoryRows(p.filter)
		if p.cur < len(rows) {
			return rows[p.cur].key
		}
	case paneTypes:
		return p.category
	case paneInstances:
		return app.defCategory(p)
	}
	return ""
}

// netHTMLMsg reports the diagram written by `o`.
type netHTMLMsg struct {
	path    string
	err     error // writing failed
	openErr error // writing worked, opening did not
}

// openURL opens a file in the browser; tests replace it.
var openURL = func(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, path).Start()
}

// netOpenHTML implements `o`: write the diagram to $TMPDIR and open it.
func (app App) netOpenHTML() (App, tea.Cmd) {
	p, ok := app.netPane()
	if !ok {
		return app, nil
	}
	if !p.net.ready {
		app.statusMsg = dimStyle.Render("still loading the network: try again in a moment")
		return app, nil
	}
	m := p.net.model
	app.statusMsg = dimStyle.Render("writing the diagram…")
	return app, func() tea.Msg {
		path, err := netmodel.WriteHTML(m, "", time.Now())
		if err != nil {
			return netHTMLMsg{err: err}
		}
		return netHTMLMsg{path: path, openErr: openURL(path)}
	}
}

func (app App) handleNetHTML(msg netHTMLMsg) App {
	switch {
	case msg.err != nil:
		app.statusMsg = errStyle.Render("could not write the diagram: " + msg.err.Error())
	case msg.openErr != nil:
		app.statusMsg = warnStyle.Render("diagram written, could not open it (" + msg.openErr.Error() + "): " + msg.path)
	default:
		app.statusMsg = infoStyle.Render("diagram opened in the browser: " + msg.path)
	}
	return app
}
