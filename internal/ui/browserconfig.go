package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/catalog"
	"github.com/florianspk/t9s/internal/talos"
)

// Config documents: the machine config's documents shown beside COSI
// resources in the types pane. Loaded once per node when a category opens.

type cfgState int

const (
	cfgNone cfgState = iota
	cfgLoading
	cfgLoaded
	cfgDenied // needs os:admin
	cfgError
)

// cfgCacheEntry is what App keeps per node IP between browser sessions.
type cfgCacheEntry struct {
	docs   []talos.ConfigDoc
	denied bool
}

// legacyKind describes the v1alpha1 document, which the catalogue does not list.
var legacyKind = catalog.ConfigKind{
	Kind:  talos.LegacyConfigKind,
	Group: "runtime",
	Since: "v1.0",
	Desc:  "The legacy v1alpha1 machine config document (.machine and .cluster). Always present; newer settings live in the separate documents.",
}

// --- row model ---

// typeEntry is one selectable row of the types pane: a config kind or a resource type.
type typeEntry struct {
	config bool
	ck     catalog.ConfigKind
	def    talos.ResourceDef
}

func (e typeEntry) name() string {
	if e.config {
		return e.ck.Kind
	}
	return e.def.DisplayType
}

// vrow is one visual line of the types pane. sel >= 0 marks a selectable row
// (its index in typeEntries); headers and notes have sel == -1.
type vrow struct {
	header string // section header text
	note   string // dim informational row
	sel    int
}

func (b browser) docsOfKind(kind string) []talos.ConfigDoc {
	var out []talos.ConfigDoc
	for _, d := range b.docs {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	return out
}

// docLabels names each document: its `name:`, or #1, #2… when unnamed.
// Duplicate names get a #n suffix so labels are unique within the list.
func docLabels(docs []talos.ConfigDoc) []string {
	out := make([]string, len(docs))
	seen := map[string]int{}
	for _, d := range docs {
		seen[d.Name]++
	}
	for i, d := range docs {
		switch {
		case d.Name == "":
			out[i] = fmt.Sprintf("#%d", i+1)
		case seen[d.Name] > 1:
			out[i] = fmt.Sprintf("%s #%d", d.Name, i+1)
		default:
			out[i] = d.Name
		}
	}
	return out
}

// configShown reports whether config kinds are listed at all (not when the
// machine config is unreadable).
func (b browser) configShown() bool { return b.cfgState != cfgDenied && b.cfgState != cfgError }

// configKindsIn lists the config kinds of one category: catalogue kinds the
// node's Talos version knows about, any kind present in the node's config
// regardless of version, and unknown present kinds (under runtime for
// v1alpha1, otherwise other).
func (b browser) configKindsIn(cat string) []catalog.ConfigKind {
	if !b.configShown() {
		return nil
	}
	present := map[string]bool{}
	for _, d := range b.docs {
		present[d.Kind] = true
	}
	avail := map[string]bool{}
	for _, k := range catalog.KindsAvailable(b.node.Version) {
		avail[k.Kind] = true
	}
	var out []catalog.ConfigKind
	known := map[string]bool{}
	for _, k := range catalog.ConfigKinds() {
		known[k.Kind] = true
		if catalog.ConfigCategoryFor(k.Group) == cat && (avail[k.Kind] || present[k.Kind]) {
			out = append(out, k)
		}
	}
	extras := make([]string, 0)
	for k := range present {
		if !known[k] {
			extras = append(extras, k)
		}
	}
	sort.Strings(extras)
	for _, k := range extras {
		ck := catalog.ConfigKind{Kind: k, Group: "other", Desc: "A document kind this t9s build does not know."}
		if k == talos.LegacyConfigKind {
			ck = legacyKind
		}
		if catalog.ConfigCategoryFor(ck.Group) == cat {
			out = append(out, ck)
		}
	}
	return out
}

// configEntries returns the config rows of a category: present kinds first,
// then absent, each by name; a filter ranks them instead.
func (b browser) configEntries(cat, filter string) []typeEntry {
	kinds := b.configKindsIn(cat)
	sort.SliceStable(kinds, func(i, j int) bool {
		pi, pj := len(b.docsOfKind(kinds[i].Kind)) > 0, len(b.docsOfKind(kinds[j].Kind)) > 0
		if pi != pj {
			return pi
		}
		return strings.ToLower(kinds[i].Kind) < strings.ToLower(kinds[j].Kind)
	})
	kinds = rankFilter(kinds, filter, func(k catalog.ConfigKind) []string { return []string{k.Kind} })
	out := make([]typeEntry, len(kinds))
	for i, k := range kinds {
		out[i] = typeEntry{config: true, ck: k}
	}
	return out
}

// typeEntries is every selectable row of the types pane in display order.
func (b browser) typeEntries(cat, filter string) []typeEntry {
	out := b.configEntries(cat, filter)
	for _, d := range b.typeRows(cat, filter) {
		out = append(out, typeEntry{def: d})
	}
	return out
}

// typeVisual lays the entries out with section headers. The CONFIG header
// stays when the machine config is loading or unreadable (with a note), but
// only when no filter is set; otherwise a header shows only if its section has
// matches.
func (b browser) typeVisual(cat, filter string) []vrow {
	cfg := b.configEntries(cat, filter)
	res := b.typeRows(cat, filter)
	var out []vrow
	sel := 0
	switch {
	case len(cfg) > 0:
		out = append(out, vrow{header: "CONFIG", sel: -1})
		for range cfg {
			out = append(out, vrow{sel: sel})
			sel++
		}
	case filter == "" && b.cfgState == cfgDenied:
		out = append(out, vrow{header: "CONFIG", sel: -1}, vrow{note: "requires os:admin", sel: -1})
	case filter == "" && b.cfgState == cfgError:
		out = append(out, vrow{header: "CONFIG", sel: -1}, vrow{note: "could not read machine config", sel: -1})
	case filter == "" && b.cfgState == cfgLoading:
		out = append(out, vrow{header: "CONFIG", sel: -1}, vrow{note: "loading…", sel: -1})
	}
	if len(res) > 0 {
		out = append(out, vrow{header: "RESOURCES", sel: -1})
		for range res {
			out = append(out, vrow{sel: sel})
			sel++
		}
	}
	return out
}

// visualIndex maps a selectable index to its line in typeVisual.
func visualIndex(vis []vrow, sel int) int {
	for i, v := range vis {
		if v.sel == sel {
			return i
		}
	}
	return 0
}

// configCell is the count column for a config kind.
func (b browser) configCell(kind string) (text string, dim bool) {
	if b.cfgState == cfgLoading || b.cfgState == cfgNone {
		return "…", false
	}
	if n := len(b.docsOfKind(kind)); n > 0 {
		return fmt.Sprint(n), false
	}
	return "-", true
}

// --- loading ---

func (app App) loadConfigDocs() tea.Cmd {
	client := app.client
	node := app.browser.node.IP
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), browserTimeout)
		defer cancel()
		raw, err := client.GetMachineConfig(ctx, node)
		if err != nil {
			return configDocsMsg{node: node, denied: talos.IsPermissionDenied(err), err: err}
		}
		docs, err := talos.SplitConfigDocs(raw)
		return configDocsMsg{node: node, docs: docs, err: err}
	}
}

// useCachedConfig fills the browser's config from the per-node cache.
func (app App) useCachedConfig() (App, bool) {
	e, ok := app.configDocs[app.browser.node.IP]
	if !ok {
		return app, false
	}
	app.browser.docs = e.docs
	app.browser.cfgState = cfgLoaded
	if e.denied {
		app.browser.cfgState = cfgDenied
	}
	return app, true
}

// ensureConfig loads the node's config documents once (cache, then fetch).
func (app App) ensureConfig() (App, tea.Cmd) {
	switch app.browser.cfgState {
	case cfgLoading, cfgLoaded, cfgDenied:
		return app, nil
	}
	if a, ok := app.useCachedConfig(); ok {
		return a, nil
	}
	app.browser.cfgState = cfgLoading
	return app, app.loadConfigDocs()
}

func (app App) handleConfigDocs(msg configDocsMsg) App {
	if len(app.browser.stack) == 0 || app.browser.node.IP != msg.node {
		return app
	}
	switch {
	case msg.denied:
		app.browser.cfgState, app.browser.docs = cfgDenied, nil
	case msg.err != nil:
		app.browser.cfgState, app.browser.docs, app.browser.cfgErr = cfgError, nil, msg.err.Error()
		return app
	default:
		app.browser.cfgState, app.browser.docs, app.browser.cfgErr = cfgLoaded, msg.docs, ""
	}
	cache := make(map[string]cfgCacheEntry, len(app.configDocs)+1)
	for k, v := range app.configDocs {
		cache[k] = v
	}
	cache[msg.node] = cfgCacheEntry{docs: app.browser.docs, denied: msg.denied}
	app.configDocs = cache
	return app.refreshConfigPanes().netRebuild()
}

// refreshConfigPanes re-reads open config instance/YAML panes after a reload.
func (app App) refreshConfigPanes() App {
	st := make([]pane, len(app.browser.stack))
	copy(st, app.browser.stack)
	for i := range st {
		p := &st[i]
		if p.cfgKind == "" {
			continue
		}
		docs := app.browser.docsOfKind(p.cfgKind)
		labels := docLabels(docs)
		switch p.kind {
		case paneInstances:
			p.items = cfgItems(p.cfgKind, labels)
			p.cur = clamp(p.cur, 0, max(0, len(filterInstances(p.items, p.filter))-1))
		case paneYAML:
			for j, l := range labels {
				if l == p.meta.ID {
					p.yaml = docs[j].YAML
				}
			}
		}
	}
	app.browser.stack = st
	return app
}

func cfgItems(kind string, labels []string) []talos.ResourceMeta {
	out := make([]talos.ResourceMeta, len(labels))
	for i, l := range labels {
		out[i] = talos.ResourceMeta{Type: kind, ID: l}
	}
	return out
}

// --- opening ---

// openConfigKind implements Enter on a config row.
func (app App) openConfigKind(ck catalog.ConfigKind) (App, tea.Cmd) {
	b := app.browser
	if b.cfgState == cfgLoading || b.cfgState == cfgNone {
		app.statusMsg = dimStyle.Render("still loading machine config…")
		return app, nil
	}
	docs := b.docsOfKind(ck.Kind)
	if len(docs) == 0 {
		app.statusMsg = dimStyle.Render(fmt.Sprintf("no %s in this node's config", ck.Kind))
		return app, nil
	}
	labels := docLabels(docs)
	app.browser = b.push(pane{kind: paneInstances, title: ck.Kind, cfgKind: ck.Kind, items: cfgItems(ck.Kind, labels)})
	if len(docs) == 1 {
		return app.openConfigYAML(ck.Kind, labels[0])
	}
	return app.syncBrowserState(), nil
}

func (app App) openConfigYAML(kind, label string) (App, tea.Cmd) {
	docs := app.browser.docsOfKind(kind)
	for i, l := range docLabels(docs) {
		if l == label {
			app.browser = app.browser.clearFind().push(pane{
				kind: paneYAML, title: label, cfgKind: kind,
				meta: talos.ResourceMeta{Type: kind, ID: label}, yaml: docs[i].YAML,
			})
			return app.syncBrowserState(), nil
		}
	}
	return app, nil
}
