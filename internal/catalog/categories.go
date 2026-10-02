// Package catalog groups Talos resource types into browsable categories.
package catalog

import (
	"strings"

	"github.com/florianspk/t9s/internal/talos"
)

type Category struct{ Key, Label string }

// Categories is the display order of the browser's first pane.
var Categories = []Category{
	{"networking", "Networking"},
	{"siderolink", "SideroLink"},
	{"kubernetes", "Kubernetes"},
	{"cluster", "Cluster and discovery"},
	{"block", "Block and volumes"},
	{"storage", "Storage (LVM/RAID)"},
	{"cri", "CRI and registries"},
	{"containers", "Containers"},
	{"hypervisor", "Hypervisor and VM images"},
	{"hardware", "Hardware"},
	{"security", "Security"},
	{"extensions", "Extensions"},
	{"runtime", "Runtime and OS"},
	{"other", "Other"},
}

// suffixCategory maps the second segment of a resource type
// (<Kind>.<suffix>.talos.dev) to a category key.
var suffixCategory = map[string]string{
	"net": "networking", "kubespan": "networking",
	"siderolink": "siderolink",
	"kubernetes": "kubernetes", "k8s": "kubernetes",
	"cluster": "cluster", "etcd": "cluster",
	"block":      "block",
	"storage":    "storage",
	"cri":        "cri",
	"containers": "containers",
	"hypervisor": "hypervisor",
	"hardware":   "hardware",
	"security":   "security", "secrets": "security",
	"runtime": "runtime", "v1alpha1": "runtime", "files": "runtime", "perf": "runtime", "config": "runtime",
}

// CategoryFor returns the category key for a resource type. Display-type
// overrides win over the suffix table; unknown suffixes land in "other".
func CategoryFor(def talos.ResourceDef) string {
	if strings.HasPrefix(def.DisplayType, "Extension") ||
		strings.HasPrefix(def.Type, "Extension") {
		return "extensions"
	}
	parts := strings.Split(def.Type, ".")
	if len(parts) < 2 {
		return "other"
	}
	if k, ok := suffixCategory[parts[1]]; ok {
		return k
	}
	return "other"
}

// Label returns the display label for a category key.
func Label(key string) string {
	for _, c := range Categories {
		if c.Key == key {
			return c.Label
		}
	}
	return key
}
