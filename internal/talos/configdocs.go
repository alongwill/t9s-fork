package talos

import (
	"errors"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LegacyConfigKind labels the v1alpha1 document, which has no `kind:` field.
const (
	LegacyConfigKind = "v1alpha1"
	LegacyConfigName = "Config"
)

// ConfigDoc is one document of a node's multi-document machine config.
type ConfigDoc struct {
	Kind string
	Name string // `name:` if present
	YAML string // original text of the document (comments and order kept)
}

var docSeparator = regexp.MustCompile(`^---(\s.*)?$`)

// SplitConfigDocs splits what GetMachineConfig returns into documents. It
// accepts either the full resource YAML (`spec:` holding the config as a
// string or mapping) or the bare multi-document stream.
func SplitConfigDocs(raw string) ([]ConfigDoc, error) {
	stream, err := unwrapMachineConfig(raw)
	if err != nil {
		return nil, err
	}
	var docs []ConfigDoc
	for _, chunk := range splitYAMLStream(stream) {
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(chunk), &node); err != nil {
			return nil, err
		}
		if len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
			continue // empty or comment-only document
		}
		root := node.Content[0]
		d := ConfigDoc{
			Kind: yamlScalar(root, "kind"),
			Name: yamlScalar(root, "name"),
			YAML: strings.Trim(chunk, "\n") + "\n",
		}
		if d.Kind == "" {
			d.Kind = LegacyConfigKind
			if d.Name == "" {
				d.Name = LegacyConfigName
			}
		}
		docs = append(docs, d)
	}
	return docs, nil
}

// unwrapMachineConfig returns the config stream inside a resource envelope,
// or raw itself when it is already a bare stream.
func unwrapMachineConfig(raw string) (string, error) {
	var node yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(raw))
	if err := dec.Decode(&node); err != nil {
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		return "", err
	}
	if len(node.Content) == 0 || node.Content[0].Kind != yaml.MappingNode {
		return raw, nil
	}
	root := node.Content[0]
	meta, spec := yamlChild(root, "metadata"), yamlChild(root, "spec")
	if meta == nil || spec == nil {
		return raw, nil
	}
	switch spec.Kind {
	case yaml.ScalarNode:
		return spec.Value, nil
	case yaml.MappingNode:
		b, err := yaml.Marshal(spec)
		return string(b), err
	}
	return raw, nil
}

// splitYAMLStream cuts on column-0 `---` lines. Block scalars are indented,
// so such a line is always a document boundary.
func splitYAMLStream(s string) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(s, "\n") {
		if docSeparator.MatchString(line) {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return out
}

func yamlChild(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func yamlScalar(m *yaml.Node, key string) string {
	if n := yamlChild(m, key); n != nil && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return ""
}
