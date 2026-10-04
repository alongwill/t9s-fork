package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/florianspk/t9s/internal/talos"
)

func TestSplitMetricsID(t *testing.T) {
	tests := []struct {
		id, ns, name string
	}{
		{"kube-system/kube-apiserver-cp-1:kube-apiserver:3f2a1b9c8d7e", "kube-system", "kube-apiserver-cp-1:kube-apiserver:3f2a1b9c8d7e"},
		{"kube-system/coredns-abc", "kube-system", "coredns-abc"}, // pod sandbox
		{"default/web-0:nginx:aabbccddeeff", "default", "web-0:nginx:aabbccddeeff"},
		{"apid", "-", "apid"},
		{"etcd", "-", "etcd"},
	}
	for _, tc := range tests {
		ns, name := splitMetricsID(tc.id)
		if ns != tc.ns || name != tc.name {
			t.Errorf("splitMetricsID(%q) = (%q, %q), want (%q, %q)", tc.id, ns, name, tc.ns, tc.name)
		}
	}
}

func metricsApp(width, height, n int) App {
	node := talos.Node{Hostname: "cp-1", IP: "10.0.0.2"}
	app := App{width: width, height: height, state: StateMetrics, selNode: &node}
	app.stats = append(app.stats,
		talos.StatsResult{ID: "apid", MemoryMB: 120, CPUNanos: 10},
		talos.StatsResult{ID: "kube-system/kube-apiserver-cp-1:kube-apiserver:3f2a1b9c8d7e", MemoryMB: 900, CPUNanos: 10},
		talos.StatsResult{ID: "local-path-storage/local-path-provisioner-6d8b:local-path-provisioner:aabbccddeeff", MemoryMB: 30, CPUNanos: 10},
	)
	for i := 0; i < n; i++ {
		app.stats = append(app.stats, talos.StatsResult{ID: fmt.Sprintf("default/web-%d:nginx:0123456789ab", i), MemoryMB: 50, CPUNanos: 10})
	}
	app.prevStats = append([]talos.StatsResult(nil), app.stats...)
	app.prevStatsAt = time.Now().Add(-5 * time.Second)
	app.statsAt = time.Now()
	return app
}

func TestMetricsNamespaceColumn(t *testing.T) {
	app := metricsApp(120, 30, 0)
	out := stripANSI(app.renderMetrics(app.mainHeight()))
	if !strings.Contains(out, "NAMESPACE") {
		t.Fatalf("no NAMESPACE header:\n%s", out)
	}
	for _, want := range []string{"kube-system", "local-path-st...", " - "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	// The namespace is not repeated inside the container column.
	if strings.Contains(out, "kube-system/") {
		t.Errorf("namespace repeated in container column:\n%s", out)
	}
	// The system container row shows "-".
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "apid") {
			if f := strings.Fields(l); len(f) < 2 || f[1] != "-" {
				t.Errorf("apid row = %q, want namespace -", l)
			}
		}
	}
}

func TestMetricsRenderBudget(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 40}, {200, 50}} {
		app := metricsApp(sz[0], sz[1], 60)
		out := app.renderMetrics(app.mainHeight())
		if got := lineCount(out); got > app.mainHeight() {
			t.Errorf("%dx%d: %d rows > %d", sz[0], sz[1], got, app.mainHeight())
		}
		if w := maxLineWidth(out); w > sz[0] {
			t.Errorf("%dx%d: line width %d > %d", sz[0], sz[1], w, sz[0])
		}
		if !strings.Contains(stripANSI(out), "NAMESPACE") {
			t.Errorf("%dx%d: no NAMESPACE column", sz[0], sz[1])
		}
	}
}

func TestMetricsWidths(t *testing.T) {
	for _, w := range []int{60, 80, 120, 200} {
		id, ns, cpu, mem := metricsWidths(w)
		if total := 2 + id + 2 + ns + 2 + cpu + 2 + mem; w >= 80 && total > w {
			t.Errorf("width %d: columns total %d", w, total)
		}
		if id < 20 || id > 60 {
			t.Errorf("width %d: id column %d", w, id)
		}
	}
}
