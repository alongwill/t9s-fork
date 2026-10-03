package ui

import (
	"time"

	"github.com/florianspk/t9s/internal/talos"
)

type tickMsg time.Time

type nodesLoadedMsg struct {
	nodes []talos.Node
	err   error
}

type servicesLoadedMsg struct {
	services []talos.Service
	err      error
}

type logStreamsLoadedMsg struct {
	streams  []string
	nodeIP   string
	sequence uint64
	err      error
}

type extensionsLoadedMsg struct {
	extensions []talos.Extension
	err        error
}

type machineConfigLoadedMsg struct {
	content string
	err     error
}

type statsLoadedMsg struct {
	stats []talos.StatsResult
	err   error
}

type catalogLoadedMsg struct {
	catalog []talos.CatalogExtension
	err     error
}

type disksLoadedMsg struct {
	disks []talos.DiskInfo
	err   error
}

type volumesLoadedMsg struct {
	volumes []talos.VolumeInfo
	err     error
}

type processesLoadedMsg struct {
	processes []talos.ProcessInfo
	err       error
}

type containersLoadedMsg struct {
	containers []talos.ContainerInfo
	err        error
}

type addressesLoadedMsg struct {
	addresses []talos.AddressInfo
	err       error
}

type healthLineMsg string
type healthDoneMsg struct{}

type actionDoneMsg struct {
	action string
	nodeIP string // IP of the node the action targeted
	err    error
}

type logLineMsg struct {
	line       string
	sessionSeq uint64
}

type logDoneMsg struct {
	sessionSeq uint64
}

type dmesgLineMsg string
type dmesgDoneMsg struct{}

type upgradeLineMsg string
type upgradeDoneMsg struct{ err error }

type kubeVersionLoadedMsg struct {
	version string
	err     error
}

type clientVersionMsg struct {
	version string // talosctl client version, e.g. "v1.11.0"
}

type nodeDetailsMsg struct {
	details map[string]talos.NodeDetails // node IP → details
}

type editorDoneMsg struct{ err error }

type machineConfigAppliedMsg struct {
	err  error
	file string // temp file to clean up
}

// Resource browser. Every message carries the node it was requested for so
// replies for a node the user has since left are dropped.
type resourceDefsMsg struct {
	node string
	defs []talos.ResourceDef
	err  error
}

type resourceCountMsg struct {
	node, typ string
	n         int
	only      talos.ResourceMeta // set when n == 1
	locked    bool
	err       error
}

type resourceInstancesMsg struct {
	node, typ string
	items     []talos.ResourceMeta
	err       error
}

type configDocsMsg struct {
	node   string
	docs   []talos.ConfigDoc
	denied bool
	err    error
}

type resourceYAMLMsg struct {
	node, typ, id string
	yaml          string
	err           error
}
