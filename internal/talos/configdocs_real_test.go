package talos

import (
	"strconv"
	"strings"
	"testing"
)

// realShapeStream follows the machine config of a Talos v1.14.2 control plane
// (secrets replaced): trailing comments after values, comment-only lines,
// repeated kinds with names, and a document whose body holds nested kinds.
const realShapeStream = `version: v1alpha1 # Indicates the schema used to decode the contents.
debug: false # Enable verbose logging to the console.
persist: true
# Provides machine specific configuration options.
machine:
    type: controlplane # Defines the role of the machine within the cluster.
    token: REDACTED # The token is used by a machine to join the PKI of the cluster.
    ca:
        crt: REDACTED
        key: REDACTED
    # Features describe individual Talos features that can be switched on or off.
    features:
        diskQuotaSupport: true
cluster:
    token: REDACTED
---
apiVersion: v1alpha1
kind: DiscoveryServiceConfig
name: default # Name of the discovery service configuration.
endpoint: https://discovery.talos.dev/ # Discovery service endpoint to use.
---
apiVersion: v1alpha1
kind: VolumeConfig
name: EPHEMERAL # Name of the volume.
# The mount describes additional mount options.
mount:
    secure: true # Enable secure mount options (nosuid, nodev).

# # The encryption describes how the volume is encrypted.
# encryption:
#     provider: luks2
---
apiVersion: v1alpha1
kind: RegistryMirrorConfig
name: docker.io # Registry name to apply the mirror configuration to.
endpoints:
    - url: http://172.30.0.1:5001 # The URL of the registry mirror endpoint.
---
apiVersion: v1alpha1
kind: RegistryMirrorConfig
name: gcr.io # Registry name to apply the mirror configuration to.
endpoints:
    - url: http://172.30.0.1:5004
---
apiVersion: v1alpha1
kind: KubeAdmissionControlConfig
name: PodSecurity
configuration:
    apiVersion: pod-security.admission.config.k8s.io/v1alpha1
    kind: PodSecurityConfiguration
---
apiVersion: v1alpha1
kind: KubeAuthorizerConfig
name: node
type: Node
---
apiVersion: v1alpha1
kind: KubeAuthorizerConfig
name: rbac
type: RBAC
---
apiVersion: v1alpha1
kind: LinkAliasConfig
name: net0 # Alias for the link.
selector:
    match: link.driver == "virtio_net" # The CEL expression to match the link.
---
apiVersion: v1alpha1
kind: DHCPv4Config
name: net0 # Name of the link (interface).

# # Raw value of the DUID to use as client identifier.
# duidRaw: 00:01:00:01:23:45:67:89:ab:cd:ef:01:23:45
`

// realShapeEnvelope renders the stream the way the CLI prints the resource:
// `spec` as one double-quoted scalar, escaped and folded across lines.
func realShapeEnvelope(stream string) string {
	q := strconv.Quote(stream)
	var sb strings.Builder
	col := 0
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == ' ' && col > 70 && i+1 < len(q) && q[i+1] != ' ' && q[i-1] != ' ' {
			sb.WriteString("\n    ")
			col = 4
			continue
		}
		sb.WriteByte(c)
		col++
	}
	return "node: 172.30.0.2\nmetadata:\n    namespace: config\n    type: MachineConfigs.config.talos.dev\n" +
		"    id: v1alpha1\n    version: 1\n    owner:\n    phase: running\n    annotations:\n        talos.dev/yaml-spec: 1\n" +
		"spec: " + sb.String() + "\n"
}

func TestSplitConfigDocsRealShape(t *testing.T) {
	raw := realShapeEnvelope(realShapeStream)
	if !strings.Contains(raw, "\n    ") || !strings.Contains(raw, `\n---\napiVersion`) {
		t.Fatalf("fixture is not in the folded, escaped form:\n%s", raw)
	}
	docs, err := SplitConfigDocs(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ kind, name string }{
		{"v1alpha1", "Config"},
		{"DiscoveryServiceConfig", "default"},
		{"VolumeConfig", "EPHEMERAL"},
		{"RegistryMirrorConfig", "docker.io"},
		{"RegistryMirrorConfig", "gcr.io"},
		{"KubeAdmissionControlConfig", "PodSecurity"},
		{"KubeAuthorizerConfig", "node"},
		{"KubeAuthorizerConfig", "rbac"},
		{"LinkAliasConfig", "net0"},
		{"DHCPv4Config", "net0"},
	}
	if len(docs) != len(want) {
		t.Fatalf("got %d docs, want %d: %+v", len(docs), len(want), docs)
	}
	for i, w := range want {
		if docs[i].Kind != w.kind || docs[i].Name != w.name {
			t.Errorf("doc %d = %s/%s, want %s/%s", i, docs[i].Kind, docs[i].Name, w.kind, w.name)
		}
	}
	// comments survive, and a nested `kind:` inside a body does not become the kind
	if !strings.Contains(docs[0].YAML, "# Provides machine specific configuration options.") {
		t.Errorf("legacy doc lost its comments:\n%s", docs[0].YAML)
	}
	if docs[5].Kind != "KubeAdmissionControlConfig" || !strings.Contains(docs[5].YAML, "kind: PodSecurityConfiguration") {
		t.Errorf("nested kind confused the split: %+v", docs[5])
	}
	if !strings.Contains(docs[9].YAML, "# duidRaw:") {
		t.Errorf("trailing comments dropped:\n%s", docs[9].YAML)
	}
	// every document stands alone: no separators leaked in
	for i, d := range docs {
		if strings.Contains(d.YAML, "\n---") || strings.HasPrefix(d.YAML, "---") {
			t.Errorf("doc %d contains a separator:\n%s", i, d.YAML)
		}
	}
}
