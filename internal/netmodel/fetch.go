package netmodel

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/florianspk/t9s/internal/talos"
)

// itemTimeout bounds one List or GetYAML call.
const itemTimeout = 10 * time.Second

// FetchResult is what Fetch gathered. Types the node denied or does not have
// are listed instead of failing the whole fetch: the model draws what it got.
type FetchResult struct {
	In     Inputs
	Denied []string          // display types that need os:admin
	Failed map[string]string // display type -> error text
}

type fetchStep struct {
	typ, ns string
	assign  func(in *Inputs, r Res)
}

var fetchSteps = []fetchStep{
	{TypeLinkStatus, NSNetwork, func(in *Inputs, r Res) { in.LinkStatuses = append(in.LinkStatuses, r) }},
	{TypeAddrStatus, NSNetwork, func(in *Inputs, r Res) { in.AddressStatuses = append(in.AddressStatuses, r) }},
	{TypeRouteStatus, NSNetwork, func(in *Inputs, r Res) { in.RouteStatuses = append(in.RouteStatuses, r) }},
	{TypeLinkSpec, NSConfig, func(in *Inputs, r Res) { in.LinkSpecs = append(in.LinkSpecs, r) }},
	{TypeLinkSpec, NSNetwork, func(in *Inputs, r Res) { in.LinkSpecs = append(in.LinkSpecs, r) }},
	{TypeAddrSpec, NSConfig, func(in *Inputs, r Res) { in.AddressSpecs = append(in.AddressSpecs, r) }},
	{TypeAddrSpec, NSNetwork, func(in *Inputs, r Res) { in.AddressSpecs = append(in.AddressSpecs, r) }},
	{TypeRouteSpec, NSConfig, func(in *Inputs, r Res) { in.RouteSpecs = append(in.RouteSpecs, r) }},
	{TypeRouteSpec, NSNetwork, func(in *Inputs, r Res) { in.RouteSpecs = append(in.RouteSpecs, r) }},
	{TypeOperatorSpec, NSNetwork, func(in *Inputs, r Res) { in.OperatorSpecs = append(in.OperatorSpecs, r) }},
	{TypeHostname, NSNetwork, func(in *Inputs, r Res) { in.Hostnames = append(in.Hostnames, r) }},
	{TypeResolver, NSNetwork, func(in *Inputs, r Res) { in.Resolvers = append(in.Resolvers, r) }},
	{TypeNodeAddress, NSNetwork, func(in *Inputs, r Res) { in.NodeAddresses = append(in.NodeAddresses, r) }},
	{TypeKubeSpanPeer, NSKube, func(in *Inputs, r Res) { in.KubeSpanPeers = append(in.KubeSpanPeers, r) }},
	{TypeKubeSpanIdent, NSKube, func(in *Inputs, r Res) { in.KubeSpanIdentities = append(in.KubeSpanIdentities, r) }},
}

// Step is one (namespace, type) to read.
type Step struct{ Type, NS string }

// FetchSteps lists every step and reads each resource's YAML. Every source call
// takes a slot of sem (the browser shares one, capacity 8, so a node is never
// hit by more than 8 calls at once); a nil sem makes a private one. The result
// has one slice per step, in step order and sorted by ID. Types the node
// denied or does not have are reported instead of failing the whole read.
func FetchSteps(ctx context.Context, src talos.ResourceSource, node string, sem chan struct{}, steps []Step) (out [][]Res, denied []string, failed map[string]string) {
	if sem == nil {
		sem = make(chan struct{}, 8)
	}
	out = make([][]Res, len(steps))
	failed = map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup

	call := func(f func(ctx context.Context)) {
		sem <- struct{}{}
		defer func() { <-sem }()
		cctx, cancel := context.WithTimeout(ctx, itemTimeout)
		defer cancel()
		f(cctx)
	}
	fail := func(typ string, err error) {
		mu.Lock()
		defer mu.Unlock()
		if talos.IsPermissionDenied(err) {
			denied = append(denied, DisplayType(typ))
		} else if _, seen := failed[DisplayType(typ)]; !seen {
			failed[DisplayType(typ)] = err.Error()
		}
	}

	for i, st := range steps {
		i, st := i, st
		wg.Add(1)
		go func() {
			defer wg.Done()
			var items []talos.ResourceMeta
			var err error
			call(func(c context.Context) { items, err = src.List(c, node, st.NS, st.Type) })
			if err != nil {
				fail(st.Type, err)
				return
			}
			var iwg sync.WaitGroup
			for _, it := range items {
				it := it
				iwg.Add(1)
				go func() {
					defer iwg.Done()
					var y string
					var gerr error
					call(func(c context.Context) { y, gerr = src.GetYAML(c, node, st.NS, st.Type, it.ID) })
					if gerr != nil {
						fail(st.Type, gerr)
						return
					}
					r, perr := ParseRes(y, st.NS)
					if perr != nil {
						fail(st.Type, perr)
						return
					}
					if r.ID == "" {
						r.ID = it.ID
					}
					mu.Lock()
					out[i] = append(out[i], r)
					mu.Unlock()
				}()
			}
			iwg.Wait()
		}()
	}
	wg.Wait()

	sort.Strings(denied)
	denied = dedupe(denied)
	for _, rs := range out {
		sortRes(rs)
	}
	return out, denied, failed
}

// Fetch reads the network resources of one node. Docs are left for the
// caller: the browser already loads and caches the machine config.
func Fetch(ctx context.Context, src talos.ResourceSource, node string, sem chan struct{}) FetchResult {
	steps := make([]Step, len(fetchSteps))
	for i, st := range fetchSteps {
		steps[i] = Step{st.typ, st.ns}
	}
	got, denied, failed := FetchSteps(ctx, src, node, sem, steps)
	res := FetchResult{In: Inputs{Node: node}, Denied: denied, Failed: failed}
	for i, st := range fetchSteps {
		for _, r := range got[i] {
			st.assign(&res.In, r)
		}
	}
	sortInputs(&res.In)
	return res
}

func dedupe(s []string) []string {
	var out []string
	for i, x := range s {
		if i == 0 || x != s[i-1] {
			out = append(out, x)
		}
	}
	return out
}

func sortRes(rs []Res) {
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].Namespace != rs[j].Namespace {
			return rs[i].Namespace < rs[j].Namespace
		}
		return rs[i].ID < rs[j].ID
	})
}

// sortInputs makes the order independent of which call finished first.
func sortInputs(in *Inputs) {
	for _, rs := range [][]Res{
		in.LinkStatuses, in.AddressStatuses, in.RouteStatuses, in.LinkSpecs, in.AddressSpecs, in.RouteSpecs,
		in.OperatorSpecs, in.Hostnames, in.Resolvers, in.NodeAddresses, in.KubeSpanPeers, in.KubeSpanIdentities,
	} {
		sortRes(rs)
	}
}
