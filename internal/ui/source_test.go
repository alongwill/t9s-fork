package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/talos"
)

func TestSourceReadySetsSource(t *testing.T) {
	app := newTestApp(120, 40)
	app.talosCtx = "prod"
	fs := newFakeSource("grpc")
	app = app.handleSourceReady(sourceReadyMsg{ctx: "prod", src: fs})
	if app.source != fs || app.sourceName() != "grpc" || !app.hasWatch() {
		t.Fatalf("source not installed: %v", app.source)
	}
}

func TestSourceReadyFailureFallsBackToCLIWithStatus(t *testing.T) {
	app := newTestApp(120, 40)
	app.talosCtx = "prod"
	app = app.handleSourceReady(sourceReadyMsg{ctx: "prod", err: errors.New("rpc error: code = Unavailable desc = connection refused\nsecond line")})
	if app.source != nil || app.sourceName() != "cli" || app.hasWatch() {
		t.Fatalf("expected CLI fallback, got %v", app.source)
	}
	if !strings.Contains(app.statusMsg, "gRPC unavailable (connection refused), using CLI") {
		t.Fatalf("status = %q", app.statusMsg)
	}
}

func TestSourceReadyForOtherContextIsDropped(t *testing.T) {
	app := newTestApp(120, 40)
	app.talosCtx = "staging"
	fs := newFakeSource("grpc")
	app = app.handleSourceReady(sourceReadyMsg{ctx: "prod", src: fs})
	if app.source != nil || !fs.closed.Load() {
		t.Fatal("stale connection must be closed and ignored")
	}
}

func TestResetSourceClosesAndClears(t *testing.T) {
	app := newTestApp(120, 40)
	fs := newFakeSource("grpc")
	app.source = fs
	app.sourceMode = SourceCLI
	app, cmd := app.resetSource()
	if app.source != nil || !fs.closed.Load() || cmd != nil {
		t.Fatalf("reset: source=%v closed=%v cmd=%v", app.source, fs.closed.Load(), cmd != nil)
	}
}

func TestConnectSourceUsesDialer(t *testing.T) {
	app := newTestApp(120, 40)
	app.talosCtx, app.cfgPath = "prod", "/tmp/cfg"
	fs := newFakeSource("grpc")
	var gotPath, gotCtx string
	app.dialSource = func(_ context.Context, p, c string) (talos.ResourceSource, error) {
		gotPath, gotCtx = p, c
		return fs, nil
	}
	cmd := app.connectSource()
	if cmd == nil {
		t.Fatal("auto mode must dial")
	}
	msg := cmd().(sourceReadyMsg)
	if msg.src != fs || gotPath != "/tmp/cfg" || gotCtx != "prod" || msg.ctx != "prod" {
		t.Fatalf("msg=%+v path=%q ctx=%q", msg, gotPath, gotCtx)
	}
	app.sourceMode = SourceCLI
	if app.connectSource() != nil {
		t.Fatal("--source=cli must not dial")
	}
}

func TestBrowserHeaderShowsSource(t *testing.T) {
	app := browserApp(120, 40, 2, 10)
	if got := resourceLine(app); !strings.HasSuffix(got, "src: cli") {
		t.Fatalf("cli header = %q", got)
	}
	app.source = newFakeSource("grpc")
	line := resourceLine(app)
	if !strings.HasSuffix(line, "src: grpc") || !strings.Contains(line, "node: ") {
		t.Fatalf("grpc header = %q", line)
	}
	for _, w := range []int{80, 120, 200} {
		app.width = w
		if got := lipglossWidth(resourceLine(app)); got > w-2 {
			t.Errorf("width %d: header is %d cells", w, got)
		}
	}
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }
