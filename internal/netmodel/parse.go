package netmodel

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/florianspk/t9s/internal/talos"
)

// Res is one resource as `get -o yaml` prints it: the identity plus the spec
// as a generic map (the model reads only the keys it knows).
type Res struct {
	Namespace string
	ID        string
	Spec      map[string]any
}

// ParseRes reads a resource's YAML (metadata and spec). ns is used when the
// metadata carries none.
func ParseRes(y, ns string) (Res, error) {
	var doc struct {
		Metadata struct {
			Namespace string `yaml:"namespace"`
			ID        string `yaml:"id"`
		} `yaml:"metadata"`
		Spec map[string]any `yaml:"spec"`
	}
	if err := yaml.Unmarshal([]byte(y), &doc); err != nil {
		return Res{}, err
	}
	r := Res{Namespace: doc.Metadata.Namespace, ID: doc.Metadata.ID, Spec: doc.Spec}
	if r.Namespace == "" {
		r.Namespace = ns
	}
	if r.Spec == nil {
		r.Spec = map[string]any{}
	}
	return r, nil
}

// Inputs is everything Build reads, already fetched.
type Inputs struct {
	Node string

	LinkStatuses, AddressStatuses, RouteStatuses []Res
	// Specs of both namespaces: Namespace tells merged (network) from per-layer (network-config).
	LinkSpecs, AddressSpecs, RouteSpecs []Res
	OperatorSpecs                       []Res
	Hostnames, Resolvers, NodeAddresses []Res
	KubeSpanPeers, KubeSpanIdentities   []Res

	Docs []talos.ConfigDoc // all documents of the machine config
}

// DisplayType shortens a full type: "LinkStatuses.net.talos.dev" -> "LinkStatus".
func DisplayType(typ string) string {
	name, _, _ := strings.Cut(typ, ".")
	switch {
	case strings.HasSuffix(name, "ies"):
		return strings.TrimSuffix(name, "ies") + "y"
	case strings.HasSuffix(name, "ses"):
		return strings.TrimSuffix(name, "es")
	case strings.HasSuffix(name, "s"):
		return strings.TrimSuffix(name, "s")
	}
	return name
}

// --- generic map accessors ---

func str(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case nil:
		return ""
	case string:
		return v
	case bool, int, int64, uint64, float64, uint32:
		return fmt.Sprint(v)
	}
	return ""
}

func num(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func boolOf(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

func sub(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

func list(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

func strList(m map[string]any, key string) []string {
	var out []string
	for _, x := range list(m, key) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// tailAfterLayer strips a "<layer>/" prefix from a network-config ID
// (network.LayeredID). IDs without a known layer prefix come back unchanged.
func tailAfterLayer(id string) (layer, rest string) {
	l, r, ok := strings.Cut(id, "/")
	if ok && isLayer(l) {
		return l, r
	}
	return "", id
}

// Talos's config layers, lowest priority first (network.ConfigLayer).
var layerOrder = []string{"default", "cmdline", "platform", "operator", "configuration"}

func isLayer(s string) bool { return layerRank(s) >= 0 }

// layerRank is the priority of a layer (higher wins), -1 when unknown.
func layerRank(s string) int {
	for i, l := range layerOrder {
		if l == s {
			return i
		}
	}
	return -1
}

// specLayer reads the layer of a spec: its `layer` field, else the ID prefix.
func specLayer(r Res) string {
	if l := str(r.Spec, "layer"); l != "" {
		return l
	}
	l, _ := tailAfterLayer(r.ID)
	return l
}
