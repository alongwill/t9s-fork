package catalog

import (
	"testing"

	"github.com/florianspk/t9s/internal/talos"
)

func TestCategoryFor(t *testing.T) {
	cases := []struct {
		typ, display, ns, want string
	}{
		{"LinkStatuses.net.talos.dev", "LinkStatus", "network", "networking"},
		{"PeerStatuses.kubespan.talos.dev", "PeerStatus", "kubespan", "networking"},
		{"Config.siderolink.talos.dev", "Config", "runtime", "siderolink"},
		{"Manifests.kubernetes.talos.dev", "Manifest", "controlplane", "kubernetes"},
		{"KubeletSpecs.k8s.talos.dev", "KubeletSpec", "k8s", "kubernetes"},
		{"Members.cluster.talos.dev", "Member", "cluster", "cluster"},
		{"Members.etcd.talos.dev", "Member", "etcd", "cluster"},
		{"Disks.block.talos.dev", "Disk", "runtime", "block"},
		{"Volumes.storage.talos.dev", "Volume", "runtime", "storage"},
		{"RegistryConfigs.cri.talos.dev", "RegistryConfig", "cri", "cri"},
		{"Containers.containers.talos.dev", "Container", "runtime", "containers"},
		{"Images.hypervisor.talos.dev", "Image", "runtime", "hypervisor"},
		{"Processors.hardware.talos.dev", "Processor", "runtime", "hardware"},
		{"Certs.security.talos.dev", "Cert", "runtime", "security"},
		{"Etcd.secrets.talos.dev", "Etcd", "secrets", "security"},
		{"Services.runtime.talos.dev", "Service", "runtime", "runtime"},
		{"MachineConfigs.config.talos.dev", "MachineConfig", "config", "runtime"},
		{"Foo.v1alpha1.talos.dev", "Foo", "runtime", "runtime"},
		{"Files.files.talos.dev", "File", "files", "runtime"},
		{"Stats.perf.talos.dev", "Stat", "perf", "runtime"},
		// override: Extension* beats the suffix
		{"ExtensionStatuses.runtime.talos.dev", "ExtensionStatus", "runtime", "extensions"},
		{"ExtensionServiceConfigs.runtime.talos.dev", "ExtensionServiceConfig", "runtime", "extensions"},
		// unknown suffix
		{"Things.brandnew.talos.dev", "Thing", "x", "other"},
		{"Weird", "Weird", "x", "other"},
	}
	for _, c := range cases {
		got := CategoryFor(talos.ResourceDef{Type: c.typ, DisplayType: c.display, DefaultNamespace: c.ns})
		if got != c.want {
			t.Errorf("CategoryFor(%s) = %s, want %s", c.typ, got, c.want)
		}
	}
}

func TestCategoryKeysAreKnown(t *testing.T) {
	known := map[string]bool{}
	for _, c := range Categories {
		known[c.Key] = true
	}
	for s, k := range suffixCategory {
		if !known[k] {
			t.Errorf("suffix %s maps to unknown category %s", s, k)
		}
	}
	if Label("block") != "Block and volumes" {
		t.Error("Label lookup")
	}
}
