package talos

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"

	inspectapi "github.com/siderolabs/talos/pkg/machinery/api/inspect"
)

// ErrNeedsGRPC is returned when a source cannot provide the controller
// dependency graph. The text is shown to the user as is.
var ErrNeedsGRPC = errors.New("relationships need the gRPC source (--source=grpc)")

// DepEdge is one edge of the controller runtime graph: a controller reads
// (Output false) or writes (Output true) resources of one type.
type DepEdge struct {
	Controller string
	Namespace  string // empty from the CLI source: its graph output has no namespaces
	Type       string // full type, e.g. "LinkStatuses.net.talos.dev"
	ID         string // empty unless the controller watches one resource ID
	Output     bool
	Weak       bool // weak input: the controller is not blocked by the resource's finalizers
}

// DepGraph is the controller-resource dependency graph of one node.
type DepGraph struct{ Edges []DepEdge }

// Producers lists the controllers with an output edge on typ.
func (g DepGraph) Producers(typ string) []string {
	return g.controllers(typ, true)
}

// Consumers lists the controllers with an input edge on typ.
func (g DepGraph) Consumers(typ string) []string {
	return g.controllers(typ, false)
}

func (g DepGraph) controllers(typ string, output bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range g.Edges {
		if e.Type == typ && e.Output == output && !seen[e.Controller] {
			seen[e.Controller] = true
			out = append(out, e.Controller)
		}
	}
	sort.Strings(out)
	return out
}

// Inputs lists the input edges of a controller, one per resource type.
func (g DepGraph) Inputs(controller string) []DepEdge { return g.edges(controller, false) }

// Outputs lists the output edges of a controller, one per resource type.
func (g DepGraph) Outputs(controller string) []DepEdge { return g.edges(controller, true) }

func (g DepGraph) edges(controller string, output bool) []DepEdge {
	seen := map[string]bool{}
	var out []DepEdge
	for _, e := range g.Edges {
		if e.Controller == controller && e.Output == output && !seen[e.Type] {
			seen[e.Type] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// Empty reports a graph without edges.
func (g DepGraph) Empty() bool { return len(g.Edges) == 0 }

// DepGraphFromProto maps the inspect API response. DestroyReady inputs are
// dropped: the CLI graph hides them too, they only add clutter.
func DepGraphFromProto(resp *inspectapi.ControllerRuntimeDependenciesResponse) DepGraph {
	var g DepGraph
	for _, msg := range resp.GetMessages() {
		for _, e := range msg.GetEdges() {
			de := DepEdge{
				Controller: e.GetControllerName(),
				Namespace:  e.GetResourceNamespace(),
				Type:       e.GetResourceType(),
				ID:         e.GetResourceId(),
			}
			switch e.GetEdgeType() {
			case inspectapi.DependencyEdgeType_OUTPUT_EXCLUSIVE, inspectapi.DependencyEdgeType_OUTPUT_SHARED:
				de.Output = true
			case inspectapi.DependencyEdgeType_INPUT_STRONG:
			case inspectapi.DependencyEdgeType_INPUT_WEAK:
				de.Weak = true
			default:
				continue
			}
			g.Edges = append(g.Edges, de)
		}
	}
	return g
}

// --- CLI source: graphviz DOT ---
//
// The CLI prints the graph with github.com/emicklei/dot: one `nK[attrs];` line
// per node (label = controller or resource type, shape "box" for controllers
// and "note" for resource types) and one `nA->nB[attrs];` line per edge. An
// edge from a controller to a type is an output; from a type to a controller
// an input (style "dotted" = weak, label = resource ID). Node ids (n1, n2…)
// are not stable, so everything is keyed by label.

var (
	dotNodeRe = regexp.MustCompile(`^\s*(n\d+)\[(.*)\];\s*$`)
	dotEdgeRe = regexp.MustCompile(`^\s*(n\d+)->(n\d+)(?:\[(.*)\])?;\s*$`)
	dotAttrRe = regexp.MustCompile(`(\w+)="((?:[^"\\]|\\.)*)"`)
)

func dotAttrs(s string) map[string]string {
	m := map[string]string{}
	for _, a := range dotAttrRe.FindAllStringSubmatch(s, -1) {
		m[a[1]] = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(a[2])
	}
	return m
}

// ParseDepDOT parses the CLI's `inspect dependencies` output. It returns
// ErrNeedsGRPC when the text holds no edges (an unexpected format).
func ParseDepDOT(data []byte) (DepGraph, error) {
	type node struct {
		label      string
		controller bool
	}
	nodes := map[string]node{}
	type rawEdge struct {
		from, to string
		attrs    map[string]string
	}
	var edges []rawEdge
	for _, line := range strings.Split(string(data), "\n") {
		if m := dotEdgeRe.FindStringSubmatch(line); m != nil {
			edges = append(edges, rawEdge{m[1], m[2], dotAttrs(m[3])})
			continue
		}
		if m := dotNodeRe.FindStringSubmatch(line); m != nil {
			a := dotAttrs(m[2])
			nodes[m[1]] = node{label: a["label"], controller: a["shape"] == "box"}
		}
	}
	var g DepGraph
	for _, e := range edges {
		from, to := nodes[e.from], nodes[e.to]
		switch {
		case from.controller && !to.controller && to.label != "":
			g.Edges = append(g.Edges, DepEdge{Controller: from.label, Type: to.label, Output: true})
		case !from.controller && to.controller && from.label != "":
			g.Edges = append(g.Edges, DepEdge{
				Controller: to.label, Type: from.label, ID: e.attrs["label"],
				Weak: e.attrs["style"] == "dotted",
			})
		}
	}
	if g.Empty() {
		return DepGraph{}, ErrNeedsGRPC
	}
	return g, nil
}

// Dependencies returns the controller dependency graph of a node by parsing
// the CLI's graphviz output.
func (c *Client) Dependencies(ctx context.Context, node string) (DepGraph, error) {
	data, err := c.run(ctx, "inspect", "dependencies", "-n", node)
	if err != nil {
		return DepGraph{}, err
	}
	return ParseDepDOT(data)
}
