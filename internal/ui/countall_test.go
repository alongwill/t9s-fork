package ui

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

// collectMsgs runs a cmd (expanding tea.Batch) and returns every message it
// yields. Blocking cmds such as waitFor* are not expected here.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Name() == "BatchMsg" {
		// like the runtime: every command of a batch runs concurrently
		var (
			mu  sync.Mutex
			wg  sync.WaitGroup
			out []tea.Msg
		)
		for i := 0; i < v.Len(); i++ {
			c, ok := v.Index(i).Interface().(tea.Cmd)
			if !ok {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				got := collectMsgs(c)
				mu.Lock()
				out = append(out, got...)
				mu.Unlock()
			}()
		}
		wg.Wait()
		return out
	}
	return []tea.Msg{msg}
}

// feed applies messages through Update and returns the resulting App.
func feed(t *testing.T, app App, msgs ...tea.Msg) App {
	t.Helper()
	for _, m := range msgs {
		model, _ := app.Update(m)
		app = model.(App)
	}
	return app
}

func countAllApp(source talos.ResourceSource, nTypes int) (App, []talos.ResourceDef) {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(3)
	defs, _ := makeBrowserDefs(nTypes)
	app.source = source
	app.resSem = make(chan struct{}, 8)
	n := app.nodes[0]
	app.browser = browser{node: n, stack: []pane{{kind: paneCategories, title: "Categories"}}, cfgState: cfgDenied}
	app = app.syncBrowserState()
	return app, defs
}

func TestGRPCCountsEverythingWhenDefsArrive(t *testing.T) {
	fs := newFakeSource("grpc")
	app, defs := countAllApp(fs, 12)
	for i, d := range defs {
		if i%2 == 0 {
			fs.lists["10.0.0.1|"+d.Type] = sampleMetas(i + 1)
		}
	}
	app, cmd := app.handleResourceDefs(resourceDefsMsg{node: "10.0.0.1", defs: defs})
	if cmd == nil {
		t.Fatal("grpc source must start counting every type")
	}
	for _, d := range defs {
		if !app.browser.loading[d.Type] {
			t.Errorf("%s not marked loading", d.DisplayType)
		}
	}
	app = feed(t, app, collectMsgs(cmd)...)
	if got := int(fs.listCalls.Load()); got != len(defs) {
		t.Fatalf("list calls = %d, want %d (one per type, no category opened)", got, len(defs))
	}
	for i, d := range defs {
		want := 0
		if i%2 == 0 {
			want = i + 1
		}
		if got, ok := app.browser.counts[d.Type]; !ok || got != want {
			t.Errorf("%s count = %d (ok=%v), want %d", d.DisplayType, got, ok, want)
		}
	}
	rows := app.browser.categoryRows("")
	var net catRow
	for _, r := range rows {
		if r.key == testNet {
			net = r
		}
	}
	if !net.counted || net.present != 6 || net.known != 12 {
		t.Fatalf("categories row = %+v, want counted 6/12 without opening it", net)
	}
}

func TestCLIKeepsLazyCounting(t *testing.T) {
	fs := newFakeSource("cli")
	app, defs := countAllApp(fs, 12)
	app, cmd := app.handleResourceDefs(resourceDefsMsg{node: "10.0.0.1", defs: defs})
	if cmd != nil || fs.listCalls.Load() != 0 || len(app.browser.loading) != 0 {
		t.Fatalf("cli source must not count up front (cmd=%v calls=%d loading=%d)", cmd != nil, fs.listCalls.Load(), len(app.browser.loading))
	}
	// Opening a category still counts just its types.
	app = press(t, app, "enter")
	if len(app.browser.stack) != 2 {
		t.Fatalf("stack = %d", len(app.browser.stack))
	}
	if len(app.browser.loading) != 12 {
		t.Fatalf("loading = %d, want the category's 12 types", len(app.browser.loading))
	}
}

func TestGRPCCountsAreCappedAtEightInFlight(t *testing.T) {
	fs := newBlockingSource()
	app, defs := countAllApp(fs, 30)
	_, cmd := app.handleResourceDefs(resourceDefsMsg{node: "10.0.0.1", defs: defs})
	done := make(chan struct{})
	go func() { collectMsgs(cmd); close(done) }()
	deadline := time.After(3 * time.Second)
	for fs.started.Load() < 8 {
		select {
		case <-deadline:
			t.Fatalf("only %d lists started", fs.started.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
	time.Sleep(50 * time.Millisecond)
	if got := fs.started.Load(); got != 8 {
		t.Fatalf("in flight = %d, want exactly 8", got)
	}
	close(fs.release)
	<-done
	if fs.peak.Load() > 8 {
		t.Fatalf("peak concurrency %d > 8", fs.peak.Load())
	}
}

func TestLateGRPCSourceCountsOpenBrowser(t *testing.T) {
	app, defs := countAllApp(nil, 6)
	app.talosCtx = "prod"
	app.browser.defs = defs
	fs := newFakeSource("grpc")
	app, cmd := app.handleSourceReady(sourceReadyMsg{ctx: "prod", src: fs})
	if cmd == nil || len(app.browser.loading) != 6 {
		t.Fatalf("late dial must count the open browser (cmd=%v loading=%d)", cmd != nil, len(app.browser.loading))
	}
}

func TestOpenBrowserWithCachedDefsCountsAll(t *testing.T) {
	fs := newFakeSource("grpc")
	app, defs := countAllApp(fs, 6)
	app.resourceDefs = map[string][]talos.ResourceDef{"10.0.0.1": defs}
	app, cmd := app.openBrowser(app.nodes[0])
	if cmd == nil || len(app.browser.loading) != 6 {
		t.Fatalf("cached defs path must count everything (cmd=%v loading=%d)", cmd != nil, len(app.browser.loading))
	}
}

// blockingSource holds every List until release is closed.
type blockingSource struct {
	*fakeSource
	started, cur, peak atomic.Int32
	release            chan struct{}
}

func newBlockingSource() *blockingSource {
	return &blockingSource{fakeSource: newFakeSource("grpc"), release: make(chan struct{})}
}

func (b *blockingSource) List(ctx context.Context, node, ns, typ string) ([]talos.ResourceMeta, error) {
	b.started.Add(1)
	n := b.cur.Add(1)
	for {
		p := b.peak.Load()
		if n <= p || b.peak.CompareAndSwap(p, n) {
			break
		}
	}
	<-b.release
	b.cur.Add(-1)
	return nil, nil
}
