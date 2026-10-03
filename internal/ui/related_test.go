package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/florianspk/t9s/internal/talos"
)

func TestStemOf(t *testing.T) {
	for in, want := range map[string]string{
		"LinkStatus":      "Link",
		"LinkSpec":        "Link",
		"LinkConfig":      "Link",
		"AddressSpec":     "Address",
		"AddressStatus":   "Address",
		"LinkAliasConfig": "LinkAlias", // not part of the Link family
		"LinkAliasSpec":   "LinkAlias",
		"Status":          "Status", // nothing left to strip
		"MachineConfig":   "Machine",
		"Node":            "Node",
		"SomethingInfo":   "Something",
		"CertRequest":     "Cert",
		"LinkStatuses":    "Link",
	} {
		if got := stemOf(in); got != want {
			t.Errorf("stemOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRoleOf(t *testing.T) {
	for in, want := range map[string]relRole{
		"LinkConfig": roleConfig, "LinkSpec": roleSpec, "LinkStatus": roleStatus,
		"Node": roleOther, "CertRequest": roleSpec, "MachineConfig": roleConfig,
	} {
		if got := roleOf(in); got != want {
			t.Errorf("roleOf(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDisplayFromType(t *testing.T) {
	for in, want := range map[string]string{
		"LinkStatuses.net.talos.dev":   "LinkStatus",
		"AddressSpecs.net.talos.dev":   "AddressSpec",
		"NodeAddresses.net.talos.dev":  "NodeAddress",
		"Hostnames.net.talos.dev":      "Hostname",
		"MachineConfigs.config.talos":  "MachineConfig",
		"RoutingPolicies.net.talos.de": "RoutingPolicy",
	} {
		if got := displayFromType(in); got != want {
			t.Errorf("displayFromType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitLayered(t *testing.T) {
	for _, c := range []struct {
		id, layer, key string
		ok             bool
	}{
		{"configuration/eth0", "configuration", "eth0", true},
		{"operator/eth0", "operator", "eth0", true},
		{"platform/eth0/10.0.0.2/24", "platform", "eth0/10.0.0.2/24", true},
		{"eth0", "", "eth0", false},
		{"eth0/10.0.0.2/24", "", "eth0/10.0.0.2/24", false}, // merged address id: eth0 is not a layer
	} {
		layer, key, ok := splitLayered(c.id)
		if layer != c.layer || key != c.key || ok != c.ok {
			t.Errorf("splitLayered(%q) = %q %q %v", c.id, layer, key, ok)
		}
	}
}

func relDefs() (spec, status, other talos.ResourceDef) {
	spec = talos.ResourceDef{Type: "LinkSpecs.net.talos.dev", DisplayType: "LinkSpec", DefaultNamespace: "network"}
	status = talos.ResourceDef{Type: "LinkStatuses.net.talos.dev", DisplayType: "LinkStatus", DefaultNamespace: "network"}
	other = talos.ResourceDef{Type: "LinkRefreshes.net.talos.dev", DisplayType: "LinkRefresh", DefaultNamespace: "network"}
	return
}

func metas(ns, typ string, ids ...string) []talos.ResourceMeta {
	var out []talos.ResourceMeta
	for _, id := range ids {
		out = append(out, talos.ResourceMeta{Namespace: ns, Type: typ, ID: id})
	}
	return out
}

func TestRelFamilyUsesExactStem(t *testing.T) {
	spec, status, _ := relDefs()
	alias := talos.ResourceDef{Type: "LinkAliasSpecs.net.talos.dev", DisplayType: "LinkAliasSpec", DefaultNamespace: "network"}
	b := browser{defs: []talos.ResourceDef{status, alias, spec}}
	var names []string
	for _, m := range b.relFamily("Link") {
		names = append(names, m.name)
	}
	want := []string{"LinkConfig", "LinkSpec", "LinkStatus"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("family = %v, want %v (LinkAliasConfig/LinkAliasSpec must not join)", names, want)
	}
}

func TestRelTableJoinsLayeredIDs(t *testing.T) {
	spec, status, _ := relDefs()
	b := browser{defs: []talos.ResourceDef{spec, status}}
	members := b.relFamily("Link")
	lists := map[string]relList{
		relKey("network-config", spec.Type): {items: metas("network-config", spec.Type,
			"configuration/eth0", "operator/eth0", "platform/eth1", "default/lo")},
		relKey("network", spec.Type):   {items: metas("network", spec.Type, "eth0", "eth1", "lo")},
		relKey("network", status.Type): {items: metas("network", status.Type, "eth0", "eth1", "lo", "eth9")},
	}
	docs := []talos.ConfigDoc{{Kind: "LinkConfig", Name: "eth0"}, {Kind: "BondConfig", Name: "bond0"}}
	tb := buildRelTable(members, lists, docs, cfgLoaded)

	var titles []string
	for _, c := range tb.cols {
		titles = append(titles, c.title)
	}
	wantCols := []string{"LinkConfig", "LinkSpec@configuration", "LinkSpec@operator", "LinkSpec@platform", "LinkSpec@default", "LinkSpec", "LinkStatus"}
	if !reflect.DeepEqual(titles, wantCols) {
		t.Fatalf("columns = %v\nwant      %v", titles, wantCols)
	}
	if !reflect.DeepEqual(tb.rows, []string{"eth0", "eth1", "eth9", "lo"}) {
		t.Fatalf("rows = %v", tb.rows)
	}
	present := func(row string) string {
		var sb strings.Builder
		for r, k := range tb.rows {
			if k != row {
				continue
			}
			for _, c := range tb.cells[r] {
				if c.state == cellPresent {
					sb.WriteByte('x')
				} else {
					sb.WriteByte('.')
				}
			}
		}
		return sb.String()
	}
	for row, want := range map[string]string{
		"eth0": "xxx..xx", // config doc, configuration + operator layers, merged, status
		"eth1": "...x.xx",
		"eth9": "......x", // only the kernel knows it
		"lo":   "....xxx",
	} {
		if got := present(row); got != want {
			t.Errorf("%s: %s, want %s", row, got, want)
		}
	}
	// the layered cell keeps its real (prefixed) ID so its YAML can be fetched
	if got := tb.cells[0][1].meta; got.ID != "configuration/eth0" || got.Namespace != "network-config" {
		t.Errorf("layered cell meta = %+v", got)
	}
}

func TestRelTableStates(t *testing.T) {
	spec, status, _ := relDefs()
	b := browser{defs: []talos.ResourceDef{spec, status}}
	lists := map[string]relList{
		relKey("network", spec.Type): {locked: true},
	}
	tb := buildRelTable(b.relFamily("Link"), lists, nil, cfgDenied)
	if len(tb.rows) != 0 {
		t.Fatalf("rows = %v", tb.rows)
	}
	// no rows, but the columns are there and typed
	if len(tb.cols) != 3 {
		t.Fatalf("cols = %d", len(tb.cols))
	}
}

func TestNormalizeRelatedYAMLKeepsIdentityOnly(t *testing.T) {
	in := "node: 10.0.0.1\nmetadata:\n  namespace: network\n  type: LinkSpecs.net.talos.dev\n  id: eth0\n  version: 4\n  owner: x\n  phase: running\n  created: a\nspec:\n  up: true\n"
	got := normalizeRelatedYAML(in)
	for _, bad := range []string{"node:", "version", "owner", "phase", "created"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q survived:\n%s", bad, got)
		}
	}
	for _, good := range []string{"id: eth0", "namespace: network", "type: LinkSpecs.net.talos.dev", "up: true"} {
		if !strings.Contains(got, good) {
			t.Errorf("%q lost:\n%s", good, got)
		}
	}
}
