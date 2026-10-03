package ui

import (
	"context"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

// Live watch of the open resource type (gRPC source only). It follows the t9s
// streaming pattern: a buffered channel, waitForResourceWatch re-armed per
// message, a CancelFunc on App and a watchSeq that drops events of an old
// watch. syncWatch is the single place that starts and stops it, called after
// every key and when the source arrives, so pop / ctrl+r / node change / quit
// all cancel through the same path.

const (
	flashDuration = time.Second
	watchBuffer   = 64
)

var nowFunc = time.Now // overridden in tests

type resourceWatchMsg struct {
	seq    uint64
	node   string
	typ    string
	ev     talos.WatchEvent
	closed bool
}

type flashExpiredMsg struct{}

// watchTarget returns the type the browser wants watched right now, if any:
// the nearest resource instances pane while only YAML / describe panes sit on
// top of it.
func (app App) watchTarget() (talos.ResourceDef, bool) {
	if app.watchOff || !app.hasWatch() || !isBrowserState(app.state) {
		return talos.ResourceDef{}, false
	}
	st := app.browser.stack
	for i := len(st) - 1; i >= 0; i-- {
		switch st[i].kind {
		case paneYAML, paneDescribe:
			continue
		case paneInstances:
			if st[i].cfgKind != "" {
				return talos.ResourceDef{}, false
			}
			return st[i].def, true
		}
		break
	}
	return talos.ResourceDef{}, false
}

func watchKeyOf(node, typ string) string { return node + "|" + typ }

// syncWatch starts, keeps or stops the watch so it matches watchTarget.
func (app App) syncWatch() (App, tea.Cmd) {
	def, want := app.watchTarget()
	if !want {
		if app.watchCancel != nil {
			app.stopWatch()
		}
		return app, nil
	}
	key := watchKeyOf(app.browser.node.IP, def.Type)
	if app.watchKey == key && app.watchCancel != nil {
		return app, nil // already running, or ended (ctrl+r / W restarts it)
	}
	return app.startWatch(def)
}

func (app App) startWatch(def talos.ResourceDef) (App, tea.Cmd) {
	app.stopWatch()
	node := app.browser.node.IP
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan talos.WatchEvent, watchBuffer)
	app.watchSeq++
	seq := app.watchSeq
	app.watchCh, app.watchCancel = ch, cancel
	app.watchKey = watchKeyOf(node, def.Type)
	app.watchLive, app.watchBootstrapped = true, false
	src := app.src()
	ns := def.DefaultNamespace
	go func() {
		defer close(ch)
		if err := src.Watch(ctx, node, ns, def.Type, ch); err != nil && ctx.Err() == nil {
			select {
			case ch <- talos.WatchEvent{Kind: "error", Err: err}:
			case <-ctx.Done():
			}
		}
	}()
	return app, waitForResourceWatch(ch, seq, node, def.Type)
}

// stopWatch cancels the running watch. Pointer receiver like stopLogs.
func (app *App) stopWatch() {
	if app.watchCancel != nil {
		app.watchCancel()
	}
	app.watchCh, app.watchCancel = nil, nil
	app.watchKey = ""
	app.watchLive, app.watchBootstrapped = false, false
}

func waitForResourceWatch(ch <-chan talos.WatchEvent, seq uint64, node, typ string) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return resourceWatchMsg{seq: seq, node: node, typ: typ, closed: true}
		}
		return resourceWatchMsg{seq: seq, node: node, typ: typ, ev: ev}
	}
}

func (app App) handleResourceWatch(msg resourceWatchMsg) (App, tea.Cmd) {
	if msg.seq != app.watchSeq || app.watchCh == nil || len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app, nil // an old watch: drop, and do not re-arm
	}
	rearm := waitForResourceWatch(app.watchCh, msg.seq, msg.node, msg.typ)
	if msg.closed {
		app.watchLive = false
		return app, nil
	}
	switch msg.ev.Kind {
	case "error":
		app.watchLive = false
		app.statusMsg = errStyle.Render("watch stopped: " + shortReason(msg.ev.Err))
		return app, rearm // drains the close
	case "bootstrapped":
		app.watchBootstrapped = true
		return app, rearm
	}
	var cmds []tea.Cmd
	cmds = append(cmds, rearm)
	var flash bool
	app, flash, cmds = app.applyWatchEvent(msg.typ, msg.ev, cmds)
	if flash {
		cmds = append(cmds, tea.Tick(flashDuration+50*time.Millisecond, func(time.Time) tea.Msg { return flashExpiredMsg{} }))
	}
	return app, tea.Batch(cmds...)
}

// applyWatchEvent folds one created/updated/destroyed event into the instances
// pane of typ and, for updates, reloads an open YAML of the same instance.
func (app App) applyWatchEvent(typ string, ev talos.WatchEvent, cmds []tea.Cmd) (App, bool, []tea.Cmd) {
	isPane := func(p pane) bool { return p.kind == paneInstances && p.cfgKind == "" && p.def.Type == typ }
	var def talos.ResourceDef
	found := false
	for _, p := range app.browser.stack {
		if isPane(p) {
			def, found = p.def, true
		}
	}
	if !found {
		return app, false, cmds
	}
	id := ev.Meta.ID
	var items []talos.ResourceMeta
	app.browser = app.browser.withPane(isPane, func(p *pane) {
		curID := ""
		if vis := filterInstances(p.items, p.filter); p.cur < len(vis) {
			curID = vis[p.cur].ID
		}
		p.items = applyInstanceEvent(p.items, ev)
		p.err, p.loading = "", false
		vis := filterInstances(p.items, p.filter)
		p.cur = clamp(p.cur, 0, max(0, len(vis)-1))
		for i, it := range vis { // keep the cursor on the same ID if it still exists
			if it.ID == curID {
				p.cur = i
				break
			}
		}
		rows := app.paneInnerRows(paneInstances)
		p.scroll = clampScrollStart(p.scroll, p.cur, len(vis), rows)
		items = p.items
	})
	app.browser = app.browser.setCount(typ, len(items))
	if len(items) == 1 {
		app.browser = app.browser.setSingle(typ, items[0], true)
	} else {
		app.browser = app.browser.setSingle(typ, talos.ResourceMeta{}, false)
	}

	flash := false
	if app.watchBootstrapped && ev.Kind != "destroyed" { // the bootstrap replays existing rows: no flash
		app.browser = app.browser.setFlash(id, nowFunc().Add(flashDuration))
		flash = true
	}

	yamlOpen := func(p pane) bool {
		return p.kind == paneYAML && p.cfgKind == "" && p.def.Type == typ && p.meta.ID == id
	}
	for _, p := range app.browser.stack {
		if !yamlOpen(p) || !app.watchBootstrapped {
			continue
		}
		switch ev.Kind {
		case "updated":
			cmds = append(cmds, app.loadYAML(def, p.meta)) // keeps the pane; scroll handled on reply
		case "destroyed":
			app.statusMsg = warnStyle.Render(id + " was destroyed")
		}
	}
	return app, flash, cmds
}

// applyInstanceEvent returns items with the event applied (copy, ID-sorted insert).
func applyInstanceEvent(items []talos.ResourceMeta, ev talos.WatchEvent) []talos.ResourceMeta {
	out := make([]talos.ResourceMeta, len(items), len(items)+1)
	copy(out, items)
	i := -1
	for j, it := range out {
		if it.ID == ev.Meta.ID {
			i = j
			break
		}
	}
	switch ev.Kind {
	case "created", "updated":
		if i >= 0 {
			out[i] = ev.Meta
			return out
		}
		at := sort.Search(len(out), func(j int) bool { return out[j].ID >= ev.Meta.ID })
		out = append(out, talos.ResourceMeta{})
		copy(out[at+1:], out[at:])
		out[at] = ev.Meta
	case "destroyed":
		if i >= 0 {
			out = append(out[:i], out[i+1:]...)
		}
	}
	return out
}

// --- flash ---

func (b browser) setFlash(id string, until time.Time) browser {
	now := nowFunc()
	m := make(map[string]time.Time, len(b.flash)+1)
	for k, v := range b.flash {
		if v.After(now) {
			m[k] = v
		}
	}
	m[id] = until
	b.flash = m
	return b
}

func (b browser) flashing(id string) bool {
	until, ok := b.flash[id]
	return ok && until.After(nowFunc())
}

// --- W key and indicator ---

func (app App) toggleWatch() (App, tea.Cmd) {
	if !app.hasWatch() {
		app.statusMsg = warnStyle.Render("watch needs the gRPC source")
		return app, nil
	}
	app.watchOff = !app.watchOff
	if app.watchOff {
		app.stopWatch()
		app.statusMsg = dimStyle.Render("watch off")
		return app, nil
	}
	app.statusMsg = dimStyle.Render("watch on")
	return app.syncWatch()
}

// watchIndicator is `watch` / `watch off`, empty when it does not apply.
func (app App) watchIndicator() string {
	if !app.hasWatch() {
		return ""
	}
	top, ok := app.browser.top()
	if !ok || (top.kind != paneInstances && top.kind != paneYAML && top.kind != paneDescribe) || top.cfgKind != "" {
		return ""
	}
	if app.watchOff {
		return "watch off"
	}
	if app.watchCancel != nil && app.watchLive {
		return "watch"
	}
	if app.watchCancel != nil {
		return "watch lost"
	}
	return "watch off"
}

func joinIndicators(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, "  ")
}
