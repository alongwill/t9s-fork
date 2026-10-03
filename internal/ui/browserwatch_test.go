package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

// watchApp is a browser at depth 3 (instances of Thing00) on a gRPC source
// whose watch has been started.
func watchApp(t *testing.T, depth int) (App, *fakeSource) {
	t.Helper()
	fs := newFakeSource("grpc")
	app := browserApp(120, 40, depth, 10)
	app.source = fs
	app.browser = app.browser.withPane(func(p pane) bool { return p.kind == paneInstances }, func(p *pane) {
		p.items = sampleMetas(5)
	})
	app, cmd := app.syncWatch()
	if cmd == nil {
		t.Fatal("syncWatch did not start a watch")
	}
	select {
	case <-fs.watchReady:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch was never called on the source")
	}
	t.Cleanup(func() { app.stopWatch() })
	return app, fs
}

func watchMsg(app App, kind string, m talos.ResourceMeta) resourceWatchMsg {
	return resourceWatchMsg{seq: app.watchSeq, node: app.browser.node.IP, typ: "Thing00.net.talos.dev", ev: talos.WatchEvent{Kind: kind, Meta: m}}
}

func instMeta(id, ver string) talos.ResourceMeta {
	return talos.ResourceMeta{Namespace: "network", Type: "Thing00.net.talos.dev", ID: id, Version: ver, Phase: "running"}
}

func instancesOf(app App) pane {
	for _, p := range app.browser.stack {
		if p.kind == paneInstances {
			return p
		}
	}
	return pane{}
}

func ids(items []talos.ResourceMeta) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}

func TestWatchStartsForInstancesPaneOnly(t *testing.T) {
	app, _ := watchApp(t, 3)
	if app.watchKey != "10.0.0.2|Thing00.net.talos.dev" || !app.watchLive {
		t.Fatalf("watch state: key=%q live=%v", app.watchKey, app.watchLive)
	}
	if got := app.watchIndicator(); got != "watch" {
		t.Errorf("indicator = %q", got)
	}
	if !strings.Contains(resourceLine(app), "watch  src: grpc") {
		t.Errorf("header = %q", resourceLine(app))
	}
	// types pane: nothing to watch
	types := browserApp(120, 40, 2, 10)
	types.source = newFakeSource("grpc")
	if _, cmd := types.syncWatch(); cmd != nil {
		t.Error("types pane must not start a watch")
	}
	if types.watchIndicator() != "" {
		t.Errorf("indicator on types pane = %q", types.watchIndicator())
	}
}

func TestWatchAlsoRunsUnderYAMLPane(t *testing.T) {
	app, _ := watchApp(t, 4)
	if app.watchCancel == nil || app.watchKey == "" {
		t.Fatal("watch must keep running with the YAML pane on top")
	}
	if app.watchIndicator() != "watch" {
		t.Errorf("indicator = %q", app.watchIndicator())
	}
}

func TestWatchEventsApplyToInstances(t *testing.T) {
	app, _ := watchApp(t, 3)
	base := instancesOf(app).items
	if len(base) != 5 {
		t.Fatalf("fixture has %d items", len(base))
	}
	var cmd tea.Cmd

	// bootstrap replay: upsert silently, no flash
	app, _ = app.handleResourceWatch(watchMsg(app, "created", base[0]))
	app, _ = app.handleResourceWatch(watchMsg(app, "bootstrapped", talos.ResourceMeta{}))
	if len(app.browser.flash) != 0 || !app.watchBootstrapped {
		t.Fatalf("bootstrap flashed or not marked: flash=%v boot=%v", app.browser.flash, app.watchBootstrapped)
	}

	// created: sorted insert + flash + count
	app, cmd = app.handleResourceWatch(watchMsg(app, "created", instMeta("aaa", "1")))
	if cmd == nil {
		t.Fatal("a flash needs a re-arm and an expiry tick")
	}
	items := instancesOf(app).items
	if len(items) != 6 || items[0].ID != "aaa" {
		t.Fatalf("created: %v", ids(items))
	}
	if !app.browser.flashing("aaa") {
		t.Error("created row must flash")
	}
	if app.browser.counts["Thing00.net.talos.dev"] != 6 {
		t.Errorf("count = %d", app.browser.counts["Thing00.net.talos.dev"])
	}

	// updated: replaced in place
	upd := items[3]
	upd.Version = "99"
	app, _ = app.handleResourceWatch(watchMsg(app, "updated", upd))
	got := instancesOf(app).items
	if len(got) != 6 || got[3].Version != "99" {
		t.Fatalf("updated: %+v", got[3])
	}
	if !app.browser.flashing(upd.ID) {
		t.Error("updated row must flash")
	}

	// destroyed: removed, no flash for it
	gone := got[4]
	app, _ = app.handleResourceWatch(watchMsg(app, "destroyed", gone))
	after := instancesOf(app).items
	if len(after) != 5 {
		t.Fatalf("destroyed: %v", ids(after))
	}
	for _, it := range after {
		if it.ID == gone.ID {
			t.Fatal("destroyed row still listed")
		}
	}
}

func TestWatchKeepsCursorOnSameID(t *testing.T) {
	app, _ := watchApp(t, 3)
	app, _ = app.handleResourceWatch(watchMsg(app, "bootstrapped", talos.ResourceMeta{}))
	items := instancesOf(app).items
	app.browser = app.browser.withTop(func(p *pane) { p.cur = 3 })
	want := items[3].ID

	// a row inserted above the cursor moves it down
	app, _ = app.handleResourceWatch(watchMsg(app, "created", instMeta("aaa", "1")))
	p := instancesOf(app)
	if p.items[p.cur].ID != want || p.cur != 4 {
		t.Fatalf("after insert cur=%d on %q, want 4 on %q", p.cur, p.items[p.cur].ID, want)
	}
	// a row removed above the cursor moves it up
	app, _ = app.handleResourceWatch(watchMsg(app, "destroyed", instMeta("aaa", "1")))
	p = instancesOf(app)
	if p.items[p.cur].ID != want || p.cur != 3 {
		t.Fatalf("after removal cur=%d on %q, want 3 on %q", p.cur, p.items[p.cur].ID, want)
	}
	// the selected row itself destroyed: cursor stays in range
	app, _ = app.handleResourceWatch(watchMsg(app, "destroyed", items[3]))
	p = instancesOf(app)
	if p.cur < 0 || p.cur >= len(p.items) {
		t.Fatalf("cursor %d out of range for %d items", p.cur, len(p.items))
	}
}

func TestWatchDropsStaleSeq(t *testing.T) {
	app, _ := watchApp(t, 3)
	before := ids(instancesOf(app).items)
	msg := watchMsg(app, "created", instMeta("zzz", "1"))
	msg.seq = app.watchSeq - 1
	app2, cmd := app.handleResourceWatch(msg)
	if cmd != nil {
		t.Error("a stale event must not re-arm the wait")
	}
	if got := ids(instancesOf(app2).items); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Fatalf("stale event applied: %v", got)
	}
	// after a restart the old sequence is stale too
	old := app.watchSeq
	app.stopWatch()
	app, _ = app.startWatch(instancesOf(app).def)
	if app.watchSeq == old {
		t.Fatal("watchSeq must change on restart")
	}
	stale := watchMsg(app, "created", instMeta("zzz", "1"))
	stale.seq = old
	if _, cmd := app.handleResourceWatch(stale); cmd != nil {
		t.Error("event of the previous watch must be dropped")
	}
}

func TestWatchCancelledOnEsc(t *testing.T) {
	app, fs := watchApp(t, 3)
	ctx := fs.watchCtx
	model, _ := app.Update(key("esc")) // pops the instances pane
	app = model.(App)
	if app.watchCancel != nil || app.watchKey != "" {
		t.Fatalf("watch still registered after esc: key=%q", app.watchKey)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("watch context not cancelled by esc")
	}
}

func TestWatchCancelledWhenLeavingBrowserAndOnCleanup(t *testing.T) {
	app, fs := watchApp(t, 3)
	ctx := fs.watchCtx
	app.cleanup()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not cancel the watch")
	}
}

func TestWatchRestartsOnCtrlR(t *testing.T) {
	app, fs := watchApp(t, 3)
	old, first := app.watchSeq, fs.watchCtx
	model, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	app = model.(App)
	select {
	case <-fs.watchReady:
	case <-time.After(2 * time.Second):
		t.Fatal("ctrl+r did not restart the watch")
	}
	t.Cleanup(func() { app.stopWatch() })
	if app.watchSeq == old || first.Err() == nil {
		t.Fatalf("seq %d→%d, old ctx err=%v", old, app.watchSeq, first.Err())
	}
}

func TestWKeyTogglesAndCLIExplains(t *testing.T) {
	app, fs := watchApp(t, 3)
	ctx := fs.watchCtx
	model, _ := app.Update(key("W"))
	app = model.(App)
	if !app.watchOff || app.watchCancel != nil || app.watchIndicator() != "watch off" {
		t.Fatalf("W did not turn the watch off: off=%v ind=%q", app.watchOff, app.watchIndicator())
	}
	if ctx.Err() == nil {
		t.Error("watch ctx not cancelled by W")
	}
	model, _ = app.Update(key("W"))
	app = model.(App)
	select {
	case <-fs.watchReady:
	case <-time.After(2 * time.Second):
		t.Fatal("W did not turn the watch back on")
	}
	t.Cleanup(func() { app.stopWatch() })
	if app.watchOff || app.watchIndicator() != "watch" {
		t.Fatalf("watch not back on: off=%v ind=%q", app.watchOff, app.watchIndicator())
	}

	cli := browserApp(120, 40, 3, 10)
	model, cmd := cli.Update(key("W"))
	cli = model.(App)
	if cmd != nil && len(collectMsgs(cmd)) > 0 {
		t.Error("cli W must not start anything")
	}
	if !strings.Contains(cli.statusMsg, "watch needs the gRPC source") {
		t.Fatalf("status = %q", cli.statusMsg)
	}
	if cli.watchIndicator() != "" {
		t.Errorf("cli indicator = %q", cli.watchIndicator())
	}
}

func TestWatchErrorStopsWithoutRestartLoop(t *testing.T) {
	app, _ := watchApp(t, 3)
	msg := watchMsg(app, "error", talos.ResourceMeta{})
	msg.ev.Err = context.DeadlineExceeded
	app, _ = app.handleResourceWatch(msg)
	if app.watchLive || !strings.Contains(app.statusMsg, "watch stopped") {
		t.Fatalf("live=%v status=%q", app.watchLive, app.statusMsg)
	}
	if app.watchIndicator() != "watch lost" {
		t.Errorf("indicator = %q", app.watchIndicator())
	}
	seq := app.watchSeq
	app, cmd := app.syncWatch()
	if cmd != nil || app.watchSeq != seq {
		t.Fatal("an ended watch must not restart by itself")
	}
}

func TestWatchYAMLReloadsOnUpdateAndKeepsScroll(t *testing.T) {
	app, fs := watchApp(t, 4)
	typ := "Thing00.net.talos.dev"
	meta := sampleMetas(1)[0]
	// the open YAML belongs to instance meta.ID
	app.browser = app.browser.withTop(func(p *pane) {
		p.meta = meta
		p.yaml = strings.Repeat("a: 1\n", 80)
		p.scroll = 17
	})
	fs.yamls["10.0.0.2|"+typ+"|"+meta.ID] = strings.Repeat("a: 2\n", 80) // same line count
	app, _ = app.handleResourceWatch(watchMsg(app, "bootstrapped", talos.ResourceMeta{}))

	app, cmd := app.handleResourceWatch(watchMsg(app, "updated", meta))
	var yamlMsg *resourceYAMLMsg
	for _, m := range collectMsgsNonBlocking(cmd) {
		if y, ok := m.(resourceYAMLMsg); ok {
			yamlMsg = &y
		}
	}
	if yamlMsg == nil {
		t.Fatal("an update of the open instance must reload its YAML")
	}
	app = app.handleResourceYAML(*yamlMsg)
	top, _ := app.browser.top()
	if !strings.HasPrefix(top.yaml, "a: 2") || top.scroll != 17 || top.loading {
		t.Fatalf("yaml reloaded wrongly: scroll=%d loading=%v", top.scroll, top.loading)
	}

	// a shorter document clamps the scroll instead of leaving it past the end
	app.browser = app.browser.withTop(func(p *pane) { p.scroll = 60 })
	app = app.handleResourceYAML(resourceYAMLMsg{node: "10.0.0.2", typ: typ, id: meta.ID, yaml: "a: 3\nb: 4\n"})
	if top, _ = app.browser.top(); top.scroll != 0 {
		t.Fatalf("scroll = %d after the document shrank, want 0", top.scroll)
	}

	// an update of another instance does not touch the YAML pane
	_, cmd = app.handleResourceWatch(watchMsg(app, "updated", instMeta("other", "5")))
	for _, m := range collectMsgsNonBlocking(cmd) {
		if _, ok := m.(resourceYAMLMsg); ok {
			t.Fatal("YAML reloaded for an unrelated instance")
		}
	}
}

// collectMsgsNonBlocking runs the cmds of a batch except the blocking
// waitForResourceWatch / tick ones (they are identified by timing out).
func collectMsgsNonBlocking(cmd tea.Cmd) []tea.Msg {
	done := make(chan []tea.Msg, 1)
	go func() { done <- collectMsgsSkippingBlocking(cmd) }()
	select {
	case m := <-done:
		return m
	case <-time.After(3 * time.Second):
		return nil
	}
}

func collectMsgsSkippingBlocking(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if b, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, collectMsgsSkippingBlocking(c)...)
			}
			return out
		}
		return []tea.Msg{msg}
	case <-time.After(150 * time.Millisecond):
		return nil // a blocking wait (watch channel / tick)
	}
}

func TestFlashExpires(t *testing.T) {
	app, _ := watchApp(t, 3)
	now := time.Now()
	nowFunc = func() time.Time { return now }
	t.Cleanup(func() { nowFunc = time.Now })
	app, _ = app.handleResourceWatch(watchMsg(app, "bootstrapped", talos.ResourceMeta{}))
	app, _ = app.handleResourceWatch(watchMsg(app, "updated", instMeta("eth0/10.0.0.0/24", "7")))
	if !app.browser.flashing("eth0/10.0.0.0/24") {
		t.Fatal("expected a flash")
	}
	now = now.Add(flashDuration + time.Millisecond)
	if app.browser.flashing("eth0/10.0.0.0/24") {
		t.Fatal("flash must expire after ~1s")
	}
}
