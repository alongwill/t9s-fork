package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/florianspk/t9s/internal/config"
	"github.com/florianspk/t9s/internal/talos"
)

func detailTestApp(width, height int) App {
	node := talos.Node{Hostname: "cp-1", IP: "10.0.0.2"}
	app := newTestApp(width, height)
	app.selNode = &node
	app.cfg = &config.TalosConfig{Context: "test"}
	app.state = StateContainers
	app.containers = []talos.ContainerInfo{
		{Namespace: "k8s.io", ID: "kube-system/coredns", Image: "registry.k8s.io/pause:3.8", PID: "1481", Status: "READY"},
		{Namespace: "k8s.io", ID: "kube-system/coredns:coredns:abc123def456", Image: "registry.k8s.io/coredns/coredns:v1.11.3@sha256:0123456789abcdef0123456789abcdef", PID: "1636", Status: "RUNNING"},
		{Namespace: "system", ID: "apid", Image: "ghcr.io/siderolabs/apid:v1.14.2", PID: "812", Status: "RUNNING"},
	}
	app.contCur = 1
	return app
}

func loadedDetail(app App) App {
	seq := app.detail.seq
	app = app.applyDetailMsg(detailStatsMsg{seq: seq, stat: &talos.StatsResult{ID: app.detail.c.ID, MemoryMB: 18.5, CPUNanos: 4210000000}})
	app = app.applyDetailMsg(detailMemMsg{seq: seq, totalMB: 3902})
	app = app.applyDetailMsg(detailProcsMsg{seq: seq, procs: []talos.ProcessInfo{
		{PID: "1", Command: "/sbin/init"},
		{PID: app.detail.c.PID, State: "S", CPUTime: "0:04", ResMem: "18 MB", Command: "/coredns -conf /etc/coredns/Corefile"},
	}})
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, "2026-10-03T12:00:00Z INFO line "+strings.Repeat("x", i))
	}
	lines[29] = "2026-10-03T12:00:00Z ERROR the newest line"
	return app.applyDetailMsg(detailLogsMsg{seq: seq, lines: lines})
}

func TestContainersEnterOpensDetail(t *testing.T) {
	app := detailTestApp(120, 40)
	got, cmd := app.handleContainersKey(lkey("enter"))
	if got.state != StateContainerDetail || cmd == nil {
		t.Fatalf("state %v, cmd nil %v", got.state, cmd == nil)
	}
	if got.detail.c.ID != app.containers[1].ID || !got.detail.id.CRI || got.detail.id.Name != "coredns" {
		t.Errorf("detail = %+v", got.detail)
	}
	if !got.detail.statsSec.loading || !got.detail.procsSec.loading || !got.detail.logsSec.loading {
		t.Errorf("sections not loading: %+v", got.detail)
	}
}

func TestContainerDetailRenderBudgets(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 40}, {200, 50}} {
		app, _ := detailTestApp(sz[0], sz[1]).handleContainersKey(lkey("enter"))
		for _, loaded := range []bool{false, true} {
			a := app
			if loaded {
				a = loadedDetail(app)
			}
			out := a.View()
			if got := strings.Count(out, "\n") + 1; got != sz[1] {
				t.Errorf("%dx%d loaded=%v: %d rows, want %d", sz[0], sz[1], loaded, got, sz[1])
			}
			if w := maxLineWidth(out); w > sz[0] {
				t.Errorf("%dx%d loaded=%v: line width %d", sz[0], sz[1], loaded, w)
			}
		}
		plain := stripANSI(loadedDetail(app).View())
		for _, want := range []string{"coredns", "RUNNING", "1636", "v1.11.3", "k8s.io", "18.5 MB", "% of 3.8 GB", "/coredns -conf", "the newest line"} {
			if !strings.Contains(plain, want) {
				t.Errorf("%dx%d: view lacks %q:\n%s", sz[0], sz[1], want, plain)
			}
		}
	}
}

func TestContainerDetailSectionsShowTheirOwnErrors(t *testing.T) {
	app, _ := detailTestApp(120, 40).handleContainersKey(lkey("enter"))
	seq := app.detail.seq
	app = app.applyDetailMsg(detailStatsMsg{seq: seq, err: context.DeadlineExceeded})
	app = app.applyDetailMsg(detailLogsMsg{seq: seq, err: context.DeadlineExceeded})
	plain := stripANSI(app.renderContainerDetail(40))
	if !strings.Contains(plain, "Stats: context deadline exceeded") ||
		!strings.Contains(plain, "Logs: context deadline exceeded") ||
		!strings.Contains(plain, "Loading processes…") {
		t.Errorf("sections not independent:\n%s", plain)
	}
}

func TestContainerDetailDropsStaleLoads(t *testing.T) {
	app, _ := detailTestApp(120, 40).handleContainersKey(lkey("enter"))
	old := app.detail.seq
	app, _ = app.handleContainerDetailKey(lkey("r"))
	if app.detail.seq == old {
		t.Fatal("r did not start a new load")
	}
	app = app.applyDetailMsg(detailLogsMsg{seq: old, lines: []string{"stale"}})
	if len(app.detail.logs) != 0 || !app.detail.logsSec.loading {
		t.Errorf("stale message applied: %+v", app.detail.logsSec)
	}
}

func TestContainerDetailEscKeepsListCursor(t *testing.T) {
	app, _ := detailTestApp(120, 40).handleContainersKey(lkey("enter"))
	app, _ = app.handleContainerDetailKey(lkey("esc"))
	if app.state != StateContainers || app.contCur != 1 || app.selNode == nil {
		t.Errorf("state %v cur %d selNode nil %v", app.state, app.contCur, app.selNode == nil)
	}
}

func TestContainerDetailLOpensStreamingLogs(t *testing.T) {
	type call struct{ node, ns, id string }
	calls := make(chan call, 1)
	app, _ := detailTestApp(120, 40).handleContainersKey(lkey("enter"))
	app.runContainerLogStream = func(_ context.Context, node, ns, id string, ch chan<- string) {
		calls <- call{node, ns, id}
		ch <- "first"
	}
	app, cmd := app.handleContainerDetailKey(lkey("l"))
	if app.state != StateLogs || cmd == nil || app.logService != "kube-system/coredns:coredns:abc123def456" {
		t.Fatalf("state %v service %q cmd nil %v", app.state, app.logService, cmd == nil)
	}
	msg := cmd()
	if _, ok := msg.(logLineMsg); !ok {
		t.Fatalf("first message %T", msg)
	}
	got := <-calls
	if got != (call{"10.0.0.2", "k8s.io", "kube-system/coredns:coredns:abc123def456"}) {
		t.Errorf("runner called with %+v", got)
	}
	app.stopLogs()

	// Esc from the logs returns to the detail view.
	app, _ = app.handleLogsKey(lkey("esc"))
	if app.state != StateContainerDetail {
		t.Errorf("esc from logs went to %v, want the detail view", app.state)
	}
}

func TestContainerDetailSandboxHasNoLogs(t *testing.T) {
	app := detailTestApp(120, 40)
	app.contCur = 0
	app, cmd := app.handleContainersKey(lkey("enter"))
	if cmd == nil || app.detail.logsSec.loading {
		t.Fatalf("sandbox should not load logs: %+v", app.detail.logsSec)
	}
	app, _ = app.handleContainerDetailKey(lkey("l"))
	if app.state != StateContainerDetail {
		t.Errorf("l on a sandbox left the detail view: %v", app.state)
	}
	if plain := stripANSI(app.renderContainerDetail(40)); !strings.Contains(plain, "pod sandbox") {
		t.Errorf("no sandbox note:\n%s", plain)
	}
}

func TestMatchContainerProcesses(t *testing.T) {
	procs := []talos.ProcessInfo{{PID: "1"}, {PID: "1636"}, {PID: "16360"}}
	if got := matchContainerProcesses(procs, "1636"); len(got) != 1 || got[0].PID != "1636" {
		t.Errorf("got %+v", got)
	}
	for _, pid := range []string{"", "0"} {
		if got := matchContainerProcesses(procs, pid); got != nil {
			t.Errorf("pid %q matched %+v", pid, got)
		}
	}
}
