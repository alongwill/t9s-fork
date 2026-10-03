package talos

import (
	"context"
	"errors"
	"reflect"
	"testing"

	inspectapi "github.com/siderolabs/talos/pkg/machinery/api/inspect"
)

func edge(name string, t inspectapi.DependencyEdgeType, ns, typ, id string) *inspectapi.ControllerDependencyEdge {
	return &inspectapi.ControllerDependencyEdge{
		ControllerName: name, EdgeType: t, ResourceNamespace: ns, ResourceType: typ, ResourceId: id,
	}
}

func sampleResponse() *inspectapi.ControllerRuntimeDependenciesResponse {
	const link, addr, spec = "LinkStatuses.net.talos.dev", "AddressStatuses.net.talos.dev", "LinkSpecs.net.talos.dev"
	return &inspectapi.ControllerRuntimeDependenciesResponse{Messages: []*inspectapi.ControllerRuntimeDependency{{
		Edges: []*inspectapi.ControllerDependencyEdge{
			edge("network.LinkStatusController", inspectapi.DependencyEdgeType_OUTPUT_EXCLUSIVE, "network", link, ""),
			edge("network.LinkStatusController", inspectapi.DependencyEdgeType_INPUT_WEAK, "network", spec, ""),
			edge("network.AddressStatusController", inspectapi.DependencyEdgeType_OUTPUT_SHARED, "network", addr, ""),
			edge("network.AddressStatusController", inspectapi.DependencyEdgeType_INPUT_STRONG, "network", link, "eth0"),
			edge("network.AddressStatusController", inspectapi.DependencyEdgeType_INPUT_DESTROY_READY, "network", link, ""),
			edge("network.RouteStatusController", inspectapi.DependencyEdgeType_INPUT_STRONG, "network", link, ""),
		},
	}}}
}

func TestDepGraphFromProto(t *testing.T) {
	g := DepGraphFromProto(sampleResponse())
	if len(g.Edges) != 5 {
		t.Fatalf("got %d edges, want 5 (DestroyReady dropped): %+v", len(g.Edges), g.Edges)
	}
	want := DepEdge{Controller: "network.LinkStatusController", Namespace: "network", Type: "LinkSpecs.net.talos.dev", Weak: true}
	if g.Edges[1] != want {
		t.Errorf("weak input edge = %+v, want %+v", g.Edges[1], want)
	}
	if e := g.Edges[3]; e.ID != "eth0" || e.Output || e.Weak {
		t.Errorf("strong input with id = %+v", e)
	}
	if DepGraphFromProto(nil).Edges != nil {
		t.Error("nil response should give an empty graph")
	}
}

func TestDepGraphQueries(t *testing.T) {
	g := DepGraphFromProto(sampleResponse())
	const link = "LinkStatuses.net.talos.dev"
	if got, want := g.Producers(link), []string{"network.LinkStatusController"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Producers = %v, want %v", got, want)
	}
	if got, want := g.Consumers(link), []string{"network.AddressStatusController", "network.RouteStatusController"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Consumers = %v, want %v", got, want)
	}
	in := g.Inputs("network.LinkStatusController")
	if len(in) != 1 || in[0].Type != "LinkSpecs.net.talos.dev" {
		t.Errorf("Inputs = %+v", in)
	}
	out := g.Outputs("network.AddressStatusController")
	if len(out) != 1 || out[0].Type != "AddressStatuses.net.talos.dev" {
		t.Errorf("Outputs = %+v", out)
	}
	if g.Producers("Nope.x.talos.dev") != nil || g.Inputs("nobody") != nil {
		t.Error("unknown names should give nothing")
	}
}

// dotFixture is hand-written in the layout github.com/emicklei/dot prints
// (formatters.RenderGraph without --with-resources).
const dotFixture = `digraph  {
	
	n1[label="network.LinkStatusController",shape="box"];
	n2[fillcolor="azure2",label="LinkStatuses.net.talos.dev",shape="note",style="filled"];
	n3[fillcolor="azure2",label="LinkSpecs.net.talos.dev",shape="note",style="filled"];
	n4[label="network.AddressStatusController",shape="box"];
	n5[fillcolor="azure2",label="AddressStatuses.net.talos.dev",shape="note",style="filled"];
	n1->n2[style="bold"];
	n3->n1[style="dotted"];
	n2->n4[label="eth0",style="solid"];
	n4->n5[style="solid"];
}
`

func TestParseDepDOT(t *testing.T) {
	g, err := ParseDepDOT([]byte(dotFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := []DepEdge{
		{Controller: "network.LinkStatusController", Type: "LinkStatuses.net.talos.dev", Output: true},
		{Controller: "network.LinkStatusController", Type: "LinkSpecs.net.talos.dev", Weak: true},
		{Controller: "network.AddressStatusController", Type: "LinkStatuses.net.talos.dev", ID: "eth0"},
		{Controller: "network.AddressStatusController", Type: "AddressStatuses.net.talos.dev", Output: true},
	}
	if !reflect.DeepEqual(g.Edges, want) {
		t.Errorf("edges =\n%+v\nwant\n%+v", g.Edges, want)
	}
	if got := g.Producers("AddressStatuses.net.talos.dev"); len(got) != 1 {
		t.Errorf("Producers = %v", got)
	}
}

func TestParseDepDOTUnknownFormat(t *testing.T) {
	for _, in := range []string{"", "digraph { }", "something else entirely\n"} {
		if _, err := ParseDepDOT([]byte(in)); !errors.Is(err, ErrNeedsGRPC) {
			t.Errorf("ParseDepDOT(%q) err = %v, want ErrNeedsGRPC", in, err)
		}
	}
}

func TestGRPCSourceDependencies(t *testing.T) {
	src := newGRPCSourceFromState(nil, nil, nil)
	if _, err := src.Dependencies(context.Background(), "10.0.0.1"); !errors.Is(err, ErrNeedsGRPC) {
		t.Errorf("without inspect: err = %v", err)
	}
	var gotNode string
	src.inspect = func(_ context.Context, node string) (*inspectapi.ControllerRuntimeDependenciesResponse, error) {
		gotNode = node
		return sampleResponse(), nil
	}
	g, err := src.Dependencies(context.Background(), "10.0.0.1")
	if err != nil || gotNode != "10.0.0.1" || len(g.Edges) != 5 {
		t.Errorf("Dependencies = %d edges, node %q, err %v", len(g.Edges), gotNode, err)
	}
	src.inspect = func(context.Context, string) (*inspectapi.ControllerRuntimeDependenciesResponse, error) {
		return nil, errors.New("boom")
	}
	if _, err := src.Dependencies(context.Background(), "x"); err == nil {
		t.Error("error from the API was swallowed")
	}
}

func TestParseResourceListOwner(t *testing.T) {
	data := []byte(`{"node":"10.0.0.1","metadata":{"namespace":"network","type":"LinkStatuses.net.talos.dev","id":"eth0","version":3,"owner":"network.LinkStatusController","phase":"running"},"spec":{}}`)
	got, err := parseResourceList(data)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if got[0].Owner != "network.LinkStatusController" {
		t.Errorf("Owner = %q", got[0].Owner)
	}
}
