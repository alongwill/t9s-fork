package diskmodel

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/florianspk/t9s/internal/talos"
)

type stubSource struct {
	data map[string]map[string]string
}

func (s *stubSource) Name() string { return "stub" }
func (s *stubSource) Definitions(context.Context, string) ([]talos.ResourceDef, error) {
	return nil, nil
}
func (s *stubSource) List(_ context.Context, _, ns, typ string) ([]talos.ResourceMeta, error) {
	items, ok := s.data[ns+"|"+typ]
	if !ok {
		if typ == TypeSystemDisk {
			return nil, errors.New("rpc error: code = PermissionDenied")
		}
		return nil, fmt.Errorf("not registered")
	}
	var out []talos.ResourceMeta
	for id := range items {
		out = append(out, talos.ResourceMeta{Namespace: ns, Type: typ, ID: id})
	}
	return out, nil
}
func (s *stubSource) GetYAML(_ context.Context, _, ns, typ, id string) (string, error) {
	return s.data[ns+"|"+typ][id], nil
}
func (s *stubSource) Watch(context.Context, string, string, string, chan<- talos.WatchEvent) error {
	return talos.ErrWatchUnsupported
}
func (s *stubSource) Dependencies(context.Context, string) (talos.DepGraph, error) {
	return talos.DepGraph{}, talos.ErrNeedsGRPC
}
func (s *stubSource) Close() error { return nil }

func TestFetchJoinsAndKeepsPartialResults(t *testing.T) {
	src := &stubSource{data: map[string]map[string]string{
		NS + "|" + TypeDisk: {"sda": "metadata:\n  id: sda\nspec:\n  dev_path: /dev/sda\n  size: 1073741824\n  transport: sata\n"},
	}}
	usageCalled := false
	res := Fetch(context.Background(), src, "10.0.0.5", nil, func(context.Context, string) (map[string]Usage, error) {
		usageCalled = true
		return map[string]Usage{"/var": {Size: 1}}, nil
	})
	if len(res.In.Disks) != 1 || res.In.Disks[0].ID != "sda" || !usageCalled || res.In.Usage["/var"].Size != 1 {
		t.Fatalf("fetch = %+v", res.In)
	}
	if len(res.Denied) != 1 || res.Denied[0] != "SystemDisk" {
		t.Errorf("denied = %v", res.Denied)
	}
	if _, ok := res.Failed["DiscoveredVolume"]; !ok {
		t.Errorf("failed = %v", res.Failed)
	}
	m := Build(res.In)
	if len(m.Disks) != 1 || !m.Disks[0].Unused {
		t.Errorf("model = %+v", m.Disks)
	}
	// usage that cannot be read leaves it unknown, nothing else
	res = Fetch(context.Background(), src, "n", nil, func(context.Context, string) (map[string]Usage, error) { return nil, errors.New("boom") })
	if res.In.Usage != nil || Build(res.In).UsageKnown {
		t.Error("usage should be unknown when the read fails")
	}
}
