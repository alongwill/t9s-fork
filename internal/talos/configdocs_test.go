package talos

import (
	"strings"
	"testing"
)

const configStream = `version: v1alpha1
# keep this comment
machine:
  type: controlplane
  files:
    - content: |
        ---
        inside a block scalar
      path: /etc/x
---
apiVersion: v1alpha1
kind: LinkConfig
name: eth0
addresses:
  - address: 10.0.0.2/24
---
apiVersion: v1alpha1
kind: LinkConfig
name: eth1
---
apiVersion: v1alpha1
kind: DHCPv4Config
name: eth0
---
---
`

func checkDocs(t *testing.T, docs []ConfigDoc) {
	t.Helper()
	want := []struct{ kind, name string }{
		{"v1alpha1", "Config"}, {"LinkConfig", "eth0"}, {"LinkConfig", "eth1"}, {"DHCPv4Config", "eth0"},
	}
	if len(docs) != len(want) {
		t.Fatalf("want %d docs, got %d: %+v", len(want), len(docs), docs)
	}
	for i, w := range want {
		if docs[i].Kind != w.kind || docs[i].Name != w.name {
			t.Errorf("doc %d = %s/%s, want %s/%s", i, docs[i].Kind, docs[i].Name, w.kind, w.name)
		}
	}
	if !strings.Contains(docs[0].YAML, "# keep this comment") || !strings.Contains(docs[0].YAML, "inside a block scalar") {
		t.Errorf("legacy doc lost comment or block scalar:\n%s", docs[0].YAML)
	}
	if strings.Contains(docs[1].YAML, "---") {
		t.Errorf("separator leaked into doc:\n%s", docs[1].YAML)
	}
}

func TestSplitConfigDocsBareStream(t *testing.T) {
	docs, err := SplitConfigDocs(configStream)
	if err != nil {
		t.Fatal(err)
	}
	checkDocs(t, docs)
}

func TestSplitConfigDocsResourceEnvelope(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("node: 10.0.0.2\nmetadata:\n  namespace: config\n  type: MachineConfigs.config.talos.dev\n  id: v1alpha1\nspec: |\n")
	for _, l := range strings.Split(configStream, "\n") {
		sb.WriteString("  " + l + "\n")
	}
	docs, err := SplitConfigDocs(sb.String())
	if err != nil {
		t.Fatal(err)
	}
	checkDocs(t, docs)
}

func TestSplitConfigDocsEmpty(t *testing.T) {
	docs, err := SplitConfigDocs("")
	if err != nil || len(docs) != 0 {
		t.Fatalf("got %v, %v", docs, err)
	}
}
