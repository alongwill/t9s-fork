package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

const browserTimeout = 10 * time.Second

func (app App) loadResourceDefs() tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
		defer cancel()
		defs, err := src.Definitions(ctx, node)
		return resourceDefsMsg{node: node, defs: defs, err: err}
	}
}

// loadCounts counts instances of every given type not already counted or in
// flight. One cmd per type, at most 8 running at once (resSem is shared by all
// batches). The caller's browser copy is updated via markCountsLoading.
func (app App) loadCounts(types []talos.ResourceDef) tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	sem := app.resSem
	if sem == nil { // bare App in tests
		sem = make(chan struct{}, 8)
	}
	var cmds []tea.Cmd
	for _, d := range types {
		if _, done := app.browser.counts[d.Type]; done || app.browser.loading[d.Type] {
			continue
		}
		d := d
		cmds = append(cmds, func() tea.Msg {
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
			defer cancel()
			items, err := src.List(ctx, node, d.DefaultNamespace, d.Type)
			msg := resourceCountMsg{node: node, typ: d.Type, n: len(items)}
			if len(items) == 1 {
				msg.only = items[0]
			}
			if err != nil {
				msg.n = 0
				msg.locked = talos.IsPermissionDenied(err)
				if !msg.locked {
					msg.err = err
				}
			}
			return msg
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// countAllTypes counts every type up front. Only the gRPC source does this:
// one connection makes it cheap. The CLI source keeps the lazy per-category
// counting (one subprocess per type).
func (app App) countAllTypes() (App, tea.Cmd) {
	if app.sourceName() != "grpc" || len(app.browser.stack) == 0 || len(app.browser.defs) == 0 {
		return app, nil
	}
	cmd := app.loadCounts(app.browser.defs)
	app.browser = app.browser.markCountsLoading(app.browser.defs)
	return app, cmd
}

// markCountsLoading flags the types loadCounts will fetch, so rows show `…`.
func (b browser) markCountsLoading(types []talos.ResourceDef) browser {
	for _, d := range types {
		if _, done := b.counts[d.Type]; done || b.loading[d.Type] {
			continue
		}
		b = b.setLoading(d.Type, true)
	}
	return b
}

func (app App) loadInstances(d talos.ResourceDef) tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
		defer cancel()
		items, err := src.List(ctx, node, d.DefaultNamespace, d.Type)
		return resourceInstancesMsg{node: node, typ: d.Type, items: items, err: err}
	}
}

func (app App) loadYAML(d talos.ResourceDef, m talos.ResourceMeta) tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	ns := m.Namespace
	if ns == "" {
		ns = d.DefaultNamespace
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
		defer cancel()
		y, err := src.GetYAML(ctx, node, ns, d.Type, m.ID)
		return resourceYAMLMsg{node: node, typ: d.Type, id: m.ID, yaml: y, err: err}
	}
}

func (app App) handleResourceDefs(msg resourceDefsMsg) (App, tea.Cmd) {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app, nil
	}
	app.browser.defsLoading = false
	if msg.err != nil {
		app.browser.defsErr = msg.err.Error()
		if w := app.browser.pendingCmd; w != "" { // the awaited type can't be resolved
			app.browser.pendingCmd = ""
			return app.unknownCommand(w), nil
		}
		app.statusMsg = errStyle.Render("Error: " + msg.err.Error())
		return app, nil
	}
	cache := make(map[string][]talos.ResourceDef, len(app.resourceDefs)+1)
	for k, v := range app.resourceDefs {
		cache[k] = v
	}
	cache[msg.node] = msg.defs
	app.resourceDefs = cache
	app.browser.defs = msg.defs
	app.browser.defsErr = ""
	app.statusMsg = ""
	var countCmd tea.Cmd
	app, countCmd = app.countAllTypes()
	if app.browser.pendingCmd != "" {
		a, cmd := app.runPendingCommand()
		return a, tea.Batch(countCmd, cmd)
	}
	if app.hasPalette() { // the palette was opened before the definitions arrived
		a, cmd := app.paletteCounts()
		return a, tea.Batch(countCmd, cmd)
	}
	return app, countCmd
}

func (app App) handleResourceCount(msg resourceCountMsg) App {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app
	}
	b := app.browser.setLoading(msg.typ, false)
	switch {
	case msg.locked:
		b = b.setCount(msg.typ, countLocked)
	case msg.err != nil:
		b = b.setCount(msg.typ, countError)
	default:
		b = b.setCount(msg.typ, msg.n)
		b = b.setSingle(msg.typ, msg.only, msg.n == 1)
	}
	app.browser = b
	return app
}

func (app App) handleResourceInstances(msg resourceInstancesMsg) App {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app
	}
	app.browser = app.browser.withPane(
		func(p pane) bool { return p.kind == paneInstances && p.def.Type == msg.typ },
		func(p *pane) {
			p.loading = false
			if msg.err != nil {
				p.err = msg.err.Error()
				return
			}
			p.err = ""
			p.items = msg.items
			p.cur = clamp(p.cur, 0, max(0, len(filterInstances(msg.items, p.filter))-1))
		})
	if msg.err == nil {
		// the fresh list also refreshes the type's count
		app.browser = app.browser.setCount(msg.typ, len(msg.items))
		if len(msg.items) == 1 {
			app.browser = app.browser.setSingle(msg.typ, msg.items[0], true)
		} else {
			app.browser = app.browser.setSingle(msg.typ, talos.ResourceMeta{}, false)
		}
	}
	return app
}

func (app App) handleResourceYAML(msg resourceYAMLMsg) App {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app
	}
	app.browser = app.browser.withPane(
		func(p pane) bool { return p.kind == paneYAML && p.def.Type == msg.typ && p.meta.ID == msg.id },
		func(p *pane) {
			p.loading = false
			if msg.err != nil {
				p.err = msg.err.Error()
				return
			}
			p.err = ""
			p.yaml = msg.yaml
		})
	if top, ok := app.browser.top(); ok && top.kind == paneYAML && top.meta.ID == msg.id && app.browser.find != "" {
		app.browser.findHits = yamlFindHits(top.yaml, app.browser.find)
		app.browser.findIdx = clamp(app.browser.findIdx, 0, max(0, len(app.browser.findHits)-1))
	}
	return app
}
