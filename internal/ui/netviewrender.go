package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/netmodel"
)

// Drawing of the network view: a summary line, the link tree on the left and a
// detail pane on the right (below the tree under 100 columns).

const (
	netWideFrom = 98 // inner width from which the detail pane sits beside the tree
	netLabelW   = 12
)

// Link kind colours. They match the HTML diagram (netmodel/web).
var (
	netKindColors = map[string]lipgloss.AdaptiveColor{
		"bond":      {Light: "#8250df", Dark: "#bc8cff"},
		"bridge":    {Light: "#bc4c00", Dark: "#ffa657"},
		"vlan":      {Light: "#0969da", Dark: "#58a6ff"},
		"wireguard": {Light: "#9a6700", Dark: "#e3b341"},
		"vxlan":     {Light: "#0a7f8a", Dark: "#39c5cf"},
	}
	netPhysColor = lipgloss.AdaptiveColor{Light: "#1f2328", Dark: "#e6edf3"}
	netWarnColor = lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#e3b341"}
	netUpColor   = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#3fb950"}
	netDownColor = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#ff7b72"}
	netSrcColors = map[string]lipgloss.AdaptiveColor{
		netmodel.SrcStatic:   colorRoleConfig, // what you wrote
		netmodel.SrcDHCP4:    colorRoleSpec,
		netmodel.SrcDHCP6:    colorRoleSpec,
		netmodel.SrcOperator: colorRoleSpec,
		netmodel.SrcVIP:      {Light: "#0a7f8a", Dark: "#39c5cf"},
		netmodel.SrcPlatform: {Light: "#bc4c00", Dark: "#ffa657"},
		netmodel.SrcCmdline:  colorRoleOther,
		netmodel.SrcDefault:  colorRoleOther,
		netmodel.SrcKernel:   colorRoleOther,
	}
)

func netKindColor(l netmodel.Link) lipgloss.AdaptiveColor {
	if l.Physical {
		return netPhysColor
	}
	if c, ok := netKindColors[l.Kind]; ok {
		return c
	}
	return colorRoleOther
}

func netStateDot(state string) string {
	switch state {
	case "up":
		return lipgloss.NewStyle().Foreground(netUpColor).Render("●")
	case "down":
		return lipgloss.NewStyle().Foreground(netDownColor).Render("●")
	}
	return dimStyle.Render("●")
}

// netBadge is the source badge of an address or route.
func netBadge(src string) string {
	c, ok := netSrcColors[src]
	if !ok {
		c = colorRoleOther
	}
	return chip(src, c)
}

func netWarnStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(netWarnColor) }

func cfgLabel(c netmodel.ConfigRef) string {
	switch {
	case c.Detail != "":
		return c.Kind + " " + c.Detail
	case c.Name != "":
		return c.Kind + "/" + c.Name
	}
	return c.Kind
}

// --- layout ---

// netLayout splits the pane body (below the summary line).
func netLayout(iw, inner int) (wide bool, leftW, rightW, treeRows, detailRows int) {
	body := max(1, inner-1)
	if iw >= netWideFrom {
		leftW = clamp(iw*55/100, 40, iw-34)
		return true, leftW, iw - leftW - 1, body, body
	}
	treeRows = max(3, body*6/10)
	return false, iw, iw, treeRows, max(0, body-treeRows-1)
}

// netTreeRows is the number of tree rows on screen, for keeping the cursor visible.
func (app App) netTreeRows() int {
	inner := max(0, app.mainHeight()-2-app.nextStepRows())
	_, _, _, rows, _ := netLayout(max(0, app.width-2), inner)
	return rows
}

// --- row text ---

func (app App) netRowText(m netmodel.Model, n netNode) string {
	link, _ := m.Link(n.link)
	switch n.kind {
	case netLink:
		return netLinkText(link)
	case netRef:
		first := ""
		if len(link.Lowers) > 0 {
			first = link.Lowers[0]
		}
		return dimStyle.Render(fmt.Sprintf("↪ %s (also on this link; shown under %s)", link.Name, first))
	case netAddr:
		a := link.Addresses[n.idx]
		if a.Missing {
			return netWarnStyle().Render("⚠ "+a.Prefix) + " " + netBadge(a.Source) + netWarnStyle().Render(" asked for, not on the link")
		}
		s := a.Prefix + " " + netBadge(a.Source)
		if a.Config != nil {
			s += " " + dimStyle.Render(cfgLabel(*a.Config))
		}
		return s
	case netRoute:
		r := link.Routes[n.idx]
		return netRouteText(r)
	case netOp:
		op := link.Operators[n.idx]
		what := op.Kind
		if op.VIP != "" {
			what += " " + op.VIP
		}
		return netWarnStyle().Render("⟳ "+what+" operator") + dimStyle.Render(", no address yet")
	case netWarn:
		w := m.Warnings[n.idx]
		return netWarnStyle().Render("⚠ " + w.Name + "  " + w.Message)
	case netOrphan:
		r := m.OrphanRoutes[n.idx]
		return netWarnStyle().Render("⚠ ") + netRouteText(r) + netWarnStyle().Render(" — no link "+r.OutLink)
	case netNote:
		return dimStyle.Render("ⓘ some types could not be read (↵ for which)")
	}
	return ""
}

func netRouteText(r netmodel.Route) string {
	s := "⇢ " + r.Dst
	if r.Gateway != "" {
		s += " via " + r.Gateway
	}
	if r.Dst == "default" && r.OutLink != "" {
		s += dimStyle.Render(" (" + r.OutLink + ")")
	}
	s += " " + netBadge(r.Source)
	if r.Config != nil {
		s += " " + dimStyle.Render(cfgLabel(*r.Config))
	}
	return s
}

func netLinkText(l netmodel.Link) string {
	name := lipgloss.NewStyle().Foreground(netKindColor(l))
	if l.Physical {
		name = name.Bold(true)
	}
	s := netStateDot(l.State) + " " + name.Render(l.Name)
	state := l.OperState
	if state == "" {
		state = l.State
	}
	switch {
	case l.Physical:
		s += "  " + state
		if l.SpeedMbit > 0 {
			s += fmt.Sprintf("  %dMb/s", l.SpeedMbit)
		}
		if l.Driver != "" {
			s += "  " + dimStyle.Render(l.Driver)
		}
		if l.HWAddr != "" {
			s += "  " + dimStyle.Render(l.HWAddr)
		}
	case l.Kind != "":
		s += "  " + lipgloss.NewStyle().Foreground(netKindColor(l)).Render(l.Kind)
		if x := netExtra(l); x != "" {
			s += " " + x
		}
		s += "  " + state
		if l.MTU > 0 {
			s += dimStyle.Render(fmt.Sprintf("  mtu %d", l.MTU))
		}
	default:
		s += "  " + dimStyle.Render(l.Type) + "  " + state
	}
	return s
}

// netExtra is the one kind-specific value worth showing in the row.
func netExtra(l netmodel.Link) string {
	for _, kv := range l.Detail {
		switch kv.Key {
		case "vlan id", "bond mode":
			return kv.Value
		}
	}
	return ""
}

// --- the tree ---

func newNetTree(root string) *tree.Tree {
	return tree.Root(root).
		Enumerator(func(c tree.Children, i int) string {
			if i == c.Length()-1 {
				return "└─"
			}
			return "├─"
		}).
		Indenter(func(c tree.Children, i int) string {
			if i == c.Length()-1 {
				return "   "
			}
			return "│  "
		}).
		EnumeratorStyle(lipgloss.NewStyle().Foreground(colorBorderQuiet).PaddingRight(1))
}

// netTreeLines draws the tree; flat is the visible rows in drawing order.
func (app App) netTreeLines(p pane, w, rows int, active bool) []string {
	nodes := app.netTreeOf(p)
	flat := netFlatten(nodes, p.net.collapsed)
	if len(flat) == 0 {
		return messageLines(w, rows, dimStyle, "no links found on this node")
	}
	m := p.net.model
	var build func(ns []netNode, t *tree.Tree)
	build = func(ns []netNode, t *tree.Tree) {
		for _, n := range ns {
			text := app.netRowText(m, n)
			if len(n.kids) == 0 {
				t.Child(text)
				continue
			}
			if p.net.collapsed[n.key] {
				t.Child(text + dimStyle.Render(fmt.Sprintf(" [+%d]", netCount(n))))
				continue
			}
			sub := newNetTree(text)
			build(n.kids, sub)
			t.Child(sub)
		}
	}
	root := newNetTree("•")
	build(nodes, root)
	lines := strings.Split(root.String(), "\n")[1:] // the root line is only an anchor
	selIdx := max(0, netIndexOf(flat, p.net.sel))
	start := clampScrollStart(p.net.scroll, selIdx, len(lines), rows)
	var out []string
	for i := start; i < len(lines) && len(out) < rows; i++ {
		line := lines[i]
		if i == selIdx {
			line = selectedStyle.Render(fit(ansi.Strip(line), w))
			if !active {
				line = pathSelStyle.Render(fit(ansi.Strip(lines[i]), w))
			}
			out = append(out, line)
			continue
		}
		out = append(out, padRight(clipANSI(line, w), w))
	}
	return out
}

// netCount is the number of descendants of a node.
func netCount(n netNode) int {
	c := len(n.kids)
	for _, k := range n.kids {
		c += netCount(k)
	}
	return c
}

// --- summary and detail ---

func (app App) netSummary(p pane) string {
	m := p.net.model
	var parts []string
	if m.Hostname != "" {
		h := m.Hostname
		if m.Domain != "" {
			h += "." + m.Domain
		}
		parts = append(parts, "hostname "+h)
	}
	if len(m.Resolvers) > 0 {
		parts = append(parts, "dns "+strings.Join(m.Resolvers, ", "))
	}
	for _, r := range m.DefaultRoutes {
		parts = append(parts, "default via "+r.Gateway+" ("+r.OutLink+")")
	}
	if len(m.DefaultRoutes) == 0 && p.net.ready {
		parts = append(parts, netWarnStyle().Render("no default route"))
	}
	if p.net.ready && !p.net.docs {
		switch app.browser.cfgState {
		case cfgDenied:
			parts = append(parts, netWarnStyle().Render("config documents need os:admin"))
		case cfgError:
			parts = append(parts, netWarnStyle().Render("machine config unreadable"))
		default:
			parts = append(parts, dimStyle.Render("loading machine config…"))
		}
	}
	if p.net.loading && p.net.ready {
		parts = append(parts, dimStyle.Render("reloading…"))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

type netKV struct{ k, v string }

// kvLines renders label/value rows, wrapping values under the value column.
func kvLines(kvs []netKV, w int) []string {
	var out []string
	vw := max(8, w-netLabelW-1)
	for _, kv := range kvs {
		chunks := wordWrap(kv.v, vw)
		for i, c := range chunks {
			label := ""
			if i == 0 {
				label = kv.k
			}
			out = append(out, dimStyle.Render(fit(label, netLabelW))+" "+c)
		}
	}
	return out
}

func refText(r netmodel.Ref) string {
	if r.Namespace != "" {
		return r.Display() + " " + r.ID + dimStyle.Render("  ("+r.Namespace+")")
	}
	return r.Display() + " " + r.ID
}

func (app App) netDetail(p pane, n netNode, w int) []string {
	m := p.net.model
	link, _ := m.Link(n.link)
	var title string
	var kvs []netKV
	var noteKeys []string
	add := func(k, v string) {
		if v != "" {
			kvs = append(kvs, netKV{k, v})
		}
	}
	switch n.kind {
	case netLink, netRef:
		title = link.Name
		kind := "physical NIC"
		switch {
		case link.Kind != "":
			kind = link.Kind
		case !link.Physical:
			kind = link.Type
		}
		add("kind", kind)
		for _, kv := range link.Detail {
			add(kv.Key, kv.Value)
		}
		add("state", link.OperState)
		add("mtu", itoaNZ(link.MTU))
		add("hwaddr", link.HWAddr)
		add("alias", link.Alias)
		if link.SpeedMbit > 0 {
			add("speed", fmt.Sprintf("%d Mb/s", link.SpeedMbit))
		}
		add("driver", link.Driver)
		add("bus", link.BusPath)
		if len(link.Lowers) > 0 {
			lab := "built on"
			if link.Kind == "bond" || link.Kind == "bridge" || link.Kind == "vrf" {
				lab = "members"
			}
			add(lab, strings.Join(link.Lowers, ", "))
		}
		for _, c := range link.Configs {
			add("config", cfgLabel(c))
		}
		for _, s := range link.Specs {
			layer := s.Layer
			add("spec", "LinkSpec@"+layer)
		}
		for _, op := range link.Operators {
			x := op.Kind
			if op.VIP != "" {
				x += " " + op.VIP
			}
			add("operator", x)
		}
		add("status", refText(link.Status))
		if link.Name == "kubespan" && m.KubeSpan != nil {
			up := 0
			for _, pr := range m.KubeSpan.Peers {
				if pr.State == "up" {
					up++
				}
			}
			add("peers", fmt.Sprintf("%d, %d up", len(m.KubeSpan.Peers), up))
			add("address", m.KubeSpan.Address)
		}
		noteKeys = []string{"LinkStatus", "LinkSpec"}
	case netAddr:
		a := link.Addresses[n.idx]
		title = a.Prefix
		add("link", link.Name)
		add("family", a.Family)
		add("scope", a.Scope)
		src := a.Source
		if a.VIP {
			src += " (virtual IP)"
		}
		add("source", src)
		if len(a.Layers) > 0 {
			add("layers", strings.Join(a.Layers, " > "))
		}
		if a.Missing {
			add("problem", "a spec asks for this address but the kernel does not have it")
		}
		if a.Config != nil {
			add("config", cfgLabel(*a.Config))
		}
		if a.Spec != nil {
			add("spec", refText(*a.Spec))
		}
		if a.Status != nil {
			add("status", refText(*a.Status))
		}
		noteKeys = []string{"AddressStatus", "AddressSpec"}
	case netRoute, netOrphan:
		var r netmodel.Route
		if n.kind == netRoute {
			r = link.Routes[n.idx]
		} else {
			r = m.OrphanRoutes[n.idx]
		}
		title = r.Dst
		add("link", r.OutLink)
		if n.kind == netOrphan {
			add("problem", "this route leaves through "+r.OutLink+", but the node has no such link")
		}
		add("gateway", r.Gateway)
		add("src", r.Src)
		add("family", r.Family)
		add("table", r.Table)
		add("priority", itoaNZ(r.Priority))
		add("protocol", r.Protocol)
		add("scope", r.Scope)
		add("type", r.Type)
		add("source", r.Source)
		if r.Config != nil {
			add("config", cfgLabel(*r.Config))
		}
		if r.Spec != nil {
			add("spec", refText(*r.Spec))
		}
		if r.Status != nil {
			add("status", refText(*r.Status))
		}
		noteKeys = []string{"RouteStatus", "RouteSpec"}
	case netOp:
		op := link.Operators[n.idx]
		title = op.Kind + " on " + link.Name
		add("link", link.Name)
		add("vip", op.VIP)
		add("layer", op.Layer)
		add("problem", "the operator is running but has produced no address yet")
		if op.Config != nil {
			add("config", cfgLabel(*op.Config))
		}
		add("spec", refText(op.Spec))
		noteKeys = []string{"OperatorSpec"}
	case netWarn:
		wn := m.Warnings[n.idx]
		title = wn.Name
		add("problem", wn.Message)
		add("config", cfgLabel(wn.Config))
		add("fix", "check the name against the links on the left; the document is applied but nothing matches")
		noteKeys = []string{wn.Config.Kind}
	case netNote:
		title = "unreadable types"
		for _, t := range p.net.res.Denied {
			add("denied", t+" (needs os:admin)")
		}
		for t, e := range p.net.res.Failed {
			add("failed", t+": "+e)
		}
	}

	lines := []string{lipgloss.NewStyle().Bold(true).Render(cutWidth(title, w))}
	lines = append(lines, kvLines(kvs, w)...)
	var notes []string
	for _, k := range noteKeys {
		nt, ok := m.Notes[k]
		if !ok || nt.What == "" {
			continue
		}
		notes = append(notes, "", dimStyle.Render(k))
		for _, l := range wordWrap(nt.What, max(8, w-1)) {
			notes = append(notes, l)
		}
		if nt.Ubuntu != "" {
			for i, l := range wordWrap("on Ubuntu: "+nt.Ubuntu, max(8, w-1)) {
				if i == 0 {
					l = dimStyle.Render("on Ubuntu:") + strings.TrimPrefix(l, "on Ubuntu:")
				}
				notes = append(notes, l)
			}
		}
	}
	lines = append(lines, notes...)
	keys := app.netKeyHint(p, n)
	if keys != "" {
		lines = append(lines, "", dimStyle.Render(cutWidth(keys, w)))
	}
	return lines
}

func itoaNZ(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

// netKeyHint lists the jumps that apply to the row.
func (app App) netKeyHint(p pane, n netNode) string {
	it := app.netItemOf(p, n)
	var parts []string
	if it.ref != nil {
		parts = append(parts, "↵ YAML")
	}
	parts = append(parts, "d describe", "p related")
	if len(it.cfgs) > 0 {
		parts = append(parts, "c config")
	}
	parts = append(parts, "o diagram")
	return strings.Join(parts, "  ")
}

// networkLines is the pane body.
func (app App) networkLines(p pane, iw, inner int, active bool) []string {
	nv := p.net
	if !nv.ready {
		return messageLines(iw, inner, dimStyle, "loading the network resources of this node…")
	}
	wide, leftW, rightW, treeRows, detailRows := netLayout(iw, inner)
	out := []string{padRight(clipANSI(" "+app.netSummary(p), iw), iw)}

	sel, _, ok := app.netSelected(p)
	var detail []string
	if ok {
		detail = app.netDetail(p, sel.node, max(8, rightW-1))
	}
	tl := app.netTreeLines(p, leftW, treeRows, active)

	if wide {
		sep := dimStyle.Render("│")
		for i := 0; i < treeRows; i++ {
			l := strings.Repeat(" ", leftW)
			if i < len(tl) {
				l = tl[i]
			}
			d := ""
			if i < len(detail) {
				d = " " + detail[i]
			}
			out = append(out, l+sep+padRight(clipANSI(d, rightW), rightW))
		}
		return out
	}
	out = append(out, tl...)
	for len(out) < 1+treeRows {
		out = append(out, "")
	}
	if detailRows > 0 {
		out = append(out, dimStyle.Render(strings.Repeat("─", iw)))
		for i := 0; i < detailRows && i < len(detail); i++ {
			out = append(out, " "+detail[i])
		}
	}
	return out
}

func (app App) networkNextStep(p pane) []string {
	if !p.net.ready {
		return []string{"loading…"}
	}
	f, _, ok := app.netSelected(p)
	if !ok {
		return []string{"esc back"}
	}
	it := app.netItemOf(p, f.node)
	parts := []string{"j/k move", "h/l fold"}
	if it.ref != nil {
		parts = append(parts, "↵ YAML")
	}
	parts = append(parts, "d what is this", "p related")
	if len(it.cfgs) > 0 {
		parts = append(parts, "c config")
	}
	return append(parts, "o diagram")
}
