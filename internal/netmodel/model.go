// Package netmodel turns a node's network resources (LinkStatus, AddressStatus,
// RouteStatus, the specs behind them, operators, and the networking documents of
// the machine config) into one picture: links from the physical NIC up, with
// the addresses, routes, operators and config documents attached to each.
//
// Build is pure: it takes already fetched resources (Inputs). Fetch gathers them
// through a talos.ResourceSource. The TUI tree and the HTML diagram both draw
// from Model, so the two cannot disagree.
package netmodel

// Resource types and namespaces the model reads.
const (
	NSNetwork = "network"
	NSConfig  = "network-config"
	NSKube    = "kubespan"

	TypeLinkStatus    = "LinkStatuses.net.talos.dev"
	TypeAddrStatus    = "AddressStatuses.net.talos.dev"
	TypeRouteStatus   = "RouteStatuses.net.talos.dev"
	TypeLinkSpec      = "LinkSpecs.net.talos.dev"
	TypeAddrSpec      = "AddressSpecs.net.talos.dev"
	TypeRouteSpec     = "RouteSpecs.net.talos.dev"
	TypeOperatorSpec  = "OperatorSpecs.net.talos.dev"
	TypeHostname      = "HostnameStatuses.net.talos.dev"
	TypeResolver      = "ResolverStatuses.net.talos.dev"
	TypeNodeAddress   = "NodeAddresses.net.talos.dev"
	TypeKubeSpanPeer  = "KubeSpanPeerStatuses.kubespan.talos.dev"
	TypeKubeSpanIdent = "KubeSpanIdentities.kubespan.talos.dev"
)

// Ref points at one Talos resource, so the UI can open its YAML.
type Ref struct {
	Type      string `json:"type"`
	Namespace string `json:"namespace"`
	ID        string `json:"id"`
}

// Display is the short type name ("LinkStatuses.net.talos.dev" -> "LinkStatus").
func (r Ref) Display() string { return DisplayType(r.Type) }

// ConfigRef points at the machine config document that asked for something.
type ConfigRef struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`             // the document's `name:` ("Config" for v1alpha1)
	Detail   string `json:"detail,omitempty"` // where inside the document, e.g. machine.network.interfaces[eth0]
	DocIndex int    `json:"docIndex"`         // index into Inputs.Docs
}

// Address sources. Layers are Talos's own (configuration, operator, platform,
// cmdline, default); the operator layer is split by what the operator was.
const (
	SrcStatic   = "static"
	SrcDHCP4    = "dhcp4"
	SrcDHCP6    = "dhcp6"
	SrcVIP      = "vip"
	SrcPlatform = "platform"
	SrcCmdline  = "cmdline"
	SrcDefault  = "default"
	SrcOperator = "operator"
	SrcKernel   = "kernel" // no spec asked for it: kernel, CNI or another process
)

// Address is one address on a link.
type Address struct {
	Prefix  string     `json:"prefix"`
	Family  string     `json:"family"`
	Scope   string     `json:"scope"`
	Source  string     `json:"source"`
	Layers  []string   `json:"layers,omitempty"` // every layer with an AddressSpec for it, highest priority first
	VIP     bool       `json:"vip,omitempty"`
	Missing bool       `json:"missing,omitempty"` // a spec asks for it but the kernel does not have it
	Config  *ConfigRef `json:"config,omitempty"`
	Status  *Ref       `json:"status,omitempty"`
	Spec    *Ref       `json:"spec,omitempty"` // the highest priority spec
}

// Route is one route, attached to the link it leaves through.
type Route struct {
	Dst      string     `json:"dst"` // "default" for the default route
	Gateway  string     `json:"gateway,omitempty"`
	Src      string     `json:"src,omitempty"`
	Family   string     `json:"family"`
	Table    string     `json:"table"`
	Priority int        `json:"priority"`
	Protocol string     `json:"protocol,omitempty"`
	Scope    string     `json:"scope,omitempty"`
	Type     string     `json:"type"`
	OutLink  string     `json:"outLink,omitempty"`
	Source   string     `json:"source"` // layer of the RouteSpec behind it, or kernel
	Default  bool       `json:"default,omitempty"`
	Config   *ConfigRef `json:"config,omitempty"`
	Status   *Ref       `json:"status,omitempty"`
	Spec     *Ref       `json:"spec,omitempty"`
}

// Operator is a Talos operator running on a link (DHCP, VIP).
type Operator struct {
	Kind   string     `json:"kind"` // dhcp4, dhcp6, vip, lldp
	VIP    string     `json:"vip,omitempty"`
	Layer  string     `json:"layer,omitempty"`
	Config *ConfigRef `json:"config,omitempty"`
	Spec   Ref        `json:"spec"`
}

// SpecRef is one LinkSpec (per layer, plus the merged one with Layer "merged").
type SpecRef struct {
	Layer string `json:"layer"`
	Ref   Ref    `json:"ref"`
}

// KV is one detail line.
type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Link is one network link.
type Link struct {
	Name      string   `json:"name"`
	Alias     string   `json:"alias,omitempty"`
	AltNames  []string `json:"altNames,omitempty"`
	Index     int      `json:"index"`
	Kind      string   `json:"kind"` // "" for physical NICs, loopback and plain kernel links
	Type      string   `json:"type"` // ether, loopback, none, …
	Physical  bool     `json:"physical"`
	State     string   `json:"state"` // up, down, unknown
	OperState string   `json:"operState"`
	MTU       int      `json:"mtu"`
	HWAddr    string   `json:"hwAddr,omitempty"`
	Driver    string   `json:"driver,omitempty"`
	SpeedMbit int      `json:"speedMbit,omitempty"`
	BusPath   string   `json:"busPath,omitempty"`

	Lowers []string `json:"lowers,omitempty"` // links this one is built on or enslaves, lowest index first; Lowers[0] is the tree parent
	Detail []KV     `json:"detail,omitempty"` // kind-specific: bond mode, VLAN id, wireguard peers…

	Addresses []Address   `json:"addresses,omitempty"`
	Routes    []Route     `json:"routes,omitempty"`
	Operators []Operator  `json:"operators,omitempty"`
	Configs   []ConfigRef `json:"configs,omitempty"` // every document naming this link
	Specs     []SpecRef   `json:"specs,omitempty"`
	Status    Ref         `json:"status"`
}

// Warning is something the config asks for that the node does not have.
type Warning struct {
	Name    string    `json:"name"` // the link (or VIP) the document names
	Message string    `json:"message"`
	Config  ConfigRef `json:"config"`
}

// Peer is one KubeSpan peer.
type Peer struct {
	Label    string `json:"label"`
	State    string `json:"state"`
	Endpoint string `json:"endpoint,omitempty"`
	Ref      Ref    `json:"ref"`
}

// KubeSpan is the node's KubeSpan identity and peers.
type KubeSpan struct {
	Address   string `json:"address,omitempty"`
	Subnet    string `json:"subnet,omitempty"`
	PublicKey string `json:"publicKey,omitempty"`
	Peers     []Peer `json:"peers,omitempty"`
}

// Service is something built on top of the addresses: a VIP, KubeSpan, the
// node's primary address.
type Service struct {
	Kind   string     `json:"kind"` // vip, kubespan, node-address
	Name   string     `json:"name"`
	Detail string     `json:"detail,omitempty"`
	Link   string     `json:"link,omitempty"`
	Config *ConfigRef `json:"config,omitempty"`
	Ref    *Ref       `json:"ref,omitempty"`
}

// NoteText is the PR A note for a type or config kind.
type NoteText struct {
	What   string `json:"what"`
	Ubuntu string `json:"ubuntu,omitempty"`
}

// Model is the whole picture for one node.
type Model struct {
	Node      string   `json:"node"`
	Hostname  string   `json:"hostname,omitempty"`
	Domain    string   `json:"domain,omitempty"`
	Resolvers []string `json:"resolvers,omitempty"`

	Links    []Link              `json:"links"`    // by kernel index
	Roots    []string            `json:"roots"`    // links with nothing below them: physical NICs first
	Children map[string][]string `json:"children"` // link -> links whose tree parent it is
	Also     map[string][]string `json:"also"`     // link -> links built on it that hang under another parent

	DefaultRoutes []Route   `json:"defaultRoutes,omitempty"`
	OrphanRoutes  []Route   `json:"orphanRoutes,omitempty"` // main-table routes that leave through no known link
	HiddenRoutes  int       `json:"hiddenRoutes,omitempty"` // local/broadcast and other non-unicast, non-main routes not listed
	NodeAddresses []string  `json:"nodeAddresses,omitempty"`
	Warnings      []Warning `json:"warnings,omitempty"`
	KubeSpan      *KubeSpan `json:"kubespan,omitempty"`
	Services      []Service `json:"services,omitempty"`

	Docs  []ConfigRef         `json:"docs,omitempty"`  // networking documents of the machine config
	Notes map[string]NoteText `json:"notes,omitempty"` // by display type and config kind
}

// Link returns the link with that name.
func (m Model) Link(name string) (Link, bool) {
	for _, l := range m.Links {
		if l.Name == name {
			return l, true
		}
	}
	return Link{}, false
}
