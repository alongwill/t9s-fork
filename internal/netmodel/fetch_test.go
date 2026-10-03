package netmodel

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianspk/t9s/internal/talos"
)

// stubSource serves resources from memory: "ns|type" -> id -> YAML.
type stubSource struct {
	data    map[string]map[string]string
	denied  map[string]bool
	cur     atomic.Int32
	maxSeen atomic.Int32
}

func (s *stubSource) enter() func() {
	n := s.cur.Add(1)
	for {
		m := s.maxSeen.Load()
		if n <= m || s.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(time.Millisecond)
	return func() { s.cur.Add(-1) }
}

func (s *stubSource) Name() string { return "stub" }
func (s *stubSource) Definitions(context.Context, string) ([]talos.ResourceDef, error) {
	return nil, nil
}
func (s *stubSource) List(_ context.Context, _, ns, typ string) ([]talos.ResourceMeta, error) {
	defer s.enter()()
	if s.denied[typ] {
		return nil, errors.New("rpc error: code = PermissionDenied")
	}
	items, ok := s.data[ns+"|"+typ]
	if !ok {
		return nil, fmt.Errorf("resource %s not registered", typ)
	}
	var out []talos.ResourceMeta
	for id := range items {
		out = append(out, talos.ResourceMeta{Namespace: ns, Type: typ, ID: id})
	}
	return out, nil
}
func (s *stubSource) GetYAML(_ context.Context, _, ns, typ, id string) (string, error) {
	defer s.enter()()
	return s.data[ns+"|"+typ][id], nil
}
func (s *stubSource) Watch(context.Context, string, string, string, chan<- talos.WatchEvent) error {
	return talos.ErrWatchUnsupported
}
func (s *stubSource) Dependencies(context.Context, string) (talos.DepGraph, error) {
	return talos.DepGraph{}, talos.ErrNeedsGRPC
}
func (s *stubSource) Close() error { return nil }

func yamlOf(ns, id, spec string) string {
	return fmt.Sprintf("metadata:\n  namespace: %s\n  id: %s\nspec:\n%s", ns, id, spec)
}

func TestFetchAssemblesAndLimitsConcurrency(t *testing.T) {
	src := &stubSource{
		data:   map[string]map[string]string{},
		denied: map[string]bool{TypeOperatorSpec: true},
	}
	links := map[string]string{}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("eth%d", i)
		links[id] = yamlOf(NSNetwork, id, fmt.Sprintf("  index: %d\n  type: ether\n  kind: \"\"\n  operationalState: up\n", i+2))
	}
	src.data[NSNetwork+"|"+TypeLinkStatus] = links
	src.data[NSNetwork+"|"+TypeHostname] = map[string]string{"hostname": yamlOf(NSNetwork, "hostname", "  hostname: cp-1\n")}

	sem := make(chan struct{}, 8)
	res := Fetch(context.Background(), src, "10.0.0.5", sem)
	if len(res.In.LinkStatuses) != 20 || res.In.LinkStatuses[0].ID != "eth0" || res.In.LinkStatuses[1].ID != "eth1" {
		t.Fatalf("link statuses = %d (sorted by namespace and id)", len(res.In.LinkStatuses))
	}
	if len(res.In.Hostnames) != 1 || str(res.In.Hostnames[0].Spec, "hostname") != "cp-1" {
		t.Fatalf("hostnames = %+v", res.In.Hostnames)
	}
	if len(res.Denied) != 1 || res.Denied[0] != "OperatorSpec" {
		t.Fatalf("denied = %v", res.Denied)
	}
	if _, ok := res.Failed["AddressStatus"]; !ok {
		t.Fatalf("a type the node does not have is reported as failed: %v", res.Failed)
	}
	if got := src.maxSeen.Load(); got > 8 {
		t.Fatalf("%d concurrent calls, the semaphore allows 8", got)
	}
	m := Build(res.In)
	if len(m.Links) != 20 || m.Hostname != "cp-1" {
		t.Fatalf("model: %d links, hostname %q", len(m.Links), m.Hostname)
	}
}
