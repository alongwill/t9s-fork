package catalog

import (
	"sort"
	"strings"
	"testing"
)

// knownDisplayTypes is the display type of every resource the notes cover,
// checked by hand against pkg/machinery/resources/ in the Talos source
// (the `Type` constant of each, with the plural dropped).
var knownDisplayTypes = map[string]string{
	"LinkStatus": "LinkStatuses.net.talos.dev", "LinkSpec": "LinkSpecs.net.talos.dev",
	"AddressStatus": "AddressStatuses.net.talos.dev", "AddressSpec": "AddressSpecs.net.talos.dev",
	"RouteStatus": "RouteStatuses.net.talos.dev", "RouteSpec": "RouteSpecs.net.talos.dev",
	"OperatorSpec": "OperatorSpecs.net.talos.dev", "ResolverStatus": "ResolverStatuses.net.talos.dev",
	"ResolverSpec": "ResolverSpecs.net.talos.dev", "HostnameStatus": "HostnameStatuses.net.talos.dev",
	"HostnameSpec": "HostnameSpecs.net.talos.dev", "NodeAddress": "NodeAddresses.net.talos.dev",
	"TimeServerStatus": "TimeServerStatuses.net.talos.dev", "TimeServerSpec": "TimeServerSpecs.net.talos.dev",
	"ProbeStatus": "ProbeStatuses.net.talos.dev", "KubeSpanPeerStatus": "KubeSpanPeerStatuses.kubespan.talos.dev",
	"Disk": "Disks.block.talos.dev", "DiscoveredVolume": "DiscoveredVolumes.block.talos.dev",
	"VolumeConfig": "VolumeConfigs.block.talos.dev", "VolumeStatus": "VolumeStatuses.block.talos.dev",
	"MountStatus": "MountStatuses.block.talos.dev", "SystemDisk": "SystemDisks.block.talos.dev",
	"MachineStatus": "MachineStatuses.runtime.talos.dev", "Service": "Services.v1alpha1.talos.dev",
	"ExtensionStatus": "ExtensionStatuses.runtime.talos.dev", "KernelParamStatus": "KernelParamStatuses.runtime.talos.dev",
	"KernelParamSpec": "KernelParamSpecs.runtime.talos.dev", "ExtensionServiceConfig": "ExtensionServiceConfigs.runtime.talos.dev",
	"MachineConfig": "MachineConfigs.config.talos.dev",
	"Member":        "Members.cluster.talos.dev", "Affiliate": "Affiliates.cluster.talos.dev", "Identity": "Identities.cluster.talos.dev",
	"EtcdMember": "EtcdMembers.etcd.talos.dev", "EtcdSpec": "EtcdSpecs.etcd.talos.dev",
	"KubeletSpec": "KubeletSpecs.kubernetes.talos.dev", "StaticPodStatus": "StaticPodStatuses.kubernetes.talos.dev",
	"Nodename": "Nodenames.kubernetes.talos.dev",
}

func TestNoteKeysAreRealDisplayTypes(t *testing.T) {
	types := NoteTypes()
	if len(types) < 35 {
		t.Fatalf("only %d notes, want about 40", len(types))
	}
	sort.Strings(types)
	for _, k := range types {
		if _, ok := knownDisplayTypes[k]; !ok {
			t.Errorf("note %q is not a checked display type", k)
		}
	}
	for k := range knownDisplayTypes {
		if _, ok := NoteFor(k); !ok {
			t.Errorf("checked type %q has no note", k)
		}
	}
}

func TestNoteShape(t *testing.T) {
	for k := range knownDisplayTypes {
		n, ok := NoteFor(k)
		if !ok {
			continue
		}
		if strings.TrimSpace(n.What) == "" {
			t.Errorf("%s: empty what", k)
		}
		if !strings.HasSuffix(n.What, ".") {
			t.Errorf("%s: what should be one sentence ending in a period: %q", k, n.What)
		}
		if strings.Count(n.What, ". ") > 0 {
			t.Errorf("%s: what has more than one sentence: %q", k, n.What)
		}
	}
	if _, ok := NoteFor("Nope"); ok {
		t.Error("NoteFor found a type that does not exist")
	}
}

func TestConfigKindsHaveUbuntu(t *testing.T) {
	n := 0
	for _, k := range ConfigKinds() {
		if k.Ubuntu != "" {
			n++
		}
	}
	if n < 90 {
		t.Errorf("only %d of %d config kinds have an Ubuntu equivalent", n, len(ConfigKinds()))
	}
}
