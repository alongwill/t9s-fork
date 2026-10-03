package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

// J (k9s: jump to owner): from an instance, go to what the controller that
// wrote it reads. One input type: jump straight there. Several: open the
// related view with those inputs highlighted.

// ownerSubject is the instance J was pressed on.
func (app App) ownerSubject() (talos.ResourceDef, talos.ResourceMeta, string, bool) {
	p, ok := app.browser.top()
	if !ok {
		return talos.ResourceDef{}, talos.ResourceMeta{}, "", false
	}
	switch p.kind {
	case paneInstances:
		if p.cfgKind != "" {
			return talos.ResourceDef{}, talos.ResourceMeta{}, "config documents have no owner controller", false
		}
		items := filterInstances(p.items, p.filter)
		if p.cur >= len(items) {
			return talos.ResourceDef{}, talos.ResourceMeta{}, "nothing selected", false
		}
		return p.def, items[p.cur], "", true
	case paneYAML:
		if p.cfgKind != "" {
			return talos.ResourceDef{}, talos.ResourceMeta{}, "config documents have no owner controller", false
		}
		return p.def, p.meta, "", true
	case paneDescribe:
		if p.sub.cfg || !p.sub.hasMeta {
			return talos.ResourceDef{}, talos.ResourceMeta{}, "J needs an instance: open describe (d) from an instance list", false
		}
		return p.sub.def, p.sub.meta, "", true
	}
	return talos.ResourceDef{}, talos.ResourceMeta{}, "", false
}

// jumpToWriter implements `J`.
func (app App) jumpToWriter() (App, tea.Cmd) {
	d, m, why, ok := app.ownerSubject()
	if !ok {
		if why != "" {
			app.statusMsg = warnStyle.Render(why)
		}
		return app, nil
	}
	if m.Owner == "" {
		app.statusMsg = dimStyle.Render(fmt.Sprintf("%s has no owner controller: it was created through the API or by apply-config", m.ID))
		return app, nil
	}
	g, note := app.depGraph()
	if note != "" {
		var cmd tea.Cmd
		app, cmd = app.ensureDeps() // press J again once the graph is in
		app.statusMsg = dimStyle.Render(note)
		return app, cmd
	}
	var inputs []string
	for _, e := range g.Inputs(m.Owner) {
		inputs = append(inputs, e.Type)
	}
	switch len(inputs) {
	case 0:
		app.statusMsg = dimStyle.Render(fmt.Sprintf("%s reads no resource types", m.Owner))
		return app, nil
	case 1:
		e, ok := app.browser.lookupExact(inputs[0])
		if !ok {
			app.statusMsg = dimStyle.Render(fmt.Sprintf("%s reads %s, which this node does not have", m.Owner, displayFromType(inputs[0])))
			return app, nil
		}
		app.statusMsg = infoStyle.Render(fmt.Sprintf("%s (writer of %s) reads %s", m.Owner, m.ID, e.name))
		return app.jumpTo(e)
	}
	hi := make(map[string]bool, len(inputs))
	for _, t := range inputs {
		hi[t] = true
	}
	app, cmd := app.pushRelated(subjectOfDef(d), hi)
	note = fmt.Sprintf("highlighted: the %d types %s reads (it wrote %s)", len(inputs), m.Owner, m.ID)
	app = app.setRel(func(rv *relatedView) { rv.hiNote = note })
	return app, cmd
}
