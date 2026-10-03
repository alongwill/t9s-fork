package ui

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/florianspk/t9s/internal/talos"
)

// fakeSource is an in-memory ResourceSource for browser tests.
type fakeSource struct {
	name  string
	defs  []talos.ResourceDef
	lists map[string][]talos.ResourceMeta // "node|type" → items
	yamls map[string]string               // "node|type|id" → yaml
	errs  map[string]error                // "node|type" → list error
	yerrs map[string]error                // "node|type|id" → GetYAML error

	listCalls  atomic.Int32
	yamlCalls  atomic.Int32
	watchCalls atomic.Int32

	mu         sync.Mutex
	watchCtx   context.Context
	watchOut   chan<- talos.WatchEvent
	watchErr   error // returned by Watch immediately when set
	watchReady chan struct{}
	closed     atomic.Bool
}

func newFakeSource(name string) *fakeSource {
	return &fakeSource{
		name:       name,
		lists:      map[string][]talos.ResourceMeta{},
		yamls:      map[string]string{},
		errs:       map[string]error{},
		yerrs:      map[string]error{},
		watchReady: make(chan struct{}, 8),
	}
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Definitions(context.Context, string) ([]talos.ResourceDef, error) {
	return f.defs, nil
}

func (f *fakeSource) List(_ context.Context, node, _, typ string) ([]talos.ResourceMeta, error) {
	f.listCalls.Add(1)
	if err := f.errs[node+"|"+typ]; err != nil {
		return nil, err
	}
	return f.lists[node+"|"+typ], nil
}

func (f *fakeSource) GetYAML(_ context.Context, node, _, typ, id string) (string, error) {
	f.yamlCalls.Add(1)
	if err := f.yerrs[node+"|"+typ+"|"+id]; err != nil {
		return "", err
	}
	if y, ok := f.yamls[node+"|"+typ+"|"+id]; ok {
		return y, nil
	}
	return "", fmt.Errorf("not found: %s|%s|%s", node, typ, id)
}

func (f *fakeSource) Watch(ctx context.Context, node, ns, typ string, out chan<- talos.WatchEvent) error {
	f.watchCalls.Add(1)
	if f.name != "grpc" {
		return talos.ErrWatchUnsupported
	}
	f.mu.Lock()
	f.watchCtx, f.watchOut = ctx, out
	err := f.watchErr
	f.mu.Unlock()
	if err != nil {
		return err
	}
	f.watchReady <- struct{}{}
	<-ctx.Done()
	return nil
}

func (f *fakeSource) Close() error { f.closed.Store(true); return nil }
