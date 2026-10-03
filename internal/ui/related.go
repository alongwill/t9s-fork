package ui

import (
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/florianspk/t9s/internal/catalog"
	"github.com/florianspk/t9s/internal/talos"
)

// Related-resources model (`p`): how a type's pipeline (Config → Spec → Status)
// looks on the controller graph, which types share its stem, and how their
// instances line up by ID. Everything here is pure data; relatedview.go draws it.

// relRole is the layer of the pipeline a type belongs to. It drives colour.
type relRole int

const (
	roleOther relRole = iota
	roleConfig
	roleSpec
	roleStatus
)

func (r relRole) String() string {
	switch r {
	case roleConfig:
		return "Config"
	case roleSpec:
		return "Spec"
	case roleStatus:
		return "Status"
	}
	return "other"
}

// stemSuffixes are stripped (once, longest first) from a display type to find
// its stem. A suffix is only stripped when something is left.
var stemSuffixes = []string{"Statuses", "Specs", "Status", "Spec", "Config", "Request", "Info"}

// stemOf: LinkStatus → Link, LinkConfig → Link, AddressSpec → Address,
// LinkAliasConfig → LinkAlias (so it is not part of the Link family).
func stemOf(display string) string {
	for _, s := range stemSuffixes {
		if strings.HasSuffix(display, s) && len(display) > len(s) {
			return strings.TrimSuffix(display, s)
		}
	}
	return display
}

// roleOf reads the role from the display type's suffix.
func roleOf(display string) relRole {
	switch {
	case display == "MachineConfig":
		return roleConfig
	case strings.HasSuffix(display, "Config"):
		return roleConfig
	case strings.HasSuffix(display, "Spec"), strings.HasSuffix(display, "Specs"), strings.HasSuffix(display, "Request"):
		return roleSpec
	case strings.HasSuffix(display, "Status"), strings.HasSuffix(display, "Statuses"):
		return roleStatus
	}
	return roleOther
}

// displayFromType turns "LinkStatuses.net.talos.dev" into "LinkStatus" for
// types the node has no definition for.
func displayFromType(typ string) string {
	name, _, _ := strings.Cut(typ, ".")
	switch {
	case strings.HasSuffix(name, "ies"):
		return strings.TrimSuffix(name, "ies") + "y"
	case strings.HasSuffix(name, "ses"):
		return strings.TrimSuffix(name, "es")
	case strings.HasSuffix(name, "s"):
		return strings.TrimSuffix(name, "s")
	}
	return name
}

// --- subject and family ---

// relSubject is what `p` was pressed on: a resource type or a config kind.
type relSubject struct {
	config  bool
	kind    string // config kind
	def     talos.ResourceDef
	display string
}

func (s relSubject) stem() string { return stemOf(s.display) }

// typ is the type the pipeline walk starts from. A config kind starts at the
// machine config, which is what controllers read.
func (s relSubject) typ() string {
	if s.config {
		return machineConfigType
	}
	return s.def.Type
}

func subjectOfDef(d talos.ResourceDef) relSubject {
	return relSubject{def: d, display: d.DisplayType}
}

func subjectOfKind(kind string) relSubject {
	return relSubject{config: true, kind: kind, display: kind}
}

// relMember is one member of a family: a config kind or a resource type.
type relMember struct {
	name   string
	role   relRole
	config bool
	kind   string
	def    talos.ResourceDef
}

// relFamily lists the types of the node and the config kinds that share the
// stem, ordered Config, Spec, Status, other (then by name).
func (b browser) relFamily(stem string) []relMember {
	var out []relMember
	seen := map[string]bool{}
	for _, k := range catalog.ConfigKinds() {
		if stemOf(k.Kind) == stem && !seen[k.Kind] {
			seen[k.Kind] = true
			out = append(out, relMember{name: k.Kind, role: roleConfig, config: true, kind: k.Kind})
		}
	}
	for _, d := range b.docs { // kinds this build does not list
		if stemOf(d.Kind) == stem && !seen[d.Kind] {
			seen[d.Kind] = true
			out = append(out, relMember{name: d.Kind, role: roleConfig, config: true, kind: d.Kind})
		}
	}
	for _, d := range b.defs {
		if stemOf(d.DisplayType) == stem {
			out = append(out, relMember{name: d.DisplayType, role: roleOf(d.DisplayType), def: d})
		}
	}
	rank := func(r relRole) int {
		switch r {
		case roleConfig:
			return 0
		case roleSpec:
			return 1
		case roleStatus:
			return 2
		}
		return 3
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := rank(out[i].role), rank(out[j].role); ri != rj {
			return ri < rj
		}
		return out[i].name < out[j].name
	})
	return out
}

// --- ID join ---

// Talos keeps the unmerged network resources in `network-config` with IDs
// "<layer>/<id>" (network.LayeredID) and the merged result in `network` under
// the plain id.
const (
	relMergedNS = "network"
	relLayerNS  = "network-config"
)

// relLayers is the display order of layer columns: highest priority first
// (network.ConfigLayer, reversed).
var relLayers = []string{"configuration", "operator", "platform", "cmdline", "default"}

func layerRank(l string) int {
	for i, x := range relLayers {
		if x == l {
			return i
		}
	}
	return len(relLayers)
}

// splitLayered splits "platform/eth0" into ("platform", "eth0"). ok is false
// when the prefix is not a known layer.
func splitLayered(id string) (layer, key string, ok bool) {
	l, rest, found := strings.Cut(id, "/")
	if !found || layerRank(l) == len(relLayers) {
		return "", id, false
	}
	return l, rest, true
}

// relList is the loaded instances of one (namespace, type).
type relList struct {
	items  []talos.ResourceMeta
	locked bool
	failed bool
}

func relKey(ns, typ string) string { return ns + "|" + typ }

// relReq is one List the related view needs.
type relReq struct{ ns, typ string }

// relRequests lists what to load for a family: every resource member in its
// default namespace, plus the layered namespace for merged network Specs.
func relRequests(members []relMember) []relReq {
	var out []relReq
	for _, m := range members {
		if m.config {
			continue
		}
		out = append(out, relReq{m.def.DefaultNamespace, m.def.Type})
		if m.role == roleSpec && m.def.DefaultNamespace == relMergedNS {
			out = append(out, relReq{relLayerNS, m.def.Type})
		}
	}
	return out
}

// --- table ---

type relCellState int

const (
	cellAbsent relCellState = iota
	cellPresent
	cellLocked
	cellLoading
	cellError
)

type relCol struct {
	title  string
	role   relRole
	config bool
	kind   string
	def    talos.ResourceDef
	ns     string
	layer  string
}

type relCell struct {
	state relCellState
	meta  talos.ResourceMeta // resources: the instance (Namespace, Type, ID)
	doc   int                // config documents: index into the document list
}

type relTable struct {
	cols  []relCol
	rows  []string
	cells [][]relCell // [row][col]
}

// buildRelTable joins the family on ID. Config documents join on their name;
// merged and layered resources on the ID without its layer prefix.
func buildRelTable(members []relMember, lists map[string]relList, docs []talos.ConfigDoc, cs cfgState) relTable {
	var t relTable
	type colData struct {
		byKey map[string]relCell
		state relCellState // for cells nobody filled: absent, loading, locked or error
	}
	var data []colData
	rowSet := map[string]bool{}

	add := func(c relCol, d colData) {
		t.cols = append(t.cols, c)
		data = append(data, d)
		for k := range d.byKey {
			rowSet[k] = true
		}
	}

	for _, m := range members {
		if m.config {
			d := colData{byKey: map[string]relCell{}}
			switch cs {
			case cfgLoaded:
				for i, doc := range docs {
					if doc.Kind == m.kind {
						d.byKey[doc.Name] = relCell{state: cellPresent, doc: i}
					}
				}
			case cfgDenied:
				d.state = cellLocked
			case cfgError:
				d.state = cellError
			default:
				d.state = cellLoading
			}
			add(relCol{title: m.name, role: roleConfig, config: true, kind: m.kind}, d)
			continue
		}

		fill := func(l relList, ok bool, layered bool) (map[string]relCell, relCellState, map[string]map[string]relCell) {
			cells := map[string]relCell{}
			layers := map[string]map[string]relCell{}
			switch {
			case !ok:
				return cells, cellLoading, layers
			case l.locked:
				return cells, cellLocked, layers
			case l.failed:
				return cells, cellError, layers
			}
			for _, it := range l.items {
				cell := relCell{state: cellPresent, meta: it}
				if layered {
					layer, key, found := splitLayered(it.ID)
					if !found {
						layer = "?"
					}
					if layers[layer] == nil {
						layers[layer] = map[string]relCell{}
					}
					layers[layer][key] = cell
					continue
				}
				cells[it.ID] = cell
			}
			return cells, cellAbsent, layers
		}

		if m.role == roleSpec && m.def.DefaultNamespace == relMergedNS {
			l, ok := lists[relKey(relLayerNS, m.def.Type)]
			_, _, layers := fill(l, ok, true)
			names := make([]string, 0, len(layers))
			for layer := range layers {
				names = append(names, layer)
			}
			sort.Slice(names, func(i, j int) bool {
				if ri, rj := layerRank(names[i]), layerRank(names[j]); ri != rj {
					return ri < rj
				}
				return names[i] < names[j]
			})
			for _, layer := range names {
				add(relCol{title: m.name + "@" + layer, role: roleSpec, def: m.def, ns: relLayerNS, layer: layer},
					colData{byKey: layers[layer]})
			}
		}
		l, ok := lists[relKey(m.def.DefaultNamespace, m.def.Type)]
		cells, st, _ := fill(l, ok, false)
		add(relCol{title: m.name, role: m.role, def: m.def, ns: m.def.DefaultNamespace}, colData{byKey: cells, state: st})
	}

	t.rows = make([]string, 0, len(rowSet))
	for k := range rowSet {
		t.rows = append(t.rows, k)
	}
	sort.Strings(t.rows)
	t.cells = make([][]relCell, len(t.rows))
	for r, key := range t.rows {
		t.cells[r] = make([]relCell, len(t.cols))
		for c := range t.cols {
			if cell, ok := data[c].byKey[key]; ok {
				t.cells[r][c] = cell
			} else {
				t.cells[r][c] = relCell{state: data[c].state}
			}
		}
	}
	return t
}

// rowLabel is the display text of a joined ID (config documents without a
// name join under "").
func rowLabel(key string) string {
	if key == "" {
		return "(unnamed)"
	}
	return key
}

// --- diff of two cells ---

// normalizeRelatedYAML prepares a cell's YAML for diffing: it drops the `node`
// line and every metadata field except id, namespace and type, so the diff
// shows only what the two resources say in their spec.
func normalizeRelatedYAML(y string) string {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(y), &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return y
	}
	root := doc.Content[0]
	dropKey(root, "node")
	if md := mapChild(root, "metadata"); md != nil && md.Kind == yaml.MappingNode {
		var keep []*yaml.Node
		for i := 0; i+1 < len(md.Content); i += 2 {
			switch md.Content[i].Value {
			case "id", "namespace", "type":
				keep = append(keep, md.Content[i], md.Content[i+1])
			}
		}
		md.Content = keep
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return y
	}
	return string(out)
}

// --- pipeline ---

const (
	pipelineHops  = 3
	maxStageBoxes = 5 // boxes per stage before "+N more"
)

// relBox is one type in the pipeline.
type relBox struct {
	typ      string
	display  string
	role     relRole
	known    bool     // the node has a definition for it
	more     int      // > 0: a "+N more" placeholder, not selectable
	cfgKinds []string // machine config box: the config kinds of the subject's family
}

func (b relBox) selectable() bool { return b.more == 0 }

type relStage struct {
	controllers []string // connector to the next stage
	boxes       []relBox
}

// relPipeline builds the stages for a subject from the cached dependency
// graph. The note explains an empty result (no graph yet, or no gRPC).
func (app App) relPipeline(s relSubject) ([]relStage, string) {
	g, note := app.depGraph()
	if note != "" {
		return nil, note
	}
	b := app.browser
	stem := s.stem()
	var keep func(string) bool
	if s.config { // the machine config feeds every controller: stay in the kind's group
		if ck, ok := findConfigKind(s.kind); ok {
			prefix := ck.Group
			if prefix == "kubernetes" {
				prefix = "k8s"
			}
			keep = func(c string) bool { return strings.HasPrefix(c, prefix+".") }
		}
	}
	stages := g.Pipeline(s.typ(), pipelineHops, pipelineHops, keep)

	var cfgKinds []string
	for _, m := range b.relFamily(stem) {
		if m.config {
			cfgKinds = append(cfgKinds, m.kind)
		}
	}
	box := func(typ string) relBox {
		d, known := b.lookupType(typ)
		display := d.DisplayType
		if !known {
			display = displayFromType(typ)
		}
		r := roleOf(display)
		if typ == machineConfigType {
			r = roleConfig
		}
		rb := relBox{typ: typ, display: display, role: r, known: known}
		if typ == machineConfigType {
			rb.cfgKinds = cfgKinds
		}
		return rb
	}

	out := make([]relStage, 0, len(stages))
	for _, st := range stages {
		boxes := make([]relBox, 0, len(st.Types))
		for _, t := range st.Types {
			boxes = append(boxes, box(t))
		}
		// keep the subject, the machine config and family members first,
		// then types present on the node, then the rest by name
		prio := func(x relBox) int {
			switch {
			case x.typ == s.typ(), x.typ == machineConfigType:
				return 0
			case stemOf(x.display) == stem:
				return 1
			case b.counts[x.typ] > 0:
				return 2
			}
			return 3
		}
		sort.SliceStable(boxes, func(i, j int) bool {
			if pi, pj := prio(boxes[i]), prio(boxes[j]); pi != pj {
				return pi < pj
			}
			return boxes[i].display < boxes[j].display
		})
		if len(boxes) > maxStageBoxes {
			rest := len(boxes) - (maxStageBoxes - 1)
			boxes = append(boxes[:maxStageBoxes-1:maxStageBoxes-1], relBox{more: rest, display: "+" + strconv.Itoa(rest) + " more"})
		}
		out = append(out, relStage{controllers: st.Controllers, boxes: boxes})
	}
	return out, ""
}
