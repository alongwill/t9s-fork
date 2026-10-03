package talos

import (
	"context"
	"encoding/json"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// rdEnvelope mirrors one `get rd -o json` object. Keys are the COSI
// ResourceDefinitionSpec yaml tags; Sensitivity is kept raw so any
// representation (string, number, bool) is tolerated.
type rdEnvelope struct {
	Spec struct {
		Type             string          `json:"type"`
		DisplayType      string          `json:"displayType"`
		Aliases          []string        `json:"aliases"`
		AllAliases       []string        `json:"allAliases"`
		DefaultNamespace string          `json:"defaultNamespace"`
		Sensitivity      json.RawMessage `json:"sensitivity"`
	} `json:"spec"`
}

type resourceEnvelope struct {
	Metadata struct {
		Namespace string `json:"namespace"`
		Type      string `json:"type"`
		ID        string `json:"id"`
		Version   any    `json:"version"`
		Phase     string `json:"phase"`
		Owner     string `json:"owner"`
	} `json:"metadata"`
}

func isSensitive(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	switch strings.Trim(s, `"`) {
	case "", "null", "0", "false", "none", "nonsensitive":
		return false
	}
	return true
}

// GetResourceDefinitions lists every resource type known to the node.
func (c *Client) GetResourceDefinitions(ctx context.Context, node string) ([]ResourceDef, error) {
	data, err := c.run(ctx, "get", "rd", "-n", node, "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseResourceDefs(data)
}

func parseResourceDefs(data []byte) ([]ResourceDef, error) {
	envs, err := parseJSONStream[rdEnvelope](data)
	if err != nil {
		return nil, err
	}
	defs := make([]ResourceDef, 0, len(envs))
	for _, e := range envs {
		if e.Spec.Type == "" {
			continue
		}
		aliases := e.Spec.Aliases
		if len(aliases) == 0 {
			aliases = e.Spec.AllAliases
		}
		defs = append(defs, ResourceDef{
			Type:             e.Spec.Type,
			DisplayType:      e.Spec.DisplayType,
			Aliases:          aliases,
			DefaultNamespace: e.Spec.DefaultNamespace,
			Sensitive:        isSensitive(e.Spec.Sensitivity),
		})
	}
	return defs, nil
}

// ListResources returns the instances of one type. Empty output is an empty slice.
func (c *Client) ListResources(ctx context.Context, node, ns, typ string) ([]ResourceMeta, error) {
	data, err := c.run(ctx, "get", typ, "-n", node, "--namespace", ns, "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseResourceList(data)
}

func parseResourceList(data []byte) ([]ResourceMeta, error) {
	envs, err := parseJSONStream[resourceEnvelope](data)
	if err != nil {
		return nil, err
	}
	out := make([]ResourceMeta, 0, len(envs))
	for _, e := range envs {
		if e.Metadata.ID == "" {
			continue
		}
		v := ""
		switch t := e.Metadata.Version.(type) {
		case string:
			v = t
		case float64:
			v = strings.TrimSuffix(strings.TrimSuffix(jsonNum(t), ".0"), ".")
		}
		out = append(out, ResourceMeta{
			Namespace: e.Metadata.Namespace,
			Type:      e.Metadata.Type,
			ID:        e.Metadata.ID,
			Version:   v,
			Phase:     e.Metadata.Phase,
			Owner:     e.Metadata.Owner,
		})
	}
	return out, nil
}

func jsonNum(f float64) string {
	b, _ := json.Marshal(f)
	return string(b)
}

// GetResourceYAML returns the YAML of one resource instance.
func (c *Client) GetResourceYAML(ctx context.Context, node, ns, typ, id string) (string, error) {
	data, err := c.run(ctx, "get", typ, id, "-n", node, "--namespace", ns, "-o", "yaml")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// IsPermissionDenied reports whether err came from a missing-role rejection.
func IsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.PermissionDenied {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "PermissionDenied") || strings.Contains(s, "not authorized")
}

// IsNotFound reports whether err says the resource (or its type) does not exist.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if status.Code(err) == codes.NotFound {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "NotFound") || strings.Contains(s, "not found") ||
		strings.Contains(s, "doesn't exist") || strings.Contains(s, "is not registered")
}
