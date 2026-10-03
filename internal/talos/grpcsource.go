package talos

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/meta"
	"github.com/cosi-project/runtime/pkg/resource/protobuf"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/siderolabs/talos/pkg/machinery/client"
	yaml "go.yaml.in/yaml/v4"
)

// machineConfigType gets the same YAML treatment as in the CLI's yaml printer.
const machineConfigType = "MachineConfigs.config.talos.dev"

// grpcSource reads COSI resources over Talos' gRPC API (pkg/machinery/client).
// It is used by the resource browser only; every other view uses Client.
type grpcSource struct {
	st       state.State
	withNode func(ctx context.Context, node string) context.Context
	closer   func() error
}

// NewGRPCSource connects with the talosconfig and context t9s already
// resolved (empty cfgPath = the default location). It probes the connection
// with one cheap read so an unusable auth mode fails here, not on first use.
func NewGRPCSource(ctx context.Context, cfgPath, contextName string) (ResourceSource, error) {
	var opts []client.OptionFunc
	if cfgPath != "" {
		opts = append(opts, client.WithConfigFromFile(cfgPath))
	} else {
		opts = append(opts, client.WithDefaultConfig())
	}
	if contextName != "" {
		opts = append(opts, client.WithContextName(contextName))
	}

	c, err := client.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	src := newGRPCSourceFromState(c.COSI, client.WithNode, c.Close)
	if _, err := c.COSI.List(ctx, resource.NewMetadata(meta.NamespaceName, meta.NamespaceType, "", resource.VersionUndefined)); err != nil {
		_ = c.Close()
		return nil, err
	}
	return src, nil
}

func newGRPCSourceFromState(st state.State, withNode func(context.Context, string) context.Context, closer func() error) *grpcSource {
	if withNode == nil {
		withNode = func(ctx context.Context, _ string) context.Context { return ctx }
	}
	if closer == nil {
		closer = func() error { return nil }
	}
	return &grpcSource{st: st, withNode: withNode, closer: closer}
}

func (*grpcSource) Name() string { return "grpc" }

func (s *grpcSource) Close() error { return s.closer() }

func (s *grpcSource) Definitions(ctx context.Context, node string) ([]ResourceDef, error) {
	list, err := s.st.List(s.withNode(ctx, node),
		resource.NewMetadata(meta.NamespaceName, meta.ResourceDefinitionType, "", resource.VersionUndefined))
	if err != nil {
		return nil, wrapGRPCErr(err)
	}
	defs := make([]ResourceDef, 0, len(list.Items))
	for _, it := range list.Items {
		rd, ok := it.(*meta.ResourceDefinition)
		if !ok {
			continue
		}
		if d, ok := defFromSpec(*rd.TypedSpec()); ok {
			defs = append(defs, d)
		}
	}
	return defs, nil
}

// defFromSpec maps the COSI spec to the same fields the CLI parser fills.
func defFromSpec(sp meta.ResourceDefinitionSpec) (ResourceDef, bool) {
	if sp.Type == "" {
		return ResourceDef{}, false
	}
	aliases := sp.Aliases
	if len(aliases) == 0 {
		aliases = sp.AllAliases
	}
	return ResourceDef{
		Type:             sp.Type,
		DisplayType:      sp.DisplayType,
		Aliases:          aliases,
		DefaultNamespace: sp.DefaultNamespace,
		Sensitive:        sp.Sensitivity != meta.NonSensitive,
	}, true
}

func (s *grpcSource) List(ctx context.Context, node, ns, typ string) ([]ResourceMeta, error) {
	list, err := s.st.List(s.withNode(ctx, node),
		resource.NewMetadata(ns, typ, "", resource.VersionUndefined),
		state.WithListUnmarshalOptions(state.WithSkipProtobufUnmarshal()))
	if err != nil {
		return nil, wrapGRPCErr(err)
	}
	out := make([]ResourceMeta, 0, len(list.Items))
	for _, it := range list.Items {
		out = append(out, metaFromResource(it))
	}
	return out, nil
}

func metaFromResource(r resource.Resource) ResourceMeta {
	md := r.Metadata()
	return ResourceMeta{
		Namespace: md.Namespace(),
		Type:      md.Type(),
		ID:        md.ID(),
		Version:   md.Version().String(),
		Phase:     md.Phase().String(),
	}
}

func (s *grpcSource) GetYAML(ctx context.Context, node, ns, typ, id string) (string, error) {
	r, err := s.st.Get(s.withNode(ctx, node),
		resource.NewMetadata(ns, typ, id, resource.VersionUndefined),
		state.WithGetUnmarshalOptions(state.WithSkipProtobufUnmarshal()))
	if err != nil {
		return "", wrapGRPCErr(err)
	}
	return marshalResourceYAML(node, r)
}

// mcYamlRepr and mcYamlSpec mirror the CLI printer: the machine config spec is
// printed as a string holding every document, not cut after the first one.
type mcYamlRepr struct{ resource.Resource }

func (m *mcYamlRepr) Spec() any { return &mcYamlSpec{res: m.Resource} }

type mcYamlSpec struct{ res resource.Resource }

func (m *mcYamlSpec) MarshalYAML() (any, error) {
	if pb, ok := m.res.(*protobuf.Resource); ok {
		p, err := pb.Marshal()
		if err != nil {
			return nil, fmt.Errorf("marshal protobuf resource: %w", err)
		}
		return p.GetSpec().GetYamlSpec(), nil
	}
	out, err := yaml.Marshal(m.res.Spec())
	if err != nil {
		return nil, err
	}
	return string(out), nil
}

// marshalResourceYAML renders a resource exactly like the CLI's `-o yaml`:
// a `node:` line, then metadata and spec.
func marshalResourceYAML(node string, r resource.Resource) (string, error) {
	if r.Metadata().Type() == machineConfigType && r.Metadata().Annotations().Empty() {
		r = &mcYamlRepr{r}
	}
	doc, err := resource.MarshalYAML(r)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "node: %s\n", node)
	if err := yaml.NewEncoder(&buf).Encode(doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *grpcSource) Watch(ctx context.Context, node, ns, typ string, out chan<- WatchEvent) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan state.Event)
	err := s.st.WatchKind(s.withNode(ctx, node),
		resource.NewMetadata(ns, typ, "", resource.VersionUndefined), ch,
		state.WithBootstrapContents(true),
		state.WithWatchKindUnmarshalOptions(state.WithSkipProtobufUnmarshal()))
	if err != nil {
		return wrapGRPCErr(err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-ch:
			we, ok := watchEventFrom(ev)
			if !ok {
				continue
			}
			if we.Kind == "error" { // reported once, through the return value
				return we.Err
			}
			select {
			case out <- we:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

// watchEventFrom maps a COSI event; ok is false for events without a use.
func watchEventFrom(ev state.Event) (WatchEvent, bool) {
	switch ev.Type {
	case state.Errored:
		return WatchEvent{Kind: "error", Err: wrapGRPCErr(ev.Error)}, true
	case state.Bootstrapped:
		return WatchEvent{Kind: "bootstrapped"}, true
	case state.Created, state.Updated, state.Destroyed:
		if ev.Resource == nil {
			return WatchEvent{}, false
		}
		return WatchEvent{
			Kind: strings.ToLower(ev.Type.String()),
			Meta: metaFromResource(ev.Resource),
		}, true
	}
	return WatchEvent{}, false
}

// wrapGRPCErr keeps the gRPC status code visible in the message so
// IsPermissionDenied matches for both sources.
func wrapGRPCErr(err error) error {
	if err == nil {
		return nil
	}
	if IsPermissionDenied(err) && !strings.Contains(err.Error(), "PermissionDenied") {
		return fmt.Errorf("PermissionDenied: %w", err)
	}
	return err
}
