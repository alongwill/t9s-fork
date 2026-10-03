package netmodel

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/florianspk/t9s/internal/catalog"
)

// layered collects every spec that asks for the same thing: one per layer in
// network-config, plus the merged one in network.
type layered struct {
	layers map[string]Ref // layer -> ref in network-config
	merged *Ref
	top    string // the highest priority layer seen
}

func (l *layered) add(r Res, typ string) {
	ref := Ref{Type: typ, Namespace: r.Namespace, ID: r.ID}
	layer := specLayer(r)
	if r.Namespace == NSNetwork {
		l.merged = &ref
	} else {
		if l.layers == nil {
			l.layers = map[string]Ref{}
		}
		l.layers[layer] = ref
	}
	if layerRank(layer) > layerRank(l.top) {
		l.top = layer
	}
}

// layerNames lists the layers highest priority first.
func (l *layered) layerNames() []string {
	var out []string
	for k := range l.layers {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return layerRank(out[i]) > layerRank(out[j]) })
	return out
}

// specRef is the spec to show for an item: the merged one (what Talos decided),
// else the top layered one.
func (l *layered) specRef() *Ref {
	if l == nil {
		return nil
	}
	if l.merged != nil {
		return l.merged
	}
	if r, ok := l.layers[l.top]; ok {
		return &r
	}
	return nil
}

// Build assembles the model from fetched resources. It never fails: anything it
// cannot place becomes a warning or is left out.
func Build(in Inputs) Model {
	m := Model{Node: in.Node, Children: map[string][]string{}, Also: map[string][]string{}}
	docs := parseDocs(in.Docs)

	// --- links ---
	byName := map[string]*Link{}
	byIndex := map[int]*Link{}
	raw := map[string]map[string]any{}
	links := make([]*Link, 0, len(in.LinkStatuses))
	for _, r := range in.LinkStatuses {
		s := r.Spec
		l := &Link{
			Name: r.ID, Alias: str(s, "alias"), AltNames: strList(s, "altNames"),
			Index: num(s, "index"), Kind: str(s, "kind"), Type: str(s, "type"),
			OperState: str(s, "operationalState"), MTU: num(s, "mtu"), HWAddr: str(s, "hardwareAddr"),
			Driver: str(s, "driver"), SpeedMbit: num(s, "speedMbit"), BusPath: str(s, "busPath"),
			Status: Ref{Type: TypeLinkStatus, Namespace: r.Namespace, ID: r.ID},
		}
		l.Physical = l.Type == "ether" && l.Kind == ""
		l.State = linkState(l.OperState)
		l.Detail = linkDetail(l, s)
		links = append(links, l)
		byName[l.Name] = l
		byIndex[l.Index] = l
		raw[l.Name] = s
	}
	sort.SliceStable(links, func(i, j int) bool { return links[i].Index < links[j].Index })

	// --- hierarchy: lowers[x] are the links x is built on or enslaves ---
	lowers := map[string][]*Link{}
	for _, l := range links {
		s := raw[l.Name]
		if mi := num(s, "masterIndex"); mi != 0 {
			if master := byIndex[mi]; master != nil && master != l {
				lowers[master.Name] = append(lowers[master.Name], l)
			}
		}
		switch l.Kind {
		case "vlan", "macvlan", "vxlan":
			if li := num(s, "linkIndex"); li != 0 {
				if parent := byIndex[li]; parent != nil && parent != l {
					lowers[l.Name] = append(lowers[l.Name], parent)
				}
			}
		}
	}
	for _, l := range links {
		ls := lowers[l.Name]
		sort.SliceStable(ls, func(i, j int) bool { return ls[i].Index < ls[j].Index })
		for _, x := range ls {
			l.Lowers = append(l.Lowers, x.Name)
		}
	}

	// --- link specs ---
	linkSpecs := map[string]*layered{}
	for _, r := range in.LinkSpecs {
		name := str(r.Spec, "name")
		if name == "" {
			_, name = tailAfterLayer(r.ID)
		}
		if linkSpecs[name] == nil {
			linkSpecs[name] = &layered{}
		}
		linkSpecs[name].add(r, TypeLinkSpec)
	}
	for name, ls := range linkSpecs {
		l := byName[name]
		if l == nil {
			continue
		}
		for _, layer := range ls.layerNames() {
			l.Specs = append(l.Specs, SpecRef{Layer: layer, Ref: ls.layers[layer]})
		}
		if ls.merged != nil {
			l.Specs = append(l.Specs, SpecRef{Layer: "merged", Ref: *ls.merged})
		}
	}

	// --- config documents ---
	resolve := func(name string, alias bool) *Link {
		if name == "" {
			return nil
		}
		if !alias {
			if l := byName[name]; l != nil {
				return l
			}
		}
		for _, l := range links {
			if l.Alias == name {
				return l
			}
		}
		if !alias {
			for _, l := range links {
				for _, a := range l.AltNames {
					if a == name {
						return l
					}
				}
			}
		}
		return nil
	}
	docsOn := map[string][]docRef{} // link name -> documents that name it
	seenDoc := map[string]bool{}
	for _, d := range docs {
		key := fmt.Sprintf("%d|%s", d.cfg.DocIndex, d.cfg.Detail)
		if !seenDoc[key] {
			seenDoc[key] = true
			m.Docs = append(m.Docs, d.cfg)
		}
		l := resolve(d.link, d.alias)
		if l == nil && d.parent != "" && d.vlanID != 0 {
			// VLANs are also told apart by parent and VLAN id
			if p := resolve(d.parent, false); p != nil {
				for _, c := range links {
					if c.Kind == "vlan" && containsStr(c.Lowers, p.Name) && vlanID(raw[c.Name]) == d.vlanID {
						l = c
					}
				}
			}
		}
		switch {
		case l != nil:
			docsOn[l.Name] = append(docsOn[l.Name], d)
			l.Configs = appendCfg(l.Configs, d.cfg)
		case d.alias:
			m.Warnings = append(m.Warnings, Warning{Name: d.link, Config: d.cfg,
				Message: fmt.Sprintf("%s: no link on this node has the alias %s", d.cfg.Kind, d.link)})
		case d.vip != "" && d.link != "":
			m.Warnings = append(m.Warnings, Warning{Name: d.link, Config: d.cfg,
				Message: fmt.Sprintf("%s for %s names link %s, which this node does not have", d.cfg.Kind, d.vip, d.link)})
		case d.link != "":
			m.Warnings = append(m.Warnings, Warning{Name: d.link, Config: d.cfg,
				Message: fmt.Sprintf("in %s, no such link on this node", d.cfg.Kind)})
		}
		for _, mem := range d.members {
			if resolve(mem, false) == nil {
				m.Warnings = append(m.Warnings, Warning{Name: mem, Config: d.cfg,
					Message: fmt.Sprintf("%s %s lists %s as a member, no such link on this node", d.cfg.Kind, d.link, mem)})
			}
		}
		if d.parent != "" && resolve(d.parent, false) == nil {
			m.Warnings = append(m.Warnings, Warning{Name: d.parent, Config: d.cfg,
				Message: fmt.Sprintf("%s %s has parent %s, no such link on this node", d.cfg.Kind, d.link, d.parent)})
		}
	}

	// --- operators ---
	type opKey struct{ kind, link, vip string }
	seenOp := map[opKey]bool{}
	for _, r := range in.OperatorSpecs {
		s := r.Spec
		kind := str(s, "operator")
		lname := str(s, "linkName")
		vip := ""
		if kind == "vip" {
			vip = str(sub(s, "vip"), "ip")
		}
		l := byName[lname]
		k := opKey{kind, lname, vip}
		if l == nil || seenOp[k] {
			continue
		}
		seenOp[k] = true
		op := Operator{Kind: kind, VIP: vip, Layer: str(s, "layer"), Spec: Ref{Type: TypeOperatorSpec, Namespace: r.Namespace, ID: r.ID}}
		for _, d := range docsOn[l.Name] {
			if (kind == "dhcp4" && d.dhcp4) || (kind == "dhcp6" && d.dhcp6) || (kind == "vip" && d.vip == vip) {
				c := d.cfg
				op.Config = &c
			}
		}
		l.Operators = append(l.Operators, op)
	}

	// --- addresses ---
	type addrKey struct{ link, prefix string }
	addrSpecs := map[addrKey]*layered{}
	for _, r := range in.AddressSpecs {
		k := addrKey{str(r.Spec, "linkName"), str(r.Spec, "address")}
		if addrSpecs[k] == nil {
			addrSpecs[k] = &layered{}
		}
		addrSpecs[k].add(r, TypeAddrSpec)
	}
	build := func(l *Link, prefix, family, scope string, st *Ref) Address {
		a := Address{Prefix: prefix, Family: family, Scope: scope, Status: st}
		sp := addrSpecs[addrKey{l.Name, prefix}]
		top := ""
		if sp != nil {
			a.Layers, a.Spec, top = sp.layerNames(), sp.specRef(), sp.top
			if len(a.Layers) == 0 && top != "" {
				a.Layers = []string{top}
			}
		}
		a.Source, a.VIP, a.Config = addressSource(l, prefix, family, top, docsOn[l.Name])
		return a
	}
	seenAddr := map[addrKey]bool{}
	for _, r := range in.AddressStatuses {
		s := r.Spec
		l := byName[str(s, "linkName")]
		if l == nil {
			l = byIndex[num(s, "linkIndex")]
		}
		if l == nil {
			continue
		}
		prefix := str(s, "address")
		seenAddr[addrKey{l.Name, prefix}] = true
		st := Ref{Type: TypeAddrStatus, Namespace: r.Namespace, ID: r.ID}
		l.Addresses = append(l.Addresses, build(l, prefix, str(s, "family"), str(s, "scope"), &st))
	}
	for k := range addrSpecs { // asked for, but the kernel does not have it
		l := byName[k.link]
		if l == nil || seenAddr[k] {
			continue
		}
		a := build(l, k.prefix, "", "", nil)
		a.Missing = true
		l.Addresses = append(l.Addresses, a)
	}
	for _, l := range links {
		sort.SliceStable(l.Addresses, func(i, j int) bool { return addrLess(l.Addresses[i], l.Addresses[j]) })
	}

	// --- routes ---
	routeSpecs := map[string]*layered{}
	for _, r := range in.RouteSpecs {
		k := routeKey(normDst(str(r.Spec, "dst"), str(r.Spec, "gateway")), str(r.Spec, "gateway"), str(r.Spec, "outLinkName"), str(r.Spec, "table"), num(r.Spec, "priority"))
		if routeSpecs[k] == nil {
			routeSpecs[k] = &layered{}
		}
		routeSpecs[k].add(r, TypeRouteSpec)
	}
	for _, r := range in.RouteStatuses {
		s := r.Spec
		table, typ := str(s, "table"), str(s, "type")
		dst := normDst(str(s, "dst"), str(s, "gateway"))
		if table != "main" || typ == "broadcast" || typ == "local" || typ == "multicast" || typ == "anycast" || strings.HasPrefix(dst, "fe80::/") {
			m.HiddenRoutes++
			continue
		}
		rt := Route{
			Dst: dst, Gateway: str(s, "gateway"), Src: str(s, "src"), Family: str(s, "family"), Table: table,
			Priority: num(s, "priority"), Protocol: str(s, "protocol"), Scope: str(s, "scope"), Type: typ,
			OutLink: str(s, "outLinkName"), Source: SrcKernel, Default: dst == "default",
			Status: &Ref{Type: TypeRouteStatus, Namespace: r.Namespace, ID: r.ID},
		}
		l := byName[rt.OutLink]
		if l == nil && rt.OutLink == "" {
			l = byIndex[num(s, "outLinkIndex")]
			if l != nil {
				rt.OutLink = l.Name
			}
		}
		if sp := routeSpecs[routeKey(dst, rt.Gateway, rt.OutLink, table, rt.Priority)]; sp != nil {
			rt.Spec = sp.specRef()
			rt.Source = routeSource(sp.top)
		}
		if l != nil {
			for _, d := range docsOn[l.Name] {
				for _, rc := range d.routes {
					if normDst(rc.dst, rc.gateway) == dst && (rc.gateway == "" || rc.gateway == rt.Gateway) {
						c := d.cfg
						rt.Config = &c
					}
				}
			}
		}
		if rt.Default {
			m.DefaultRoutes = append(m.DefaultRoutes, rt)
		}
		if l == nil {
			m.OrphanRoutes = append(m.OrphanRoutes, rt)
			continue
		}
		l.Routes = append(l.Routes, rt)
	}
	for _, l := range links {
		sort.SliceStable(l.Routes, func(i, j int) bool { return routeLess(l.Routes[i], l.Routes[j]) })
	}

	// --- node-wide ---
	if len(in.Hostnames) > 0 {
		m.Hostname, m.Domain = str(in.Hostnames[0].Spec, "hostname"), str(in.Hostnames[0].Spec, "domainname")
	}
	if len(in.Resolvers) > 0 {
		m.Resolvers = strList(in.Resolvers[0].Spec, "dnsServers")
	}
	for _, r := range in.NodeAddresses {
		if r.ID == "default" || (len(m.NodeAddresses) == 0 && len(in.NodeAddresses) == 1) {
			m.NodeAddresses = strList(r.Spec, "addresses")
		}
	}
	m.KubeSpan = buildKubeSpan(in)

	// --- forest ---
	for _, l := range links {
		m.Links = append(m.Links, *l)
	}
	for _, l := range m.Links {
		if len(l.Lowers) == 0 {
			m.Roots = append(m.Roots, l.Name)
			continue
		}
		m.Children[l.Lowers[0]] = append(m.Children[l.Lowers[0]], l.Name)
		for _, x := range l.Lowers[1:] {
			m.Also[x] = append(m.Also[x], l.Name)
		}
	}
	sort.SliceStable(m.Roots, func(i, j int) bool {
		a, b := byName[m.Roots[i]], byName[m.Roots[j]]
		if a.Physical != b.Physical {
			return a.Physical
		}
		return a.Index < b.Index
	})

	m.Services = buildServices(m)
	m.Notes = buildNotes(m)
	return m
}

func linkState(oper string) string {
	switch strings.ToLower(oper) {
	case "up":
		return "up"
	case "", "unknown":
		return "unknown"
	}
	return "down"
}

func vlanID(spec map[string]any) int { return num(sub(spec, "vlan"), "vlanID") }

func linkDetail(l *Link, s map[string]any) []KV {
	var kv []KV
	add := func(k, v string) {
		if v != "" && v != "0" {
			kv = append(kv, KV{k, v})
		}
	}
	switch l.Kind {
	case "bond":
		b := sub(s, "bondMaster")
		add("bond mode", str(b, "mode"))
		add("hash policy", str(b, "xmitHashPolicy"))
		add("lacp rate", str(b, "lacpRate"))
		add("miimon", str(b, "miimon"))
	case "vlan":
		v := sub(s, "vlan")
		add("vlan id", str(v, "vlanID"))
		add("vlan protocol", str(v, "protocol"))
	case "bridge":
		b := sub(s, "bridgeMaster")
		if flag(sub(b, "stp"), "enabled") {
			add("stp", "enabled")
		}
		if flag(sub(b, "vlan"), "filtering") {
			add("vlan filtering", "on")
		}
	case "wireguard":
		w := sub(s, "wireguard")
		add("listen port", str(w, "listenPort"))
		if n := len(list(w, "peers")); n > 0 {
			add("peers", fmt.Sprint(n))
		}
	case "vxlan":
		add("vni", str(sub(s, "vxlan"), "id"))
	case "macvlan":
		add("macvlan mode", str(sub(s, "macvlan"), "mode"))
	}
	return kv
}

// addressSource decides where an address came from. top is the highest layer
// with an AddressSpec for it ("" when none does).
func addressSource(l *Link, prefix, family, top string, docs []docRef) (src string, vip bool, cfg *ConfigRef) {
	ip := prefix
	if p, err := netip.ParsePrefix(prefix); err == nil {
		ip = p.Addr().String()
	}
	pick := func(match func(d docRef) bool) *ConfigRef {
		for _, d := range docs {
			if match(d) {
				c := d.cfg
				return &c
			}
		}
		return nil
	}
	for _, op := range l.Operators {
		if op.Kind == "vip" && op.VIP == ip {
			vip = true
		}
	}
	vipDoc := pick(func(d docRef) bool { return d.vip == ip })
	if vipDoc != nil {
		vip = true
	}
	switch {
	case vip:
		cfg = vipDoc
		return SrcVIP, true, cfg
	case top == "":
		return SrcKernel, false, nil
	case top == "configuration":
		return SrcStatic, false, pick(func(d docRef) bool {
			for _, a := range d.addrs {
				if samePrefix(a, prefix) {
					return true
				}
			}
			return false
		})
	case top == "operator":
		for _, op := range l.Operators {
			if (op.Kind == "dhcp4" && (family == "inet4" || strings.Contains(prefix, "."))) ||
				(op.Kind == "dhcp6" && (family == "inet6" || strings.Contains(prefix, ":"))) {
				return op.Kind, false, op.Config
			}
		}
		return SrcOperator, false, nil
	}
	return top, false, nil
}

func routeSource(top string) string {
	switch top {
	case "configuration":
		return SrcStatic
	case "operator":
		return SrcOperator
	case "":
		return SrcKernel
	}
	return top
}

// normDst names the default route: an empty or /0 destination.
func normDst(dst, gateway string) string {
	switch {
	case dst == "" && gateway != "":
		return "default"
	case dst == "0.0.0.0/0" || dst == "::/0":
		return "default"
	}
	return dst
}

func routeKey(dst, gw, link, table string, prio int) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d", dst, gw, link, table, prio)
}

func addrLess(a, b Address) bool {
	if a.Family != b.Family {
		return a.Family < b.Family
	}
	if a.Missing != b.Missing {
		return !a.Missing
	}
	pa, ea := netip.ParsePrefix(a.Prefix)
	pb, eb := netip.ParsePrefix(b.Prefix)
	if ea == nil && eb == nil {
		if c := pa.Addr().Compare(pb.Addr()); c != 0 {
			return c < 0
		}
		return pa.Bits() < pb.Bits()
	}
	return a.Prefix < b.Prefix
}

func routeLess(a, b Route) bool {
	if a.Default != b.Default {
		return a.Default
	}
	if a.Dst != b.Dst {
		return a.Dst < b.Dst
	}
	return a.Priority < b.Priority
}

func containsStr(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

func appendCfg(cs []ConfigRef, c ConfigRef) []ConfigRef {
	for _, x := range cs {
		if x.DocIndex == c.DocIndex && x.Detail == c.Detail {
			return cs
		}
	}
	return append(cs, c)
}

func buildKubeSpan(in Inputs) *KubeSpan {
	if len(in.KubeSpanIdentities) == 0 && len(in.KubeSpanPeers) == 0 {
		return nil
	}
	ks := &KubeSpan{}
	if len(in.KubeSpanIdentities) > 0 {
		s := in.KubeSpanIdentities[0].Spec
		ks.Address, ks.Subnet, ks.PublicKey = str(s, "address"), str(s, "subnet"), str(s, "publicKey")
	}
	for _, r := range in.KubeSpanPeers {
		ks.Peers = append(ks.Peers, Peer{
			Label: str(r.Spec, "label"), State: str(r.Spec, "state"), Endpoint: str(r.Spec, "endpoint"),
			Ref: Ref{Type: TypeKubeSpanPeer, Namespace: r.Namespace, ID: r.ID},
		})
	}
	sort.SliceStable(ks.Peers, func(i, j int) bool { return ks.Peers[i].Label < ks.Peers[j].Label })
	return ks
}

func buildServices(m Model) []Service {
	var out []Service
	for _, l := range m.Links {
		for _, a := range l.Addresses {
			if a.VIP {
				out = append(out, Service{Kind: "vip", Name: a.Prefix, Link: l.Name, Config: a.Config, Ref: a.Status})
			}
		}
	}
	if ks := m.KubeSpan; ks != nil {
		up := 0
		for _, p := range ks.Peers {
			if p.State == "up" {
				up++
			}
		}
		svc := Service{Kind: "kubespan", Name: "KubeSpan", Detail: fmt.Sprintf("%d peers, %d up", len(ks.Peers), up)}
		if ks.Address != "" {
			svc.Detail += ", address " + ks.Address
		}
		if _, ok := m.Link("kubespan"); ok {
			svc.Link = "kubespan"
		}
		out = append(out, svc)
	}
	if len(m.NodeAddresses) > 0 {
		out = append(out, Service{Kind: "node-address", Name: "node address", Detail: strings.Join(m.NodeAddresses, ", ")})
	}
	return out
}

// buildNotes gathers the PR A notes for the types the model drew from and the
// config kinds of its documents.
func buildNotes(m Model) map[string]NoteText {
	out := map[string]NoteText{}
	for _, t := range []string{TypeLinkStatus, TypeLinkSpec, TypeAddrStatus, TypeAddrSpec, TypeRouteStatus,
		TypeRouteSpec, TypeOperatorSpec, TypeHostname, TypeResolver, TypeNodeAddress, TypeKubeSpanPeer, TypeKubeSpanIdent} {
		d := DisplayType(t)
		if n, ok := catalog.NoteFor(d); ok {
			out[d] = NoteText{What: n.What, Ubuntu: n.Ubuntu}
		}
	}
	kinds := map[string]bool{}
	for _, d := range m.Docs {
		kinds[d.Kind] = true
	}
	for _, k := range catalog.ConfigKinds() {
		if kinds[k.Kind] {
			out[k.Kind] = NoteText{What: k.Desc, Ubuntu: k.Ubuntu}
		}
	}
	return out
}
