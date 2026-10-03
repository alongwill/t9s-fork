package netmodel

import (
	"strings"
	"testing"
)

func link(t *testing.T, m Model, name string) Link {
	t.Helper()
	l, ok := m.Link(name)
	if !ok {
		t.Fatalf("no link %q in model", name)
	}
	return l
}

func addr(t *testing.T, l Link, prefix string) Address {
	t.Helper()
	for _, a := range l.Addresses {
		if a.Prefix == prefix {
			return a
		}
	}
	t.Fatalf("link %s has no address %s: %+v", l.Name, prefix, l.Addresses)
	return Address{}
}

func TestSingleNICDHCP(t *testing.T) {
	m := Build(Fixture("single-nic-dhcp"))
	if got := strings.Join(m.Roots, ","); got != "eth0,lo" {
		t.Fatalf("roots = %s, want physical NIC first: eth0,lo", got)
	}
	eth0 := link(t, m, "eth0")
	if !eth0.Physical || eth0.State != "up" || eth0.Driver != "e1000e" || eth0.SpeedMbit != 1000 {
		t.Fatalf("eth0 = %+v", eth0)
	}
	a := addr(t, eth0, "10.0.0.5/24")
	if a.Source != SrcDHCP4 {
		t.Fatalf("address source = %q, want dhcp4", a.Source)
	}
	if a.Config == nil || a.Config.Kind != "DHCPv4Config" {
		t.Fatalf("address config = %+v, want the DHCPv4Config", a.Config)
	}
	if len(eth0.Operators) != 1 || eth0.Operators[0].Kind != "dhcp4" || eth0.Operators[0].Config == nil {
		t.Fatalf("operators = %+v", eth0.Operators)
	}
	if got := link(t, m, "lo"); addr(t, got, "127.0.0.1/8").Source != SrcDefault {
		t.Fatalf("lo address source = %q, want default", addr(t, got, "127.0.0.1/8").Source)
	}
	if len(m.DefaultRoutes) != 1 || m.DefaultRoutes[0].Gateway != "10.0.0.1" || m.DefaultRoutes[0].OutLink != "eth0" {
		t.Fatalf("default routes = %+v", m.DefaultRoutes)
	}
	if len(eth0.Routes) != 2 || !eth0.Routes[0].Default || eth0.Routes[0].Dst != "default" {
		t.Fatalf("eth0 routes = %+v, want default first then the link route", eth0.Routes)
	}
	if eth0.Routes[0].Source != SrcOperator {
		t.Fatalf("default route source = %q, want operator (DHCP)", eth0.Routes[0].Source)
	}
	if m.HiddenRoutes != 1 {
		t.Fatalf("hidden routes = %d, want the one local-table route", m.HiddenRoutes)
	}
	if m.Hostname != "cp-1" || strings.Join(m.Resolvers, ",") != "1.1.1.1,8.8.8.8" || strings.Join(m.NodeAddresses, ",") != "10.0.0.5/24" {
		t.Fatalf("node-wide: %q %v %v", m.Hostname, m.Resolvers, m.NodeAddresses)
	}
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %+v", m.Warnings)
	}
	if len(eth0.Specs) != 2 || eth0.Specs[0].Layer != "configuration" || eth0.Specs[1].Layer != "merged" {
		t.Fatalf("eth0 specs = %+v", eth0.Specs)
	}
}

func TestBondVLANStaticVIP(t *testing.T) {
	m := Build(Fixture("bond-vlan-vip"))

	// hierarchy: the bond hangs under its first member, the VLAN under the bond
	eth0, eth1, bond, vlan := link(t, m, "eth0"), link(t, m, "eth1"), link(t, m, "bond0"), link(t, m, "bond0.100")
	if strings.Join(bond.Lowers, ",") != "eth0,eth1" {
		t.Fatalf("bond lowers = %v", bond.Lowers)
	}
	if strings.Join(vlan.Lowers, ",") != "bond0" {
		t.Fatalf("vlan lowers = %v", vlan.Lowers)
	}
	if strings.Join(m.Roots, ",") != "eth0,eth1,lo" {
		t.Fatalf("roots = %v", m.Roots)
	}
	if strings.Join(m.Children["eth0"], ",") != "bond0" || len(m.Children["eth1"]) != 0 {
		t.Fatalf("children = %v", m.Children)
	}
	if strings.Join(m.Also["eth1"], ",") != "bond0" {
		t.Fatalf("also = %v: the bond must be reachable from its second member", m.Also)
	}
	if strings.Join(m.Children["bond0"], ",") != "bond0.100" {
		t.Fatalf("bond children = %v", m.Children["bond0"])
	}
	if !eth0.Physical || bond.Physical || bond.Kind != "bond" {
		t.Fatalf("physical flags: eth0 %v, bond %v/%q", eth0.Physical, bond.Physical, bond.Kind)
	}
	_ = eth1
	if len(bond.Detail) == 0 || bond.Detail[0].Key != "bond mode" || bond.Detail[0].Value != "802.3ad" {
		t.Fatalf("bond detail = %+v", bond.Detail)
	}
	if len(vlan.Detail) == 0 || vlan.Detail[0].Value != "100" {
		t.Fatalf("vlan detail = %+v", vlan.Detail)
	}

	// addresses and their sources
	static := addr(t, bond, "10.0.0.5/24")
	if static.Source != SrcStatic || static.Config == nil || static.Config.Kind != "BondConfig" {
		t.Fatalf("static address = %+v", static)
	}
	vip := addr(t, bond, "10.0.0.10/32")
	if vip.Source != SrcVIP || !vip.VIP || vip.Config == nil || vip.Config.Kind != "Layer2VIPConfig" {
		t.Fatalf("vip address = %+v", vip)
	}
	dhcp := addr(t, vlan, "192.168.100.5/24")
	if dhcp.Source != SrcDHCP4 || dhcp.Config == nil || dhcp.Config.Kind != "DHCPv4Config" {
		t.Fatalf("vlan address = %+v", dhcp)
	}

	// documents attach to the links they name
	kinds := func(l Link) string {
		var ks []string
		for _, c := range l.Configs {
			ks = append(ks, c.Kind)
		}
		return strings.Join(ks, ",")
	}
	if kinds(bond) != "BondConfig,Layer2VIPConfig" {
		t.Fatalf("bond configs = %s", kinds(bond))
	}
	if kinds(vlan) != "VLANConfig,DHCPv4Config" {
		t.Fatalf("vlan configs = %s", kinds(vlan))
	}

	// default route: static, from the BondConfig
	if len(m.DefaultRoutes) != 1 {
		t.Fatalf("default routes = %+v", m.DefaultRoutes)
	}
	if r := m.DefaultRoutes[0]; r.Source != SrcStatic || r.Config == nil || r.Config.Kind != "BondConfig" || r.OutLink != "bond0" {
		t.Fatalf("default route = %+v", r)
	}

	// the stray LinkConfig is a warning, and it is the only one
	if len(m.Warnings) != 1 || m.Warnings[0].Name != "eth9" || m.Warnings[0].Config.Kind != "LinkConfig" {
		t.Fatalf("warnings = %+v", m.Warnings)
	}

	// services: the VIP and the node address
	var kinds2 []string
	for _, s := range m.Services {
		kinds2 = append(kinds2, s.Kind+":"+s.Name)
	}
	if got := strings.Join(kinds2, "|"); got != "vip:10.0.0.10/32|node-address:node address" {
		t.Fatalf("services = %s", got)
	}
	if _, ok := m.Notes["LinkStatus"]; !ok {
		t.Fatalf("notes lack LinkStatus: %v", m.Notes)
	}
	if n, ok := m.Notes["BondConfig"]; !ok || n.What == "" {
		t.Fatalf("notes lack BondConfig: %v", m.Notes["BondConfig"])
	}
}

func TestBridgeWithMember(t *testing.T) {
	m := Build(Fixture("bridge"))
	if strings.Join(m.Children["eth0"], ",") != "br0" {
		t.Fatalf("children = %v", m.Children)
	}
	br := link(t, m, "br0")
	if len(br.Detail) == 0 || br.Detail[0].Key != "stp" {
		t.Fatalf("bridge detail = %+v", br.Detail)
	}
	if a := addr(t, br, "10.1.0.6/24"); a.Source != SrcStatic || a.Config == nil || a.Config.Kind != "BridgeConfig" {
		t.Fatalf("bridge address = %+v", a)
	}
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %+v", m.Warnings)
	}
}

func TestConfigOnlyLinkAndOrphanRoute(t *testing.T) {
	m := Build(Fixture("config-only"))
	if len(m.Warnings) != 1 {
		t.Fatalf("warnings = %+v, want one for eth9", m.Warnings)
	}
	w := m.Warnings[0]
	if w.Name != "eth9" || w.Config.Kind != "LinkConfig" || !strings.Contains(w.Message, "no such link") {
		t.Fatalf("warning = %+v", w)
	}
	if _, ok := m.Link("eth9"); ok {
		t.Fatal("a link that exists only in config must not become a link")
	}
	if len(m.OrphanRoutes) != 1 || m.OrphanRoutes[0].OutLink != "eth7" || m.OrphanRoutes[0].Dst != "10.9.0.0/16" {
		t.Fatalf("orphan routes = %+v", m.OrphanRoutes)
	}
	if eth0 := link(t, m, "eth0"); len(eth0.Routes) != 1 || !eth0.Routes[0].Default {
		t.Fatalf("eth0 routes = %+v", eth0.Routes)
	}
}

func TestMissingMemberAndParentWarn(t *testing.T) {
	in := Fixture("bond-vlan-vip")
	in.Docs = fixDocs(`version: v1alpha1
machine:
  type: controlplane
---
apiVersion: v1alpha1
kind: BondConfig
name: bond0
links:
  - eth0
  - eth8
---
apiVersion: v1alpha1
kind: VLANConfig
name: bond9.200
vlanID: 200
parent: bond9
`)
	m := Build(in)
	var msgs []string
	for _, w := range m.Warnings {
		msgs = append(msgs, w.Name)
	}
	if got := strings.Join(msgs, ","); got != "eth8,bond9.200,bond9" {
		t.Fatalf("warnings = %v (%+v)", got, m.Warnings)
	}
}

func TestVLANMatchedByParentAndID(t *testing.T) {
	in := Fixture("bond-vlan-vip")
	// the document names the VLAN differently from the kernel link
	in.Docs = fixDocs(`version: v1alpha1
machine:
  type: controlplane
---
apiVersion: v1alpha1
kind: VLANConfig
name: vlan-storage
vlanID: 100
parent: bond0
`)
	m := Build(in)
	if len(m.Warnings) != 0 {
		t.Fatalf("warnings = %+v, the VLAN exists under bond0 with id 100", m.Warnings)
	}
	if v := link(t, m, "bond0.100"); len(v.Configs) != 1 || v.Configs[0].Kind != "VLANConfig" {
		t.Fatalf("vlan configs = %+v", v.Configs)
	}
}

func TestLegacyInterfacesMatch(t *testing.T) {
	in := Fixture("single-nic-dhcp")
	in.Docs = fixDocs(`version: v1alpha1
machine:
  type: controlplane
  network:
    interfaces:
      - interface: eth0
        dhcp: true
      - interface: eth5
        addresses: [10.5.0.1/24]
`)
	m := Build(in)
	eth0 := link(t, m, "eth0")
	if len(eth0.Configs) != 1 || eth0.Configs[0].Kind != "v1alpha1" || eth0.Configs[0].Detail != "machine.network.interfaces[eth0]" {
		t.Fatalf("eth0 configs = %+v", eth0.Configs)
	}
	if a := addr(t, eth0, "10.0.0.5/24"); a.Source != SrcDHCP4 || a.Config == nil || a.Config.Kind != "v1alpha1" {
		t.Fatalf("address = %+v", a)
	}
	if len(m.Warnings) != 1 || m.Warnings[0].Name != "eth5" {
		t.Fatalf("warnings = %+v", m.Warnings)
	}
}

func TestMissingAddressIsFlagged(t *testing.T) {
	in := Fixture("single-nic-dhcp")
	in.AddressSpecs = append(in.AddressSpecs,
		fixRes(NSConfig, "configuration/eth0/10.0.0.99/24", "address: 10.0.0.99/24\nlinkName: eth0\nfamily: inet4\nscope: global\nlayer: configuration"))
	m := Build(in)
	a := addr(t, link(t, m, "eth0"), "10.0.0.99/24")
	if !a.Missing || a.Source != SrcStatic || a.Status != nil {
		t.Fatalf("address = %+v", a)
	}
}

func TestParseResAndHelpers(t *testing.T) {
	r, err := ParseRes("node: x\nmetadata:\n  namespace: network\n  id: eth0\nspec:\n  mtu: 1500\n  up: true\n", "")
	if err != nil || r.ID != "eth0" || r.Namespace != "network" || num(r.Spec, "mtu") != 1500 || !flag(r.Spec, "up") {
		t.Fatalf("ParseRes = %+v, %v", r, err)
	}
	for in, want := range map[string]string{
		"LinkStatuses.net.talos.dev":              "LinkStatus",
		"AddressSpecs.net.talos.dev":              "AddressSpec",
		"NodeAddresses.net.talos.dev":             "NodeAddress",
		"KubeSpanPeerStatuses.kubespan.talos.dev": "KubeSpanPeerStatus",
	} {
		if got := DisplayType(in); got != want {
			t.Errorf("DisplayType(%q) = %q, want %q", in, got, want)
		}
	}
	if l, rest := tailAfterLayer("operator/eth0/10.0.0.5/24"); l != "operator" || rest != "eth0/10.0.0.5/24" {
		t.Errorf("tailAfterLayer = %q %q", l, rest)
	}
	if l, rest := tailAfterLayer("eth0/10.0.0.5/24"); l != "" || rest != "eth0/10.0.0.5/24" {
		t.Errorf("tailAfterLayer without layer = %q %q", l, rest)
	}
}
