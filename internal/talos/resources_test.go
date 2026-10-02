package talos

import (
	"errors"
	"testing"
)

const rdFixture = `{
	"node": "10.0.0.1",
	"metadata": {"namespace": "meta", "type": "ResourceDefinitions.meta.cosi.dev", "id": "addressstatuses.net.talos.dev", "version": 1, "phase": "running"},
	"spec": {
		"type": "AddressStatuses.net.talos.dev",
		"displayType": "AddressStatus",
		"defaultNamespace": "network",
		"aliases": ["addresses", "address"],
		"allAliases": ["addresses", "address", "addressstatus"]
	}
}
{
	"node": "10.0.0.1",
	"metadata": {"namespace": "meta", "type": "ResourceDefinitions.meta.cosi.dev", "id": "disks.block.talos.dev", "version": 1, "phase": "running"},
	"spec": {
		"type": "Disks.block.talos.dev",
		"displayType": "Disk",
		"defaultNamespace": "runtime",
		"allAliases": ["disks", "disk"]
	}
}
{
	"node": "10.0.0.1",
	"metadata": {"namespace": "meta", "type": "ResourceDefinitions.meta.cosi.dev", "id": "kubeletspecs.k8s.talos.dev", "version": 1, "phase": "running"},
	"spec": {
		"type": "KubeletSpecs.kubernetes.talos.dev",
		"displayType": "KubeletSpec",
		"defaultNamespace": "k8s",
		"aliases": ["kubeletspecs"],
		"sensitivity": "sensitive"
	}
}
{
	"node": "10.0.0.1",
	"metadata": {"namespace": "meta", "type": "ResourceDefinitions.meta.cosi.dev", "id": "empty", "version": 1, "phase": "running"},
	"spec": {}
}`

func TestParseResourceDefs(t *testing.T) {
	defs, err := parseResourceDefs([]byte(rdFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 3 {
		t.Fatalf("want 3 defs (empty spec skipped), got %d", len(defs))
	}
	a := defs[0]
	if a.Type != "AddressStatuses.net.talos.dev" || a.DisplayType != "AddressStatus" ||
		a.DefaultNamespace != "network" || a.Aliases[0] != "addresses" || a.Sensitive {
		t.Errorf("unexpected def: %+v", a)
	}
	if defs[1].Aliases[0] != "disks" {
		t.Errorf("allAliases fallback failed: %+v", defs[1])
	}
	if !defs[2].Sensitive {
		t.Errorf("sensitive flag not set: %+v", defs[2])
	}
}

func TestIsSensitive(t *testing.T) {
	cases := map[string]bool{
		``: false, `null`: false, `""`: false, `0`: false, `false`: false,
		`"sensitive"`: true, `1`: true, `true`: true,
	}
	for in, want := range cases {
		if got := isSensitive([]byte(in)); got != want {
			t.Errorf("isSensitive(%q) = %v, want %v", in, got, want)
		}
	}
}

const listFixture = `{
	"node": "10.0.0.1",
	"metadata": {"namespace": "network", "type": "AddressStatuses.net.talos.dev", "id": "lo/127.0.0.1/8", "version": "2", "phase": "running"},
	"spec": {"address": "127.0.0.1/8"}
}
{
	"node": "10.0.0.1",
	"metadata": {"namespace": "network", "type": "AddressStatuses.net.talos.dev", "id": "eth0/10.0.0.1/24", "version": 5, "phase": "running"},
	"spec": {"address": "10.0.0.1/24"}
}`

func TestParseResourceList(t *testing.T) {
	got, err := parseResourceList([]byte(listFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].ID != "lo/127.0.0.1/8" || got[0].Version != "2" || got[0].Namespace != "network" || got[0].Phase != "running" {
		t.Errorf("unexpected: %+v", got[0])
	}
	if got[1].Version != "5" {
		t.Errorf("numeric version: %+v", got[1])
	}
}

func TestParseResourceListEmpty(t *testing.T) {
	got, err := parseResourceList(nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil slice, got %v, %v", got, err)
	}
}

func TestIsPermissionDenied(t *testing.T) {
	denied := errors.New(`exit status 1: rpc error: code = PermissionDenied desc = not authorized`)
	if !IsPermissionDenied(denied) {
		t.Error("expected permission denied")
	}
	if !IsPermissionDenied(errors.New("exit status 1: not authorized for role")) {
		t.Error("expected 'not authorized' match")
	}
	if IsPermissionDenied(errors.New("exit status 1: connection refused")) || IsPermissionDenied(nil) {
		t.Error("false positive")
	}
}
