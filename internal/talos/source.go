package talos

import (
	"context"
	"errors"
)

// ErrWatchUnsupported is returned by sources that cannot stream changes.
var ErrWatchUnsupported = errors.New("watch needs the gRPC source")

// WatchEvent is one change to a watched resource type.
type WatchEvent struct {
	Kind string // created|updated|destroyed|bootstrapped|error
	Meta ResourceMeta
	Err  error
}

// ResourceSource is where the resource browser reads COSI resources from.
// Every other t9s view keeps using the subprocess Client.
type ResourceSource interface {
	Name() string // "grpc" or "cli", shown in the header
	Definitions(ctx context.Context, node string) ([]ResourceDef, error)
	List(ctx context.Context, node, ns, typ string) ([]ResourceMeta, error)
	GetYAML(ctx context.Context, node, ns, typ, id string) (string, error)
	// Watch streams changes of one type until ctx is cancelled (returns nil) or
	// the stream fails (returns the error; nothing is sent for it). It sends the
	// initial contents as created events followed by one bootstrapped event and
	// never closes out.
	Watch(ctx context.Context, node, ns, typ string, out chan<- WatchEvent) error
	// Dependencies returns the controller-resource graph of the node, or
	// ErrNeedsGRPC when the source cannot provide it.
	Dependencies(ctx context.Context, node string) (DepGraph, error)
	Close() error
}

// cliSource implements ResourceSource on top of the subprocess client.
type cliSource struct{ c *Client }

// NewCLISource wraps the subprocess client as a ResourceSource.
func NewCLISource(c *Client) ResourceSource { return cliSource{c: c} }

func (cliSource) Name() string { return "cli" }

func (s cliSource) Definitions(ctx context.Context, node string) ([]ResourceDef, error) {
	return s.c.GetResourceDefinitions(ctx, node)
}

func (s cliSource) List(ctx context.Context, node, ns, typ string) ([]ResourceMeta, error) {
	return s.c.ListResources(ctx, node, ns, typ)
}

func (s cliSource) GetYAML(ctx context.Context, node, ns, typ, id string) (string, error) {
	return s.c.GetResourceYAML(ctx, node, ns, typ, id)
}

func (s cliSource) Dependencies(ctx context.Context, node string) (DepGraph, error) {
	return s.c.Dependencies(ctx, node)
}

func (cliSource) Watch(context.Context, string, string, string, chan<- WatchEvent) error {
	return ErrWatchUnsupported
}

func (cliSource) Close() error { return nil }
