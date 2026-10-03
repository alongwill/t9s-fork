package netmodel

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/florianspk/t9s/internal/talos"
)

// A networking config document names a link (and sometimes more). docRef is
// what the model needs from one: which link, what it says about it. Matching
// follows pkg/machinery/config/types/network in Talos: documents carry the
// link in `name:` (LinkConfig, BondConfig, BridgeConfig, VLANConfig, DHCPv4Config…),
// VIP documents carry the VIP in `name:` and the link in `link:`, and
// LinkAliasConfig names an alias, not a link. The legacy v1alpha1 document
// lists `machine.network.interfaces[]` by `interface:`.

type routeCfg struct{ dst, gateway string }

type docRef struct {
	cfg ConfigRef

	link    string   // link the document names (a name or alias; for VLAN also the link it creates)
	alias   bool     // link is an alias (LinkAliasConfig)
	members []string // links enslaved by a bond, bridge or VRF
	parent  string   // lower link of a VLAN or MACVLAN
	vlanID  int
	logical bool // the document declares a logical link (it exists because of the document)

	vip    string // VIP address (Layer2VIPConfig, HCloudVIPConfig, legacy vip)
	addrs  []string
	routes []routeCfg
	dhcp4  bool
	dhcp6  bool
}

// linkDocKinds name a link in `name:`.
var linkDocKinds = map[string]bool{
	"LinkConfig": true, "BondConfig": true, "BridgeConfig": true, "VLANConfig": true,
	"DummyLinkConfig": true, "WireguardConfig": true, "VRFConfig": true, "VXLANConfig": true,
	"VethConfig": true, "MACVLANConfig": true, "EthernetConfig": true,
	"DHCPv4Config": true, "DHCPv6Config": true,
}

// logicalDocKinds create a link of their own.
var logicalDocKinds = map[string]bool{
	"BondConfig": true, "BridgeConfig": true, "VLANConfig": true, "DummyLinkConfig": true,
	"WireguardConfig": true, "VRFConfig": true, "VXLANConfig": true, "VethConfig": true, "MACVLANConfig": true,
}

// IsNetworkDocKind reports whether the model reads documents of this kind.
func IsNetworkDocKind(kind string) bool {
	return linkDocKinds[kind] || kind == "Layer2VIPConfig" || kind == "HCloudVIPConfig" ||
		kind == "LinkAliasConfig" || kind == talos.LegacyConfigKind
}

// parseDocs extracts a docRef for every networking document (and every
// interface of the legacy document). The index is the position in docs.
func parseDocs(docs []talos.ConfigDoc) []docRef {
	var out []docRef
	for i, d := range docs {
		if !IsNetworkDocKind(d.Kind) {
			continue
		}
		var m map[string]any
		if err := yaml.Unmarshal([]byte(d.YAML), &m); err != nil || m == nil {
			continue
		}
		base := ConfigRef{Kind: d.Kind, Name: d.Name, DocIndex: i}
		switch {
		case d.Kind == talos.LegacyConfigKind:
			out = append(out, parseLegacy(m, base)...)
		case d.Kind == "Layer2VIPConfig" || d.Kind == "HCloudVIPConfig":
			out = append(out, docRef{cfg: base, link: str(m, "link"), vip: str(m, "name")})
		case d.Kind == "LinkAliasConfig":
			out = append(out, docRef{cfg: base, link: str(m, "name"), alias: true})
		default:
			r := docRef{cfg: base, link: str(m, "name"), logical: logicalDocKinds[d.Kind]}
			r.members = strList(m, "links")
			r.parent = str(m, "parent")
			r.vlanID = num(m, "vlanID")
			r.dhcp4 = d.Kind == "DHCPv4Config"
			r.dhcp6 = d.Kind == "DHCPv6Config"
			for _, a := range list(m, "addresses") {
				if am, ok := a.(map[string]any); ok {
					r.addrs = append(r.addrs, str(am, "address"))
				}
			}
			for _, rt := range list(m, "routes") {
				if rm, ok := rt.(map[string]any); ok {
					r.routes = append(r.routes, routeCfg{dst: str(rm, "destination"), gateway: str(rm, "gateway")})
				}
			}
			out = append(out, r)
		}
	}
	return out
}

// parseLegacy reads machine.network.interfaces of the v1alpha1 document.
// Interfaces selected by deviceSelector rather than name cannot be matched by
// name and are skipped.
func parseLegacy(m map[string]any, base ConfigRef) []docRef {
	var out []docRef
	machine := sub(m, "machine")
	network := sub(machine, "network")
	for _, x := range list(network, "interfaces") {
		im, ok := x.(map[string]any)
		if !ok {
			continue
		}
		name := str(im, "interface")
		if name == "" {
			continue
		}
		ref := base
		ref.Detail = fmt.Sprintf("machine.network.interfaces[%s]", name)
		r := legacyIface(im, ref, name)
		if b := sub(im, "bond"); b != nil {
			r.members = strList(b, "interfaces")
			r.logical = true
		}
		if b := sub(im, "bridge"); b != nil {
			r.members = strList(b, "interfaces")
			r.logical = true
		}
		out = append(out, r)
		for _, v := range list(im, "vlans") {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			id := num(vm, "vlanId")
			vname := fmt.Sprintf("%s.%d", name, id)
			vref := base
			vref.Detail = fmt.Sprintf("machine.network.interfaces[%s].vlans[%d]", name, id)
			vr := legacyIface(vm, vref, vname)
			vr.parent, vr.vlanID, vr.logical = name, id, true
			out = append(out, vr)
		}
	}
	return out
}

func legacyIface(m map[string]any, ref ConfigRef, name string) docRef {
	r := docRef{cfg: ref, link: name}
	r.addrs = strList(m, "addresses")
	r.dhcp4 = boolOf(m, "dhcp")
	if v := sub(m, "vip"); v != nil {
		r.vip = str(v, "ip")
	}
	for _, rt := range list(m, "routes") {
		if rm, ok := rt.(map[string]any); ok {
			r.routes = append(r.routes, routeCfg{dst: str(rm, "network"), gateway: str(rm, "gateway")})
		}
	}
	return r
}

// matchesLink reports whether the document names this link: by name, by alias
// (LinkAliasConfig, or a name that is the link's alias), or by altName.
func (d docRef) matchesLink(l *Link) bool {
	if d.link == "" {
		return false
	}
	if d.alias {
		return l.Alias == d.link
	}
	if d.link == l.Name || (l.Alias != "" && d.link == l.Alias) {
		return true
	}
	for _, a := range l.AltNames {
		if d.link == a {
			return true
		}
	}
	return false
}

func samePrefix(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }
