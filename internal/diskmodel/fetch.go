package diskmodel

import (
	"context"
	"sync"

	"github.com/florianspk/t9s/internal/netmodel"
	"github.com/florianspk/t9s/internal/talos"
)

// FetchResult is what Fetch gathered. Types the node denied or lacks are
// listed instead of failing the whole read.
type FetchResult struct {
	In     Inputs
	Denied []string
	Failed map[string]string
}

// UsageFunc reads mount usage; it is optional (the CLI source and the gRPC
// source both lack a COSI resource for it).
type UsageFunc func(ctx context.Context, node string) (map[string]Usage, error)

// Fetch reads the block resources of one node through src (at most 8 calls at
// once through sem) and, when usage is not nil, the mount usage alongside.
func Fetch(ctx context.Context, src talos.ResourceSource, node string, sem chan struct{}, usage UsageFunc) FetchResult {
	var wg sync.WaitGroup
	var u map[string]Usage
	if usage != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := usage(ctx, node); err == nil {
				u = got
			}
		}()
	}
	got, denied, failed := netmodel.FetchSteps(ctx, src, node, sem, []netmodel.Step{
		{Type: TypeDisk, NS: NS}, {Type: TypeSystemDisk, NS: NS}, {Type: TypeDiscovered, NS: NS}, {Type: TypeVolume, NS: NS},
	})
	wg.Wait()
	return FetchResult{
		In:     Inputs{Node: node, Disks: got[0], SystemDisks: got[1], Discovered: got[2], Volumes: got[3], Usage: u},
		Denied: denied, Failed: failed,
	}
}
