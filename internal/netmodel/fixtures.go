package netmodel

import (
	"fmt"
	"strings"

	"github.com/florianspk/t9s/internal/talos"
)

// Hand-written inputs for tests, the TUI's render tests and the example HTML
// (hack/netview-example.sh). They follow the shapes Talos prints for the
// resources (pkg/machinery/resources/network) and the config documents
// (pkg/machinery/config/types/network). They are not captured from a cluster.

// FixtureNames lists the fixtures Fixture knows.
func FixtureNames() []string {
	return []string{"single-nic-dhcp", "bond-vlan-vip", "bridge", "config-only"}
}

// Fixture returns the named fixture; it panics on a name FixtureNames lacks.
func Fixture(name string) Inputs {
	switch name {
	case "single-nic-dhcp":
		return fixSingleNIC()
	case "bond-vlan-vip":
		return fixBondVLANVIP()
	case "bridge":
		return fixBridge()
	case "config-only":
		return fixConfigOnly()
	}
	panic("unknown netmodel fixture " + name)
}

// fixRes builds a resource from a spec written with relative indentation.
func fixRes(ns, id, spec string) Res {
	var sb strings.Builder
	for _, l := range strings.Split(strings.Trim(spec, "\n"), "\n") {
		sb.WriteString("  " + l + "\n")
	}
	r, err := ParseRes(fmt.Sprintf("metadata:\n  namespace: %s\n  id: %s\nspec:\n%s", ns, id, sb.String()), ns)
	if err != nil {
		panic(err)
	}
	return r
}

func fixDocs(stream string) []talos.ConfigDoc {
	docs, err := talos.SplitConfigDocs(stream)
	if err != nil {
		panic(err)
	}
	return docs
}

func fixNIC(id string, index int, speed int, hw string, master int) Res {
	m := ""
	if master != 0 {
		m = fmt.Sprintf("masterIndex: %d\nslaveKind: bond\n", master)
	}
	return fixRes(NSNetwork, id, fmt.Sprintf(`
index: %d
type: ether
kind: ""
hardwareAddr: %s
mtu: 1500
operationalState: up
linkState: true
speedMbit: %d
driver: e1000e
busPath: "0000:00:%02x.0"
pciID: "8086:10d3"
%s`, index, hw, speed, index, m))
}

func fixLo() Res {
	return fixRes(NSNetwork, "lo", `
index: 1
type: loopback
kind: ""
mtu: 65536
operationalState: unknown
linkState: true
`)
}

func fixSingleNIC() Inputs {
	return Inputs{
		Node:         "10.0.0.5",
		LinkStatuses: []Res{fixLo(), fixNIC("eth0", 2, 1000, "52:54:00:aa:bb:01", 0)},
		AddressStatuses: []Res{
			fixRes(NSNetwork, "eth0/10.0.0.5/24", "address: 10.0.0.5/24\nlinkName: eth0\nlinkIndex: 2\nfamily: inet4\nscope: global"),
			fixRes(NSNetwork, "lo/127.0.0.1/8", "address: 127.0.0.1/8\nlinkName: lo\nlinkIndex: 1\nfamily: inet4\nscope: host"),
		},
		AddressSpecs: []Res{
			fixRes(NSConfig, "operator/eth0/10.0.0.5/24", "address: 10.0.0.5/24\nlinkName: eth0\nfamily: inet4\nscope: global\nlayer: operator"),
			fixRes(NSNetwork, "eth0/10.0.0.5/24", "address: 10.0.0.5/24\nlinkName: eth0\nfamily: inet4\nscope: global\nlayer: operator"),
			fixRes(NSConfig, "default/lo/127.0.0.1/8", "address: 127.0.0.1/8\nlinkName: lo\nfamily: inet4\nscope: host\nlayer: default"),
		},
		RouteStatuses: []Res{
			fixRes(NSNetwork, "inet4/10.0.0.1//1024", "family: inet4\ndst: \"\"\ngateway: 10.0.0.1\noutLinkName: eth0\noutLinkIndex: 2\ntable: main\npriority: 1024\nscope: global\ntype: unicast\nprotocol: dhcp"),
			fixRes(NSNetwork, "inet4//10.0.0.0/24/1024", "family: inet4\ndst: 10.0.0.0/24\nsrc: 10.0.0.5\noutLinkName: eth0\noutLinkIndex: 2\ntable: main\npriority: 1024\nscope: link\ntype: unicast\nprotocol: kernel"),
			fixRes(NSNetwork, "local/127.0.0.0/8", "family: inet4\ndst: 127.0.0.0/8\noutLinkName: lo\ntable: local\ntype: local\nprotocol: kernel"),
		},
		RouteSpecs: []Res{
			fixRes(NSConfig, "operator/inet4/10.0.0.1//1024", "family: inet4\ndst: \"\"\ngateway: 10.0.0.1\noutLinkName: eth0\ntable: main\npriority: 1024\nlayer: operator"),
		},
		LinkSpecs: []Res{
			fixRes(NSConfig, "configuration/eth0", "name: eth0\nup: true\nlayer: configuration"),
			fixRes(NSNetwork, "eth0", "name: eth0\nup: true\nlayer: configuration"),
		},
		OperatorSpecs: []Res{
			fixRes(NSNetwork, "dhcp4/eth0", "operator: dhcp4\nlinkName: eth0\nrequireUp: true\nlayer: configuration\ndhcp4:\n  routeMetric: 1024"),
		},
		Hostnames:     []Res{fixRes(NSNetwork, "hostname", "hostname: cp-1\ndomainname: \"\"")},
		Resolvers:     []Res{fixRes(NSNetwork, "resolvers", "dnsServers:\n  - 1.1.1.1\n  - 8.8.8.8")},
		NodeAddresses: []Res{fixRes(NSNetwork, "default", "addresses:\n  - 10.0.0.5/24")},
		Docs: fixDocs(`version: v1alpha1
machine:
  type: controlplane
---
apiVersion: v1alpha1
kind: DHCPv4Config
name: eth0
`),
	}
}

func fixBondVLANVIP() Inputs {
	return Inputs{
		Node: "10.0.0.5",
		LinkStatuses: []Res{
			fixLo(),
			fixNIC("eth0", 2, 1000, "52:54:00:aa:bb:01", 4),
			fixNIC("eth1", 3, 1000, "52:54:00:aa:bb:02", 4),
			fixRes(NSNetwork, "bond0", "index: 4\ntype: ether\nkind: bond\nhardwareAddr: 52:54:00:aa:bb:01\nmtu: 1500\noperationalState: up\nlinkState: true\nbondMaster:\n  mode: 802.3ad\n  xmitHashPolicy: layer3+4\n  lacpRate: fast\n  miimon: 100"),
			fixRes(NSNetwork, "bond0.100", "index: 5\ntype: ether\nkind: vlan\nhardwareAddr: 52:54:00:aa:bb:01\nmtu: 1500\noperationalState: up\nlinkState: true\nlinkIndex: 4\nvlan:\n  vlanID: 100\n  protocol: 802.1q"),
		},
		AddressStatuses: []Res{
			fixRes(NSNetwork, "bond0/10.0.0.5/24", "address: 10.0.0.5/24\nlinkName: bond0\nlinkIndex: 4\nfamily: inet4\nscope: global"),
			fixRes(NSNetwork, "bond0/10.0.0.10/32", "address: 10.0.0.10/32\nlinkName: bond0\nlinkIndex: 4\nfamily: inet4\nscope: global"),
			fixRes(NSNetwork, "bond0.100/192.168.100.5/24", "address: 192.168.100.5/24\nlinkName: bond0.100\nlinkIndex: 5\nfamily: inet4\nscope: global"),
		},
		AddressSpecs: []Res{
			fixRes(NSConfig, "configuration/bond0/10.0.0.5/24", "address: 10.0.0.5/24\nlinkName: bond0\nfamily: inet4\nscope: global\nlayer: configuration"),
			fixRes(NSNetwork, "bond0/10.0.0.5/24", "address: 10.0.0.5/24\nlinkName: bond0\nfamily: inet4\nscope: global\nlayer: configuration"),
			fixRes(NSConfig, "operator/bond0/10.0.0.10/32", "address: 10.0.0.10/32\nlinkName: bond0\nfamily: inet4\nscope: global\nlayer: operator"),
			fixRes(NSConfig, "operator/bond0.100/192.168.100.5/24", "address: 192.168.100.5/24\nlinkName: bond0.100\nfamily: inet4\nscope: global\nlayer: operator"),
		},
		RouteStatuses: []Res{
			fixRes(NSNetwork, "inet4/10.0.0.1//10", "family: inet4\ndst: \"\"\ngateway: 10.0.0.1\noutLinkName: bond0\noutLinkIndex: 4\ntable: main\npriority: 10\nscope: global\ntype: unicast\nprotocol: static"),
			fixRes(NSNetwork, "inet4//192.168.100.0/24/1024", "family: inet4\ndst: 192.168.100.0/24\nsrc: 192.168.100.5\noutLinkName: bond0.100\ntable: main\npriority: 1024\nscope: link\ntype: unicast\nprotocol: kernel"),
		},
		RouteSpecs: []Res{
			fixRes(NSConfig, "configuration/inet4/10.0.0.1//10", "family: inet4\ndst: \"\"\ngateway: 10.0.0.1\noutLinkName: bond0\ntable: main\npriority: 10\nlayer: configuration"),
		},
		LinkSpecs: []Res{
			fixRes(NSConfig, "configuration/bond0", "name: bond0\nkind: bond\nlogical: true\nup: true\nlayer: configuration"),
			fixRes(NSNetwork, "bond0", "name: bond0\nkind: bond\nlogical: true\nup: true\nlayer: configuration"),
			fixRes(NSConfig, "configuration/bond0.100", "name: bond0.100\nkind: vlan\nlogical: true\nparentName: bond0\nup: true\nlayer: configuration"),
		},
		OperatorSpecs: []Res{
			fixRes(NSNetwork, "vip/bond0/10.0.0.10", "operator: vip\nlinkName: bond0\nlayer: configuration\nvip:\n  ip: 10.0.0.10\n  gratuitousARP: true"),
			fixRes(NSNetwork, "dhcp4/bond0.100", "operator: dhcp4\nlinkName: bond0.100\nlayer: configuration\ndhcp4:\n  routeMetric: 2048"),
		},
		Hostnames:     []Res{fixRes(NSNetwork, "hostname", "hostname: cp-1\ndomainname: \"\"")},
		Resolvers:     []Res{fixRes(NSNetwork, "resolvers", "dnsServers:\n  - 1.1.1.1\n  - 8.8.8.8")},
		NodeAddresses: []Res{fixRes(NSNetwork, "default", "addresses:\n  - 10.0.0.5/24\n  - 192.168.100.5/24")},
		Docs: fixDocs(`version: v1alpha1
machine:
  type: controlplane
---
apiVersion: v1alpha1
kind: BondConfig
name: bond0
links:
  - eth0
  - eth1
bondMode: 802.3ad
addresses:
  - address: 10.0.0.5/24
routes:
  - gateway: 10.0.0.1
    metric: 10
---
apiVersion: v1alpha1
kind: VLANConfig
name: bond0.100
vlanID: 100
parent: bond0
---
apiVersion: v1alpha1
kind: DHCPv4Config
name: bond0.100
---
apiVersion: v1alpha1
kind: Layer2VIPConfig
name: 10.0.0.10
link: bond0
---
apiVersion: v1alpha1
kind: LinkConfig
name: eth9
up: true
`),
	}
}

func fixBridge() Inputs {
	return Inputs{
		Node: "10.0.0.6",
		LinkStatuses: []Res{
			fixLo(),
			fixNIC("eth0", 2, 10000, "52:54:00:cc:dd:01", 3),
			fixRes(NSNetwork, "br0", "index: 3\ntype: ether\nkind: bridge\nhardwareAddr: 52:54:00:cc:dd:01\nmtu: 1500\noperationalState: up\nlinkState: true\nbridgeMaster:\n  stp:\n    enabled: true"),
		},
		AddressStatuses: []Res{
			fixRes(NSNetwork, "br0/10.1.0.6/24", "address: 10.1.0.6/24\nlinkName: br0\nlinkIndex: 3\nfamily: inet4\nscope: global"),
		},
		AddressSpecs: []Res{
			fixRes(NSConfig, "configuration/br0/10.1.0.6/24", "address: 10.1.0.6/24\nlinkName: br0\nfamily: inet4\nscope: global\nlayer: configuration"),
		},
		RouteStatuses: []Res{
			fixRes(NSNetwork, "inet4/10.1.0.1//1024", "family: inet4\ndst: \"\"\ngateway: 10.1.0.1\noutLinkName: br0\ntable: main\npriority: 1024\nscope: global\ntype: unicast\nprotocol: static"),
		},
		Hostnames: []Res{fixRes(NSNetwork, "hostname", "hostname: w-1\ndomainname: \"\"")},
		Docs: fixDocs(`version: v1alpha1
machine:
  type: worker
---
apiVersion: v1alpha1
kind: BridgeConfig
name: br0
links:
  - eth0
addresses:
  - address: 10.1.0.6/24
`),
	}
}

// fixConfigOnly has a LinkConfig for a link the node does not have (a typo),
// and a route that leaves through a link that does not exist.
func fixConfigOnly() Inputs {
	return Inputs{
		Node:         "10.0.0.7",
		LinkStatuses: []Res{fixLo(), fixNIC("eth0", 2, 1000, "52:54:00:ee:ff:01", 0)},
		AddressStatuses: []Res{
			fixRes(NSNetwork, "eth0/10.0.0.7/24", "address: 10.0.0.7/24\nlinkName: eth0\nlinkIndex: 2\nfamily: inet4\nscope: global"),
		},
		RouteStatuses: []Res{
			fixRes(NSNetwork, "inet4/10.0.0.1//1024", "family: inet4\ndst: \"\"\ngateway: 10.0.0.1\noutLinkName: eth0\ntable: main\npriority: 1024\nscope: global\ntype: unicast\nprotocol: static"),
			fixRes(NSNetwork, "inet4/172.16.0.1/10.9.0.0/16/1024", "family: inet4\ndst: 10.9.0.0/16\ngateway: 172.16.0.1\noutLinkName: eth7\ntable: main\npriority: 1024\nscope: global\ntype: unicast\nprotocol: static"),
		},
		Docs: fixDocs(`version: v1alpha1
machine:
  type: worker
---
apiVersion: v1alpha1
kind: LinkConfig
name: eth9
up: true
addresses:
  - address: 10.0.9.7/24
`),
	}
}
