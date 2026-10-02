package talos

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// --- talosctl client version gating ---

// modernMinor is the first talosctl minor release whose flags t9s relies on
// when available (upgrade --drain/--progress, containers --namespace).
const modernMinor = 14

// SetClientVersion records the talosctl client version (e.g. "v1.14.2") so
// commands can pick flags the binary understands.
func (c *Client) SetClientVersion(v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientMinor = ParseMinor(v)
}

// ModernCLI reports whether talosctl is >= 1.14. An unknown version is
// treated as modern: 1.14 is the release t9s targets.
func (c *Client) ModernCLI() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clientMinor < 0 || c.clientMinor >= modernMinor
}

// ParseMinor returns the minor component of a "v1.14.2"-style version, or -1.
func ParseMinor(v string) int {
	parts := strings.SplitN(strings.TrimPrefix(v, "v"), ".", 3)
	if len(parts) < 2 || parts[1] == "" {
		return -1
	}
	n := 0
	for _, ch := range parts[1] {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// --- tabular output parsing ---

var columnGap = regexp.MustCompile(`\S+(?: \S+)*`)

// parseTable parses talosctl tabwriter output into one map per row, keyed by
// header name. Cells are cut at the header's column offsets (in runes), so
// empty cells — e.g. the LABEL column of `processes` on nodes without
// SELinux — don't shift the following columns. The last column takes the
// rest of the line, since COMMAND / LAST EVENT may contain spaces.
func parseTable(data []byte) []map[string]string {
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}
	header := []rune(lines[0])
	type column struct {
		name  string
		start int
	}
	var cols []column
	for _, loc := range columnGap.FindAllStringIndex(string(header), -1) {
		start := len([]rune(string(header)[:loc[0]]))
		cols = append(cols, column{name: string(header)[loc[0]:loc[1]], start: start})
	}
	if len(cols) == 0 {
		return nil
	}

	var rows []map[string]string
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		r := []rune(line)
		row := make(map[string]string, len(cols))
		for i, col := range cols {
			if col.start >= len(r) {
				row[col.name] = ""
				continue
			}
			end := len(r)
			if i+1 < len(cols) && cols[i+1].start < end {
				end = cols[i+1].start
			}
			row[col.name] = strings.TrimSpace(string(r[col.start:end]))
		}
		rows = append(rows, row)
	}
	return rows
}

// --- Machine status (stage / readiness) ---

// MachineStatus is the node lifecycle state from MachineStatuses.runtime.talos.dev.
type MachineStatus struct {
	Stage           string   // booting, installing, maintenance, running, rebooting, upgrading, …
	Ready           bool     // all readiness conditions met
	UnmetConditions []string // e.g. "nodeReady: node not ready"
}

type machineStatusEnvelope struct {
	Spec struct {
		Stage  string `json:"stage"`
		Status struct {
			Ready           bool `json:"ready"`
			UnmetConditions []struct {
				Name   string `json:"name"`
				Reason string `json:"reason"`
			} `json:"unmetConditions"`
		} `json:"status"`
	} `json:"spec"`
}

func (c *Client) GetMachineStatus(ctx context.Context, node string) (MachineStatus, error) {
	data, err := c.run(ctx, "get", "machinestatus", "-n", node, "-o", "json")
	if err != nil {
		return MachineStatus{}, err
	}
	envs, _ := parseJSONStream[machineStatusEnvelope](data)
	if len(envs) == 0 {
		return MachineStatus{}, fmt.Errorf("machinestatus not found")
	}
	e := envs[0]
	ms := MachineStatus{Stage: e.Spec.Stage, Ready: e.Spec.Status.Ready}
	for _, u := range e.Spec.Status.UnmetConditions {
		ms.UnmetConditions = append(ms.UnmetConditions, u.Name+": "+u.Reason)
	}
	return ms, nil
}

// NodeStatus maps a MachineStatus to the short status shown in the node list.
func (ms MachineStatus) NodeStatus() string {
	switch {
	case ms.Stage == "running" && ms.Ready:
		return "ready"
	case ms.Stage == "running":
		return "not ready"
	case ms.Stage == "":
		return "unknown"
	default:
		return ms.Stage
	}
}

// --- Kubernetes version ---

type kubeletImageEnvelope struct {
	Spec struct {
		Image string `json:"image"`
	} `json:"spec"`
}

// imageTag extracts the tag from an image reference, ignoring any digest
// and registry port: "reg:5000/siderolabs/kubelet:v1.37.0@sha256:…" → "v1.37.0".
func imageTag(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if i := strings.LastIndex(ref, ":"); i >= 0 && !strings.Contains(ref[i:], "/") {
		return ref[i+1:]
	}
	return ""
}

// GetKubernetesVersion returns the kubelet version (without "v") of a node.
//
// Talos 1.14 added the non-sensitive KubeletStatus resource; KubeletSpec is
// now sensitive (os:admin only), so it is only used as a fallback for older
// nodes. On a Kubernetes-less node (Talos 1.14+) neither resource exists and
// ErrNoKubernetes is returned.
func (c *Client) GetKubernetesVersion(ctx context.Context, node string) (string, error) {
	var lastErr error
	for _, resource := range []string{"kubeletstatus", "kubeletspec"} {
		data, err := c.run(ctx, "get", resource, "-n", node, "-o", "json")
		if err != nil {
			lastErr = err
			continue
		}
		envs, _ := parseJSONStream[kubeletImageEnvelope](data)
		for _, e := range envs {
			if tag := imageTag(e.Spec.Image); tag != "" {
				return strings.TrimPrefix(tag, "v"), nil
			}
		}
		if len(envs) == 0 {
			// The resource type exists but there is no kubelet:
			// Kubernetes is not configured on this node.
			return "", ErrNoKubernetes
		}
	}
	return "", lastErr
}

// ErrNoKubernetes means the node runs without kubelet (k8s-less mode).
var ErrNoKubernetes = errors.New("kubernetes is not configured on this node")

// NodeDetails is per-node information fetched after the member list.
type NodeDetails struct {
	KubeVersion string // "v1.37.0", empty when unknown / k8s-less
	NoK8s       bool   // node definitely runs without Kubernetes
	Status      MachineStatus
	StatusErr   error
}

// GetNodeDetails fetches kubelet version and machine status for every node
// concurrently. Returns node IP → details.
func (c *Client) GetNodeDetails(ctx context.Context, nodes []Node) map[string]NodeDetails {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out = make(map[string]NodeDetails, len(nodes))
	)
	for _, n := range nodes {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			var d NodeDetails
			d.Status, d.StatusErr = c.GetMachineStatus(ctx, ip)
			if v, err := c.GetKubernetesVersion(ctx, ip); err == nil {
				d.KubeVersion = "v" + v
			} else if errors.Is(err, ErrNoKubernetes) {
				d.NoK8s = true
			}
			mu.Lock()
			out[ip] = d
			mu.Unlock()
		}(n.IP)
	}
	wg.Wait()
	return out
}
