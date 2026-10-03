package talos

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/meta"
	"github.com/cosi-project/runtime/pkg/resource/typed"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	"github.com/cosi-project/runtime/pkg/state/impl/namespaced"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const testType = "Widgets.test.talos.dev"

type widgetSpec struct {
	Color string `yaml:"color"`
	Size  int    `yaml:"size"`
}

func (s widgetSpec) DeepCopy() widgetSpec { return s }

type widgetExt struct{}

func (widgetExt) ResourceDefinition() meta.ResourceDefinitionSpec {
	return meta.ResourceDefinitionSpec{Type: testType, DisplayType: "Widget", DefaultNamespace: "test"}
}

type widget = typed.Resource[widgetSpec, widgetExt]

func newWidget(id, color string, size int) *widget {
	return typed.NewResource[widgetSpec, widgetExt](
		resource.NewMetadata("test", testType, id, resource.VersionUndefined),
		widgetSpec{Color: color, Size: size})
}

func newTestSource(t *testing.T) (*grpcSource, state.State) {
	t.Helper()
	st := state.WrapCore(namespaced.NewState(inmem.Build))
	ctx := context.Background()
	for _, w := range []*widget{newWidget("a", "red", 1), newWidget("b", "blue", 2)} {
		if err := st.Create(ctx, w); err != nil {
			t.Fatal(err)
		}
	}
	for _, sp := range []meta.ResourceDefinitionSpec{
		{Type: testType, DisplayType: "Widget", DefaultNamespace: "test", Aliases: []string{"wd"}, AllAliases: []string{"wd", "widget"}},
		{Type: "Secrets.test.talos.dev", DisplayType: "Secret", DefaultNamespace: "test", AllAliases: []string{"sec"}, Sensitivity: meta.Sensitive},
	} {
		rd, err := meta.NewResourceDefinition(sp)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Create(ctx, rd); err != nil {
			t.Fatal(err)
		}
	}
	return newGRPCSourceFromState(st, nil, nil), st
}

func TestGRPCSourceDefinitions(t *testing.T) {
	s, _ := newTestSource(t)
	defs, err := s.Definitions(context.Background(), "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ResourceDef{}
	for _, d := range defs {
		by[d.Type] = d
	}
	w := by[testType]
	if w.DisplayType != "Widget" || w.DefaultNamespace != "test" || w.Sensitive || len(w.Aliases) == 0 || w.Aliases[0] != "wd" {
		t.Fatalf("widget def = %+v", w)
	}
	if sec := by["Secrets.test.talos.dev"]; !sec.Sensitive || len(sec.Aliases) == 0 {
		t.Fatalf("secret def = %+v (aliases fall back to allAliases, sensitive set)", sec)
	}
}

func TestGRPCSourceList(t *testing.T) {
	s, _ := newTestSource(t)
	items, err := s.List(context.Background(), "n", "test", testType)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	got := map[string]ResourceMeta{}
	for _, it := range items {
		got[it.ID] = it
	}
	a := got["a"]
	if a.Namespace != "test" || a.Type != testType || a.Version != "1" || a.Phase != "running" {
		t.Fatalf("a = %+v", a)
	}
	empty, err := s.List(context.Background(), "n", "test", "Nothing.test.talos.dev")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty list = %v, %v", empty, err)
	}
}

func TestGRPCSourceGetYAMLMatchesCLIShape(t *testing.T) {
	s, _ := newTestSource(t)
	y, err := s.GetYAML(context.Background(), "10.0.0.1", "test", testType, "a")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"node: 10.0.0.1\n", "metadata:\n", "namespace: test", "type: " + testType, "id: a", "spec:\n", "color: red", "size: 1"} {
		if !strings.Contains(y, want) {
			t.Errorf("yaml missing %q:\n%s", want, y)
		}
	}
	if !strings.HasPrefix(y, "node: ") {
		t.Errorf("yaml must start with the node line:\n%s", y)
	}
	if _, err := s.GetYAML(context.Background(), "n", "test", testType, "zzz"); err == nil {
		t.Fatal("missing resource must error")
	}
}

func recv(t *testing.T, ch <-chan WatchEvent) WatchEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a watch event")
	}
	return WatchEvent{}
}

func TestGRPCSourceWatch(t *testing.T) {
	s, st := newTestSource(t)
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan WatchEvent, 16)
	done := make(chan error, 1)
	go func() { done <- s.Watch(ctx, "n", "test", testType, out) }()

	created := map[string]bool{}
	for range 2 {
		ev := recv(t, out)
		if ev.Kind != "created" {
			t.Fatalf("bootstrap event = %+v", ev)
		}
		created[ev.Meta.ID] = true
	}
	if !created["a"] || !created["b"] {
		t.Fatalf("bootstrap ids = %v", created)
	}
	if ev := recv(t, out); ev.Kind != "bootstrapped" {
		t.Fatalf("want bootstrapped, got %+v", ev)
	}

	bg := context.Background()
	if _, err := safeModify(bg, st, "a"); err != nil {
		t.Fatal(err)
	}
	if ev := recv(t, out); ev.Kind != "updated" || ev.Meta.ID != "a" || ev.Meta.Version != "2" {
		t.Fatalf("update event = %+v", ev)
	}
	if err := st.Destroy(bg, newWidget("b", "", 0).Metadata()); err != nil {
		t.Fatal(err)
	}
	if ev := recv(t, out); ev.Kind != "destroyed" || ev.Meta.ID != "b" {
		t.Fatalf("destroy event = %+v", ev)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Watch after cancel = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after cancel")
	}
}

func safeModify(ctx context.Context, st state.State, id string) (*widget, error) {
	r, err := st.Get(ctx, resource.NewMetadata("test", testType, id, resource.VersionUndefined))
	if err != nil {
		return nil, err
	}
	w := r.(*widget)
	w.TypedSpec().Color = "green"
	return w, st.Update(ctx, w)
}

func TestWatchEventFrom(t *testing.T) {
	if ev, ok := watchEventFrom(state.Event{Type: state.Errored, Error: status.Error(codes.PermissionDenied, "no")}); !ok || ev.Kind != "error" || !IsPermissionDenied(ev.Err) {
		t.Fatalf("errored = %+v %v", ev, ok)
	}
	if _, ok := watchEventFrom(state.Event{Type: state.Created}); ok {
		t.Fatal("created without a resource must be skipped")
	}
}

func TestIsPermissionDeniedGRPCStatus(t *testing.T) {
	err := status.Error(codes.PermissionDenied, "rpc denied")
	if !IsPermissionDenied(err) || !IsPermissionDenied(wrapGRPCErr(err)) {
		t.Fatal("gRPC PermissionDenied must be recognised")
	}
	if IsPermissionDenied(status.Error(codes.NotFound, "x")) {
		t.Fatal("NotFound is not a permission error")
	}
}
