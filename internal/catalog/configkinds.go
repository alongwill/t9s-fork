package catalog

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strconv"
	"sync"
)

//go:embed config-kinds.json
var configKindsJSON []byte

// ConfigKind is one machine-config document kind (e.g. DHCPv4Config).
type ConfigKind struct {
	Kind   string `json:"kind"`
	Group  string `json:"group"`
	Since  string `json:"since"`
	Desc   string `json:"desc"`
	Ubuntu string `json:"ubuntu"` // the closest Ubuntu tool or file
}

var (
	configKindsOnce sync.Once
	configKinds     []ConfigKind
)

// ConfigKinds returns the embedded catalogue, parsed once.
func ConfigKinds() []ConfigKind {
	configKindsOnce.Do(func() {
		var doc struct {
			Kinds []ConfigKind `json:"kinds"`
		}
		if err := json.Unmarshal(configKindsJSON, &doc); err == nil {
			configKinds = doc.Kinds
		}
	})
	return configKinds
}

// ConfigCategoryFor maps a config group to a category key.
func ConfigCategoryFor(group string) string {
	switch group {
	case "network":
		return "networking"
	case "container":
		return "containers"
	case "kubernetes", "block", "storage", "cri", "runtime", "hardware",
		"hypervisor", "cluster", "security", "extensions", "siderolink":
		return group
	}
	return "other"
}

var majorMinorRe = regexp.MustCompile(`^v?(\d+)\.(\d+)`)

// parseMajorMinor extracts (major, minor) from "v1.14", "1.15.0-alpha.0", etc.
func parseMajorMinor(v string) (int, int, bool) {
	m := majorMinorRe.FindStringSubmatch(v)
	if m == nil {
		return 0, 0, false
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	return maj, min, true
}

// KindsAvailable drops kinds introduced after the node's vMAJOR.MINOR. An
// empty or unparsable node version keeps everything.
func KindsAvailable(talosVersion string) []ConfigKind {
	all := ConfigKinds()
	nMaj, nMin, ok := parseMajorMinor(talosVersion)
	if !ok {
		return all
	}
	out := make([]ConfigKind, 0, len(all))
	for _, k := range all {
		kMaj, kMin, kok := parseMajorMinor(k.Since)
		if kok && (kMaj > nMaj || (kMaj == nMaj && kMin > nMin)) {
			continue
		}
		out = append(out, k)
	}
	return out
}
