package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/florianspk/t9s/internal/talos"
)

// Cross-node compare (`c`): the same resource (or config document) on every
// node of the cluster, with a unified diff against the node the browser was
// opened on. The key is `c`, not the plan's `x`: `x` is t9s' global context
// switcher and is handled before the browser's key table.

type cmpState int

const (
	cmpLoading cmpState = iota
	cmpPresent
	cmpAbsent
	cmpLocked
	cmpError
)

// compareSubject identifies what is compared: a COSI resource instance, or a
// config document (kind + name).
type compareSubject struct {
	cfg   bool
	def   talos.ResourceDef // resources
	ns    string
	id    string // resources: instance ID
	kind  string // config documents
	name  string // config documents: `name:` (may be empty)
	label string
}

type cmpRow struct {
	node    talos.Node
	state   cmpState
	yaml    string // normalised
	version string // metadata.version of the instance on that node
	err     string
}

type compareView struct {
	subject compareSubject
	base    string // IP of the node the browser was opened on
	seq     uint64
	rows    []cmpRow
}

type compareNodeMsg struct {
	seq     uint64
	ip      string
	state   cmpState
	yaml    string
	version string
	err     error
}

// --- normalisation ---

// normalizeResourceYAML drops what always differs between nodes: the `node`
// line and metadata.version / created / updated. It also returns the
// instance's metadata.version for the VERSION column.
func normalizeResourceYAML(y string) (norm, version string) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(y), &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return y, ""
	}
	root := doc.Content[0]
	dropKey(root, "node")
	if md := mapChild(root, "metadata"); md != nil && md.Kind == yaml.MappingNode {
		if v := mapChild(md, "version"); v != nil && v.Kind == yaml.ScalarNode {
			version = v.Value
		}
		for _, k := range []string{"version", "created", "updated"} {
			dropKey(md, k)
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return y, version
	}
	return string(out), version
}

func mapChild(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func dropKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i:i], m.Content[i+2:]...)
			return
		}
	}
}

// --- fetching ---

// fetchCompare reads the subject from one node. It never returns an error:
// the outcome is in the message state.
func fetchCompare(ctx context.Context, src talos.ResourceSource, getCfg func(context.Context, string) (string, error), subj compareSubject, ip string, seq uint64) compareNodeMsg {
	msg := compareNodeMsg{seq: seq, ip: ip}
	fail := func(err error) compareNodeMsg {
		switch {
		case talos.IsPermissionDenied(err):
			msg.state = cmpLocked
		case talos.IsNotFound(err):
			msg.state = cmpAbsent
		default:
			msg.state, msg.err = cmpError, err
		}
		return msg
	}
	if subj.cfg {
		raw, err := getCfg(ctx, ip)
		if err != nil {
			return fail(err)
		}
		docs, err := talos.SplitConfigDocs(raw)
		if err != nil {
			return fail(err)
		}
		for _, d := range docs {
			if d.Kind == subj.kind && d.Name == subj.name {
				msg.state, msg.yaml = cmpPresent, d.YAML
				return msg
			}
		}
		msg.state = cmpAbsent
		return msg
	}
	y, err := src.GetYAML(ctx, ip, subj.ns, subj.def.Type, subj.id)
	if err != nil {
		return fail(err)
	}
	msg.state = cmpPresent
	msg.yaml, msg.version = normalizeResourceYAML(y)
	return msg
}

func (app App) configGetter() func(context.Context, string) (string, error) {
	if app.getConfig != nil {
		return app.getConfig
	}
	client := app.client
	return func(ctx context.Context, node string) (string, error) { return client.GetMachineConfig(ctx, node) }
}

// loadCompare fetches the subject from every node concurrently (shared
// semaphore, 8 at a time, 10 s each).
func (app App) loadCompare(cv compareView) tea.Cmd {
	src := app.src()
	getCfg := app.configGetter()
	sem := app.resSem
	if sem == nil {
		sem = make(chan struct{}, 8)
	}
	cmds := make([]tea.Cmd, 0, len(cv.rows))
	for _, r := range cv.rows {
		ip, subj, seq := r.node.IP, cv.subject, cv.seq
		cmds = append(cmds, func() tea.Msg {
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
			defer cancel()
			return fetchCompare(ctx, src, getCfg, subj, ip, seq)
		})
	}
	return tea.Batch(cmds...)
}

// --- opening ---

// compareNodes lists the nodes to compare: every cluster node, the browser's
// node included (added if the member list does not have it).
func (app App) compareNodes() []talos.Node {
	var out []talos.Node
	seen := map[string]bool{}
	for _, n := range app.nodes {
		if !seen[n.IP] {
			seen[n.IP] = true
			out = append(out, n)
		}
	}
	if b := app.browser.node; b.IP != "" && !seen[b.IP] {
		out = append([]talos.Node{b}, out...)
	}
	return out
}

func (app App) newCompareView(subj compareSubject) compareView {
	app.compareSeq++
	cv := compareView{subject: subj, base: app.browser.node.IP, seq: app.compareSeq}
	for _, n := range app.compareNodes() {
		cv.rows = append(cv.rows, cmpRow{node: n})
	}
	return cv
}

// compareSubjectFor resolves what `c` compares from the top pane.
func (app App) compareSubjectFor() (compareSubject, string) {
	p, ok := app.browser.top()
	if !ok {
		return compareSubject{}, ""
	}
	b := app.browser
	resource := func(def talos.ResourceDef, m talos.ResourceMeta) compareSubject {
		ns := m.Namespace
		if ns == "" {
			ns = def.DefaultNamespace
		}
		return compareSubject{def: def, ns: ns, id: m.ID, label: def.DisplayType + "/" + m.ID}
	}
	config := func(kind, label string) (compareSubject, string) {
		docs := b.docsOfKind(kind)
		for i, l := range docLabels(docs) {
			if l == label {
				name := docs[i].Name
				lbl := kind
				if name != "" {
					lbl += "/" + name
				}
				return compareSubject{cfg: true, kind: kind, name: name, label: lbl}, ""
			}
		}
		return compareSubject{}, "that config document is gone"
	}
	switch p.kind {
	case paneInstances:
		items := filterInstances(p.items, p.filter)
		if p.cur >= len(items) {
			return compareSubject{}, "nothing selected"
		}
		if p.cfgKind != "" {
			return config(p.cfgKind, items[p.cur].ID)
		}
		return resource(p.def, items[p.cur]), ""
	case paneYAML:
		if p.cfgKind != "" {
			return config(p.cfgKind, p.meta.ID)
		}
		return resource(p.def, p.meta), ""
	case paneTypes:
		rows := b.typeEntries(p.category, p.filter)
		if p.cur >= len(rows) {
			return compareSubject{}, "nothing selected"
		}
		e := rows[p.cur]
		if e.config {
			docs := b.docsOfKind(e.ck.Kind)
			if len(docs) != 1 {
				return compareSubject{}, "compare needs exactly one " + e.ck.Kind + " (open it and pick one)"
			}
			return config(e.ck.Kind, docLabels(docs)[0])
		}
		if only, ok := b.singles[e.def.Type]; ok && b.counts[e.def.Type] == 1 {
			return resource(e.def, only), ""
		}
		return compareSubject{}, "compare needs a type with exactly one instance (open it and pick one)"
	}
	return compareSubject{}, ""
}

// openCompare implements `c`.
func (app App) openCompare() (App, tea.Cmd) {
	subj, why := app.compareSubjectFor()
	if why != "" {
		app.statusMsg = warnStyle.Render(why)
		return app, nil
	}
	if subj.label == "" {
		return app, nil
	}
	cv := app.newCompareView(subj)
	app.compareSeq = cv.seq
	app.browser = app.browser.clearFind().push(pane{kind: paneCompare, title: "Compare " + subj.label, cmp: cv})
	app = app.syncBrowserState()
	app.statusMsg = ""
	return app, app.loadCompare(cv)
}

func (app App) reloadCompare() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok || p.kind != paneCompare {
		return app, nil
	}
	cv := app.newCompareView(p.cmp.subject)
	app.compareSeq = cv.seq
	app.browser = app.browser.withTop(func(p *pane) { p.cmp = cv })
	return app, app.loadCompare(cv)
}

func (app App) handleCompareNode(msg compareNodeMsg) App {
	isPane := func(p pane) bool { return p.kind == paneCompare && p.cmp.seq == msg.seq }
	app.browser = app.browser.withPane(isPane, func(p *pane) {
		rows := make([]cmpRow, len(p.cmp.rows))
		copy(rows, p.cmp.rows)
		for i := range rows {
			if rows[i].node.IP != msg.ip {
				continue
			}
			rows[i].state, rows[i].yaml, rows[i].version, rows[i].err = msg.state, msg.yaml, msg.version, ""
			if msg.err != nil {
				rows[i].err = shortReason(msg.err)
			}
		}
		p.cmp.rows = rows
	})
	return app
}

// --- row model ---

func (cv compareView) baseRow() (cmpRow, bool) {
	for _, r := range cv.rows {
		if r.node.IP == cv.base {
			return r, true
		}
	}
	return cmpRow{}, false
}

// presentText is the PRESENT column.
func (r cmpRow) presentText() string {
	switch r.state {
	case cmpLoading:
		return "…"
	case cmpPresent:
		return "yes"
	case cmpAbsent:
		return "no"
	case cmpLocked:
		return padlock()
	}
	return "err"
}

// sameText is the SAME? column: each node against the browser's node.
func (cv compareView) sameText(r cmpRow) string {
	if r.node.IP == cv.base {
		return "base"
	}
	base, ok := cv.baseRow()
	switch {
	case r.state == cmpLoading || !ok || base.state == cmpLoading:
		return "…"
	case r.state != cmpPresent || base.state != cmpPresent:
		return "-"
	case r.yaml == base.yaml:
		return "yes"
	}
	return "no"
}

// --- diff ---

func (app App) compareEnter() (App, tea.Cmd) {
	p, ok := app.browser.top()
	if !ok || p.kind != paneCompare || p.cur >= len(p.cmp.rows) {
		return app, nil
	}
	cv := p.cmp
	row := cv.rows[p.cur]
	base, hasBase := cv.baseRow()
	switch {
	case row.node.IP == cv.base:
		app.statusMsg = dimStyle.Render("this is the node the browser was opened on")
	case !hasBase || base.state != cmpPresent:
		app.statusMsg = warnStyle.Render("nothing to diff against: absent on the browser's node")
	case row.state != cmpPresent:
		app.statusMsg = warnStyle.Render(row.node.Hostname + ": " + app.describeCmpState(row))
	default:
		lines := unifiedDiff(nodeTitle(base.node), nodeTitle(row.node), base.yaml, row.yaml, 3)
		app.browser = app.browser.push(pane{kind: paneDiff, title: "Diff " + cv.subject.label, diff: lines})
		app = app.syncBrowserState()
		app.statusMsg = ""
		if !diffChanged(lines) {
			app.statusMsg = okStyle.Render("identical")
		}
	}
	return app, nil
}

func (app App) describeCmpState(r cmpRow) string {
	switch r.state {
	case cmpLoading:
		return "still loading"
	case cmpAbsent:
		return "absent"
	case cmpLocked:
		return padlock() + " " + app.lockReason()
	}
	if r.err != "" {
		return "error: " + r.err
	}
	return "error"
}

func nodeTitle(n talos.Node) string {
	if n.Hostname == "" {
		return n.IP
	}
	return fmt.Sprintf("%s (%s)", n.Hostname, n.IP)
}
