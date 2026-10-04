package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/diskmodel"
	"github.com/florianspk/t9s/internal/netmodel"
	"github.com/florianspk/t9s/internal/talos"
)

// Disk view (`i` on the node list, `:disks`): every physical disk of a node as
// a bar split into its partitions, sized in proportion and coloured by what
// Talos uses each for, with a table for the selected segment. The picture comes
// from diskmodel; this file holds the pane state, loading and keys,
// diskviewrender.go draws it. Like the network view it is a pane on the
// browser stack, so jumps (YAML, describe, related) return here with Esc.

const diskFetchTimeout = 30 * time.Second

type diskView struct {
	seq     uint64
	ready   bool
	loading bool
	res     diskmodel.FetchResult
	model   diskmodel.Model

	disk   int  // selected disk, among the shown ones
	seg    int  // selected segment of that disk
	scroll int  // first visible line of the disk list
	all    bool // `a`: loop, cdrom, read-only and zram devices too
	si     bool // `u`: decimal units (GB) instead of binary (GiB)
}

type diskFetchMsg struct {
	node string
	seq  uint64
	res  diskmodel.FetchResult
}

func (app App) diskPane() (pane, bool) {
	p, ok := app.browser.top()
	if !ok || p.kind != paneDisks {
		return pane{}, false
	}
	return p, true
}

func (app App) setDisk(f func(dv *diskView)) App {
	app.browser = app.browser.withTop(func(p *pane) {
		if p.kind == paneDisks {
			f(&p.disk)
		}
	})
	return app
}

// --- opening and loading ---

// openDisks pushes the disk view. root makes it the only pane (from the node
// list, so Esc returns straight to the node).
func (app App) openDisks(root bool) (App, tea.Cmd) {
	if !isBrowserState(app.state) || len(app.browser.stack) == 0 {
		return app, nil
	}
	if p, ok := app.browser.top(); ok && p.kind == paneDisks {
		return app, nil
	}
	dp := pane{kind: paneDisks, title: "Disks", disk: diskView{seq: 1, loading: true}}
	if root {
		app.browser.stack = []pane{dp}
	} else {
		app.browser = app.browser.clearFind().push(dp)
	}
	app.browser.fullscreen = false
	app.statusMsg = ""
	app = app.syncBrowserState()
	return app, app.loadDisks(1)
}

// openDisksFromList implements `i` on the node list.
func (app App) openDisksFromList(n talos.Node) (App, tea.Cmd) {
	app, openCmd := app.openBrowser(n)
	app, cmd := app.openDisks(true)
	return app, tea.Batch(openCmd, cmd)
}

// usageFunc reads mount usage through the subprocess client; nil without one.
func (app App) usageFunc() diskmodel.UsageFunc {
	c := app.client
	if c == nil {
		return nil
	}
	return func(ctx context.Context, node string) (map[string]diskmodel.Usage, error) {
		ms, err := c.GetMounts(ctx, node)
		if err != nil {
			return nil, err
		}
		out := make(map[string]diskmodel.Usage, len(ms))
		for _, m := range ms {
			u := diskmodel.Usage{Size: m.Size, Used: m.Used, Avail: m.Avail}
			out[m.MountedOn] = u
			if _, dup := out[m.Filesystem]; strings.HasPrefix(m.Filesystem, "/dev/") && !dup {
				out[m.Filesystem] = u // first mount of a device wins (/var, not its bind mounts)
			}
		}
		return out, nil
	}
}

func (app App) loadDisks(seq uint64) tea.Cmd {
	src := app.src()
	node := app.browser.node.IP
	sem := app.resSem
	usage := app.usageFunc()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), diskFetchTimeout)
		defer cancel()
		return diskFetchMsg{node: node, seq: seq, res: diskmodel.Fetch(ctx, src, node, sem, usage)}
	}
}

func (app App) handleDiskFetch(msg diskFetchMsg) App {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app
	}
	model := diskmodel.Build(msg.res.In)
	app.browser = app.browser.withPane(
		func(p pane) bool { return p.kind == paneDisks && p.disk.seq == msg.seq },
		func(p *pane) {
			p.disk.loading, p.disk.ready = false, true
			p.disk.res, p.disk.model = msg.res, model
			p.disk = clampDiskSel(p.disk)
		})
	return app
}

func (app App) reloadDisks() (App, tea.Cmd) {
	p, ok := app.diskPane()
	if !ok {
		return app, nil
	}
	seq := p.disk.seq + 1
	app = app.setDisk(func(dv *diskView) { dv.seq, dv.loading = seq, true })
	app.statusMsg = dimStyle.Render("reloading the disks…")
	return app, app.loadDisks(seq)
}

// --- selection ---

func (dv diskView) shown() []diskmodel.Disk { return dv.model.Shown(dv.all) }

func clampDiskSel(dv diskView) diskView {
	ds := dv.shown()
	dv.disk = clamp(dv.disk, 0, max(0, len(ds)-1))
	if len(ds) > 0 {
		dv.seg = clamp(dv.seg, 0, max(0, len(ds[dv.disk].Segments)-1))
	} else {
		dv.seg = 0
	}
	return dv
}

// diskSel returns the selected disk and segment (seg is false for a disk with none).
func (dv diskView) sel() (diskmodel.Disk, diskmodel.Segment, bool, bool) {
	ds := dv.shown()
	if dv.disk >= len(ds) {
		return diskmodel.Disk{}, diskmodel.Segment{}, false, false
	}
	d := ds[dv.disk]
	if dv.seg >= len(d.Segments) {
		return d, diskmodel.Segment{}, false, true
	}
	return d, d.Segments[dv.seg], true, true
}

func (app App) diskMoveDisk(delta int) (App, tea.Cmd) {
	return app.setDisk(func(dv *diskView) {
		old := dv.disk
		dv.disk = clamp(dv.disk+delta, 0, max(0, len(dv.shown())-1))
		if dv.disk != old { // keep the segment's relative position
			dv.seg = 0
		}
		*dv = clampDiskSel(*dv)
	}), nil
}

func (app App) diskMoveSeg(delta int) (App, tea.Cmd) {
	return app.setDisk(func(dv *diskView) {
		dv.seg += delta
		*dv = clampDiskSel(*dv)
	}), nil
}

func (app App) diskEdge(top bool) (App, tea.Cmd) {
	if top {
		return app.diskMoveDisk(-bigMove)
	}
	return app.diskMoveDisk(bigMove)
}

func (app App) diskToggleAll() (App, tea.Cmd) {
	app = app.setDisk(func(dv *diskView) {
		dv.all = !dv.all
		*dv = clampDiskSel(*dv)
	})
	if p, ok := app.diskPane(); ok {
		if p.disk.all {
			app.statusMsg = dimStyle.Render("showing every device, including loop, cdrom and read-only ones")
		} else {
			app.statusMsg = dimStyle.Render("hiding loop, cdrom and read-only devices")
		}
	}
	return app, nil
}

func (app App) diskToggleUnits() (App, tea.Cmd) {
	app = app.setDisk(func(dv *diskView) { dv.si = !dv.si })
	if p, ok := app.diskPane(); ok {
		if p.disk.si {
			app.statusMsg = dimStyle.Render("sizes in GB (powers of 10)")
		} else {
			app.statusMsg = dimStyle.Render("sizes in GiB (powers of 2)")
		}
	}
	return app, nil
}

// --- what the selection stands for ---

// diskRef is the resource Enter opens: the volume status when Talos has one,
// else the probed volume, else (for unallocated space and unused disks) the disk.
func (dv diskView) ref() (*netmodel.Ref, string) {
	d, s, hasSeg, ok := dv.sel()
	if !ok {
		return nil, "no disk selected"
	}
	if hasSeg {
		switch {
		case s.Volume != nil:
			return &s.Volume.Ref, ""
		case s.Discovered != nil:
			return s.Discovered, ""
		}
	}
	r := d.Ref
	return &r, ""
}

func (app App) diskEnter() (App, tea.Cmd) {
	p, ok := app.diskPane()
	if !ok {
		return app, nil
	}
	ref, why := p.disk.ref()
	if ref == nil {
		app.statusMsg = dimStyle.Render(why)
		return app, nil
	}
	d, _ := app.netDef(*ref)
	return app.openYAML(d, netMeta(*ref))
}

func (app App) diskSubject() (relSubject, talos.ResourceMeta, bool) {
	p, ok := app.diskPane()
	if !ok {
		return relSubject{}, talos.ResourceMeta{}, false
	}
	ref, why := p.disk.ref()
	if ref == nil {
		app.statusMsg = dimStyle.Render(why)
		return relSubject{}, talos.ResourceMeta{}, false
	}
	d, known := app.netDef(*ref)
	if !known {
		return relSubject{}, talos.ResourceMeta{}, false
	}
	return subjectOfDef(d), netMeta(*ref), true
}

func (app App) diskDescribe() (App, tea.Cmd) {
	s, meta, ok := app.diskSubject()
	if !ok {
		app.statusMsg = dimStyle.Render("still loading the resource definitions: try again in a moment")
		return app, nil
	}
	app.browser = app.browser.push(pane{kind: paneDescribe, title: "Describe " + s.display, sub: descSubject{def: s.def, meta: meta, hasMeta: true}})
	app = app.syncBrowserState()
	return app.ensureDeps()
}

func (app App) diskRelated() (App, tea.Cmd) {
	s, _, ok := app.diskSubject()
	if !ok {
		app.statusMsg = dimStyle.Render("still loading the resource definitions: try again in a moment")
		return app, nil
	}
	return app.pushRelated(s, nil)
}

// diskActions is the key table of the disk view.
func diskActions() []keyAction {
	return []keyAction{
		{keys: []string{"up", "k"}, label: "↑↓", desc: "Previous disk", visible: true, fn: func(app App) (App, tea.Cmd) { return app.diskMoveDisk(-1) }},
		{keys: []string{"down", "j"}, desc: "Next disk", fn: func(app App) (App, tea.Cmd) { return app.diskMoveDisk(1) }},
		{keys: []string{"left", "h"}, label: "←→", desc: "Previous partition", visible: true, fn: func(app App) (App, tea.Cmd) { return app.diskMoveSeg(-1) }},
		{keys: []string{"right", "l"}, desc: "Next partition", fn: func(app App) (App, tea.Cmd) { return app.diskMoveSeg(1) }},
		{keys: []string{"g", "home"}, desc: "First disk", fn: func(app App) (App, tea.Cmd) { return app.diskEdge(true) }},
		{keys: []string{"G", "end"}, desc: "Last disk", fn: func(app App) (App, tea.Cmd) { return app.diskEdge(false) }},
		{keys: []string{"enter"}, label: "↵", desc: "YAML of the selected partition (volume status, else discovered volume)", visible: true, fn: (App).diskEnter},
		{keys: []string{"d"}, desc: "Describe (what is this, on Ubuntu)", visible: true, fn: (App).diskDescribe},
		{keys: []string{"p"}, desc: "Related resources (pipeline and family)", visible: true, fn: (App).diskRelated},
		{keys: []string{"a"}, desc: "Show all devices (loop, cdrom, read-only)", visible: true, fn: (App).diskToggleAll},
		{keys: []string{"u"}, desc: "Toggle GiB / GB", visible: true, fn: (App).diskToggleUnits},
		{keys: []string{"ctrl+a"}, label: "^a", desc: "All types (aliases palette)", fn: (App).openPalette},
		{keys: []string{":"}, desc: "Command mode (:nodes :disks :netview :q)", fn: (App).openCommandPrompt},
		{keys: []string{"esc", "q"}, label: "Esc/q", desc: "Back", visible: true, fn: (App).browserBack},
		{keys: []string{"ctrl+r"}, label: "^r", desc: "Reload", visible: true, fn: (App).browserReload},
		{keys: []string{"ctrl+c"}, desc: "Quit", fn: func(app App) (App, tea.Cmd) {
			app.cleanup()
			return app, tea.Quit
		}},
	}
}

// diskSummary is the line above the disks.
func diskSummary(dv diskView) string {
	n, sys, raw, free := diskmodel.Totals(dv.shown())
	parts := []string{
		fmt.Sprintf("%d disk%s", n, plural(n)),
		fmt.Sprintf("%d system", sys),
		fmtSize(raw, dv.si) + " raw",
		fmtSize(free, dv.si) + " unallocated",
	}
	if !dv.model.UsageKnown {
		parts = append(parts, "usage unavailable")
	}
	if hidden := len(dv.model.Disks) - n; hidden > 0 {
		parts = append(parts, fmt.Sprintf("%d hidden (a)", hidden))
	}
	return strings.Join(parts, " · ")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
