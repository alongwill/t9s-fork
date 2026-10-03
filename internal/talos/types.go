package talos

type Node struct {
	Hostname    string
	IP          string // actual node IP used for talosctl -n
	DisplayIP   string // shown in UI (may include VIP)
	Role        string
	Version     string   // Talos version
	KubeVersion string   // Kubernetes/kubelet version (fetched async)
	NoK8s       bool     // node runs without Kubernetes (Talos 1.14 k8s-less mode)
	Status      string   // derived from MachineStatus: ready, not ready, booting, upgrading, …
	Unmet       []string // unmet readiness conditions, e.g. "nodeReady: node not ready"
}

type Service struct {
	ID      string
	State   string
	Healthy string
}

type Extension struct {
	Name        string
	Version     string
	Description string
}

type StatsResult struct {
	ID       string
	CPUNanos int64   // cumulative CPU nanoseconds
	MemoryMB float64 // memory in MB
}

type CatalogExtension struct {
	Name        string
	ImageRef    string // full ref with digest, e.g. ghcr.io/siderolabs/amd-ucode:v1.6.4@sha256:...
	Author      string
	Description string
}

type ProcessInfo struct {
	PID     string
	State   string
	CPUTime string
	ResMem  string
	Command string
}

type ContainerInfo struct {
	Namespace string
	ID        string
	Image     string
	PID       string
	Status    string
}

type AddressInfo struct {
	Interface string
	Address   string
	Family    string
	Scope     string
}

// ResourceDef describes one COSI resource type, as reported by `get rd`.
type ResourceDef struct {
	Type             string // e.g. "AddressStatuses.net.talos.dev"
	DisplayType      string // e.g. "AddressStatus"
	Aliases          []string
	DefaultNamespace string
	Sensitive        bool
}

// ResourceMeta is the metadata of one resource instance. Owner is the
// controller that wrote it (empty for resources written by the API).
type ResourceMeta struct{ Namespace, Type, ID, Version, Phase, Owner string }
