package ui

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// rawMachineConfig mimics `get machineconfig -o yaml`: spec is a string
// holding the whole (possibly multi-document) machine config.
func rawMachineConfig(t *testing.T, spec string) string {
	t.Helper()
	out, err := yaml.Marshal(map[string]any{
		"node":     "10.0.0.1",
		"metadata": map[string]any{"namespace": "config", "type": "MachineConfigs.config.talos.dev", "id": "v1alpha1"},
		"spec":     spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestExtractSpecContentSingleDocument(t *testing.T) {
	got := extractSpecContent(rawMachineConfig(t, "version: v1alpha1\nmachine:\n  type: worker\n"))
	if !strings.Contains(got, "type: worker") {
		t.Fatalf("missing machine.type in %q", got)
	}
	if strings.Contains(got, "---") {
		t.Fatalf("single document must not get a separator: %q", got)
	}
}

func TestExtractSpecContentKeepsAllDocuments(t *testing.T) {
	spec := "version: v1alpha1\nmachine:\n  type: controlplane\n" +
		"---\napiVersion: v1alpha1\nkind: HostnameConfig\nauto: stable\n" +
		"---\napiVersion: v1alpha1\nkind: SysctlConfig\nsysctls:\n  vm.swappiness: \"10\"\n" +
		"---\n"
	got := extractSpecContent(rawMachineConfig(t, spec))

	for _, want := range []string{"type: controlplane", "kind: HostnameConfig", "kind: SysctlConfig", "vm.swappiness"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	// Round-trip: the result must decode back into exactly three documents.
	dec := yaml.NewDecoder(strings.NewReader(got))
	n := 0
	for {
		var v map[string]any
		if err := dec.Decode(&v); err != nil {
			break
		}
		n++
	}
	if n != 3 {
		t.Fatalf("want 3 documents, got %d:\n%s", n, got)
	}
}

func TestExtractSpecContentInvalidInnerYAMLReturnedVerbatim(t *testing.T) {
	spec := "machine: [unterminated\n"
	if got := extractSpecContent(rawMachineConfig(t, spec)); got != spec {
		t.Fatalf("want verbatim spec, got %q", got)
	}
}
