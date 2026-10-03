package talos

import (
	"slices"
	"testing"
)

// Real talosctl 1.14 `processes` output: LABEL is empty without SELinux,
// and COMMAND contains spaces.
const processes114 = `NODE         PID    STATE   THREADS   CPU-TIME   VIRTMEM   RESMEM   LABEL                     COMMAND
10.5.0.2     1      S       14        12.34      1.4 GB    120 MB                             /sbin/init
10.5.0.2     2155   S       30        99.01      2.1 GB    400 MB   system_u:system_r:etcd_t  /usr/local/bin/etcd --name=cp-1 --data-dir=/var/lib/etcd
`

func TestParseProcessLinesV114(t *testing.T) {
	got := parseProcessLines([]byte(processes114))
	if len(got) != 2 {
		t.Fatalf("want 2 processes, got %d: %+v", len(got), got)
	}
	want := ProcessInfo{PID: "1", State: "S", CPUTime: "12.34", ResMem: "120 MB", Command: "/sbin/init"}
	if got[0] != want {
		t.Errorf("row 0 = %+v, want %+v", got[0], want)
	}
	if got[1].Command != "/usr/local/bin/etcd --name=cp-1 --data-dir=/var/lib/etcd" {
		t.Errorf("command with spaces not preserved: %q", got[1].Command)
	}
	if got[1].ResMem != "400 MB" {
		t.Errorf("ResMem = %q", got[1].ResMem)
	}
}

func TestParseTableShortLines(t *testing.T) {
	rows := parseTable([]byte("A    B    C\nx    y\n"))
	if len(rows) != 1 || rows[0]["A"] != "x" || rows[0]["B"] != "y" || rows[0]["C"] != "" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestParseTableMultiByteCells(t *testing.T) {
	// tabwriter aligns on runes; the "└─" tree marker is multi-byte.
	data := "NODE   ID        PID\n" +
		"n1     └─ abc    42\n"
	rows := parseTable([]byte(data))
	if len(rows) != 1 || rows[0]["PID"] != "42" || rows[0]["ID"] != "└─ abc" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestImageTag(t *testing.T) {
	cases := map[string]string{
		"ghcr.io/siderolabs/kubelet:v1.37.0":                 "v1.37.0",
		"registry.local:5000/siderolabs/kubelet:v1.36.2":     "v1.36.2",
		"ghcr.io/siderolabs/kubelet:v1.37.0@sha256:deadbeef": "v1.37.0",
		"registry.local:5000/kubelet@sha256:deadbeef":        "",
		"kubelet": "",
	}
	for in, want := range cases {
		if got := imageTag(in); got != want {
			t.Errorf("imageTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUpgradeArgs(t *testing.T) {
	opts := UpgradeOptions{Image: "img:v1.14.2", Drain: false, Preserve: true}

	modern := upgradeArgs("10.0.0.1", opts, true)
	if slices.Contains(modern, "--preserve") {
		t.Errorf("--preserve is a no-op on 1.14 and must not be passed: %v", modern)
	}
	for _, want := range []string{"--progress", "plain", "--drain=false"} {
		if !slices.Contains(modern, want) {
			t.Errorf("modern args missing %q: %v", want, modern)
		}
	}

	legacy := upgradeArgs("10.0.0.1", opts, false)
	if !slices.Contains(legacy, "--preserve") || slices.Contains(legacy, "--progress") {
		t.Errorf("legacy args wrong: %v", legacy)
	}
}

func TestMachineStatusNodeStatus(t *testing.T) {
	cases := []struct {
		ms   MachineStatus
		want string
	}{
		{MachineStatus{Stage: "running", Ready: true}, "ready"},
		{MachineStatus{Stage: "running"}, "not ready"},
		{MachineStatus{Stage: "upgrading"}, "upgrading"},
		{MachineStatus{}, "unknown"},
	}
	for _, c := range cases {
		if got := c.ms.NodeStatus(); got != c.want {
			t.Errorf("%+v → %q, want %q", c.ms, got, c.want)
		}
	}
}

func TestMachineStatusEnvelopeDecoding(t *testing.T) {
	data := `{
  "node": "10.5.0.2",
  "metadata": {"id": "machine"},
  "spec": {"stage": "running", "status": {"ready": false,
    "unmetConditions": [{"name": "nodeReady", "reason": "node not ready"}]}}
}`
	envs, _ := parseJSONStream[machineStatusEnvelope]([]byte(data))
	if len(envs) != 1 || envs[0].Spec.Stage != "running" || len(envs[0].Spec.Status.UnmetConditions) != 1 {
		t.Fatalf("decode failed: %+v", envs)
	}
}

func TestModernCLI(t *testing.T) {
	c := New("", "")
	if !c.ModernCLI() {
		t.Error("unknown client version should be treated as modern")
	}
	c.SetClientVersion("v1.13.5")
	if c.ModernCLI() {
		t.Error("1.13 is not modern")
	}
	c.SetClientVersion("v1.14.0-beta.1")
	if !c.ModernCLI() {
		t.Error("1.14 pre-release is modern")
	}
}
