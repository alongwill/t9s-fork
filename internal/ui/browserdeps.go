package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

// depEntry is the cached controller dependency graph of one node.
type depEntry struct {
	g       talos.DepGraph
	err     error
	loading bool
}

type depsMsg struct {
	node string
	g    talos.DepGraph
	err  error
}

func (app App) setDeps(node string, e depEntry) App {
	m := make(map[string]depEntry, len(app.deps)+1)
	for k, v := range app.deps {
		m[k] = v
	}
	m[node] = e
	app.deps = m
	return app
}

func (app App) loadDeps() tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
		defer cancel()
		g, err := src.Dependencies(ctx, node)
		return depsMsg{node: node, g: g, err: err}
	}
}

// ensureDeps loads the node's graph unless it is cached or on its way.
func (app App) ensureDeps() (App, tea.Cmd) {
	if _, ok := app.deps[app.browser.node.IP]; ok {
		return app, nil
	}
	app = app.setDeps(app.browser.node.IP, depEntry{loading: true})
	return app, app.loadDeps()
}

// reloadDeps forgets the cached graph and fetches it again (ctrl+r).
func (app App) reloadDeps() (App, tea.Cmd) {
	app = app.setDeps(app.browser.node.IP, depEntry{loading: true})
	return app, app.loadDeps()
}

func (app App) handleDeps(msg depsMsg) App {
	return app.setDeps(msg.node, depEntry{g: msg.g, err: msg.err})
}
