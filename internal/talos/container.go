package talos

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CRIID is a CRI container ID as `containers` prints it: a pod sandbox is
// "namespace/pod", a container is "namespace/pod:name:id12".
type CRIID struct {
	Namespace string
	Pod       string
	Name      string // container name; empty for a pod sandbox
	ShortID   string // first 12 characters of the runtime ID
	CRI       bool   // false for Talos system containers such as "apid"
}

// ParseCRIID splits a container ID into its parts. IDs without a "/" are
// Talos system containers and come back with only Name set.
func ParseCRIID(id string) CRIID {
	ns, rest, ok := strings.Cut(id, "/")
	if !ok {
		return CRIID{Name: id}
	}
	pod, rest, ok := strings.Cut(rest, ":")
	if !ok {
		return CRIID{Namespace: ns, Pod: pod, CRI: true}
	}
	name, short, _ := strings.Cut(rest, ":")
	return CRIID{Namespace: ns, Pod: pod, Name: name, ShortID: short, CRI: true}
}

// ImageRef is a container image reference split into its parts.
type ImageRef struct {
	Registry string
	Repo     string
	Tag      string
	Digest   string
}

// ParseImageRef splits "registry/repo/name:tag@sha256:…". A first path
// component counts as a registry when it holds a "." or ":" or is
// "localhost", as the reference grammar does.
func ParseImageRef(ref string) ImageRef {
	var r ImageRef
	if i := strings.Index(ref, "@"); i >= 0 {
		r.Digest = ref[i+1:]
		ref = ref[:i]
	}
	// A tag is after the last ":" and after the last "/".
	if i := strings.LastIndex(ref, ":"); i >= 0 && i > strings.LastIndex(ref, "/") {
		r.Tag = ref[i+1:]
		ref = ref[:i]
	}
	if first, rest, ok := strings.Cut(ref, "/"); ok &&
		(strings.ContainsAny(first, ".:") || first == "localhost") {
		r.Registry = first
		ref = rest
	}
	r.Repo = ref
	return r
}

// parseStatsLines parses `stats`: NODE NAMESPACE [└─] ID MEMORY(MB) CPU. Pod
// children carry a tree marker before the ID that shifts the columns.
func parseStatsLines(data []byte) []StatsResult {
	var results []StatsResult
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		idIdx := 2
		if len(f) > 2 && strings.HasPrefix(f[2], "└") {
			idIdx = 3
		}
		if len(f) < idIdx+3 {
			continue
		}
		memMB, _ := strconv.ParseFloat(f[idIdx+1], 64)
		cpuNanos, _ := strconv.ParseInt(f[idIdx+2], 10, 64)
		results = append(results, StatsResult{ID: f[idIdx], MemoryMB: memMB, CPUNanos: cpuNanos})
	}
	return results
}

// namespaceArgs selects the containerd namespace on containers, stats and
// logs: nothing for the default "system", the CRI namespace for k8s.io.
func (c *Client) namespaceArgs(namespace string) []string {
	if namespace == "k8s.io" || namespace == "cri" {
		if c.ModernCLI() {
			return []string{"--namespace=cri"}
		}
		return []string{"-k"}
	}
	return nil
}

// GetContainerStats returns the stats of the containers of one namespace.
func (c *Client) GetContainerStats(ctx context.Context, node, namespace string) ([]StatsResult, error) {
	args := append([]string{"stats", "-n", node}, c.namespaceArgs(namespace)...)
	data, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseStatsLines(data), nil
}

// parseMemoryTotalMB reads TOTAL (MB) of the node from `memory` output:
// NODE TOTAL USED FREE SHARED BUFFERS CACHE AVAILABLE.
func parseMemoryTotalMB(data []byte) (float64, bool) {
	for i, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 2 {
			continue
		}
		if v, err := strconv.ParseFloat(f[1], 64); err == nil {
			return v, true
		}
	}
	return 0, false
}

// GetNodeMemoryTotalMB returns the node's total memory in MB.
func (c *Client) GetNodeMemoryTotalMB(ctx context.Context, node string) (float64, error) {
	data, err := c.run(ctx, "memory", "-n", node)
	if err != nil {
		return 0, err
	}
	v, ok := parseMemoryTotalMB(data)
	if !ok {
		return 0, fmt.Errorf("unexpected memory output")
	}
	return v, nil
}

func (c *Client) containerLogArgs(node, namespace, id string, follow bool, tail int) []string {
	args := []string{"logs", "-n", node}
	args = append(args, c.namespaceArgs(namespace)...)
	if follow {
		args = append(args, "-f")
	}
	return append(args, "--tail", strconv.Itoa(tail), id)
}

// GetContainerLogs returns the last tail lines of a container's logs without
// following.
func (c *Client) GetContainerLogs(ctx context.Context, node, namespace, id string, tail int) ([]string, error) {
	data, err := c.run(ctx, c.containerLogArgs(node, namespace, id, false, tail)...)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = StripNodePrefix(node, l)
	}
	return lines, nil
}

// StreamContainerLogs follows a container's logs, like StreamLogs but in the
// container's namespace.
func (c *Client) StreamContainerLogs(ctx context.Context, node, namespace, id string, ch chan<- string) {
	cmd := exec.CommandContext(ctx, "talosctl", append(c.baseArgs(), c.containerLogArgs(node, namespace, id, true, 500)...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ch <- fmt.Sprintf("ERROR: %v", err)
		return
	}
	if err := cmd.Start(); err != nil {
		ch <- fmt.Sprintf("ERROR: %v", err)
		return
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			cmd.Process.Kill() //nolint:errcheck
			return
		case ch <- StripNodePrefix(node, scanner.Text()):
		}
	}
	cmd.Wait() //nolint:errcheck
}
