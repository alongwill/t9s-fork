package talos

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Client struct {
	ConfigPath string
	Context    string

	mu          sync.Mutex
	clientMinor int // talosctl minor version, -1 until known
}

func New(configPath, ctx string) *Client {
	return &Client{ConfigPath: configPath, Context: ctx, clientMinor: -1}
}

func (c *Client) baseArgs() []string {
	var args []string
	if c.ConfigPath != "" {
		args = append(args, "--talosconfig", c.ConfigPath)
	}
	if c.Context != "" {
		args = append(args, "--context", c.Context)
	}
	return args
}

// GetClientVersion returns the talosctl binary version without any network call.
func GetClientVersion(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "talosctl", "version", "--client").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Tag:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Tag:"))
		}
	}
	return ""
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	cmdArgs := append(c.baseArgs(), args...)
	cmd := exec.CommandContext(ctx, "talosctl", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// parseJSONStream decodes a stream of JSON objects (pretty-printed or NDJSON).
func parseJSONStream[T any](data []byte) ([]T, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var results []T
	for {
		var item T
		err := dec.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Try to advance past the bad token. If Token() also fails the
			// decoder is stuck (e.g. truly malformed bytes) — stop to avoid
			// an infinite loop.
			if _, tokErr := dec.Token(); tokErr != nil {
				break
			}
			continue
		}
		results = append(results, item)
	}
	return results, nil
}

// --- Members / Nodes ---

type memberEnvelope struct {
	Node string `json:"node"` // IP of the Talos node that produced this response
	Spec struct {
		Addresses       []string `json:"addresses"`
		Hostname        string   `json:"hostname"`
		MachineType     string   `json:"machineType"`
		OperatingSystem string   `json:"operatingSystem"`
	} `json:"spec"`
}

func (c *Client) GetNodes(ctx context.Context) ([]Node, error) {
	data, err := c.run(ctx, "get", "members", "-o", "json")
	if err != nil {
		return nil, err
	}
	envs, err := parseJSONStream[memberEnvelope](data)
	if err != nil {
		return nil, err
	}

	// Deduplicate by hostname — each cluster node reports all members.
	// Use the `node` field as the target IP: it's the actual node IP that
	// responded, not a VIP (which may appear as addresses[0] on controlplanes).
	seen := map[string]bool{}
	var nodes []Node
	for _, e := range envs {
		if seen[e.Spec.Hostname] {
			continue
		}
		seen[e.Spec.Hostname] = true

		// Find the actual node IP to use with talosctl -n.
		// The `node` field is the responding node's IP; for a member, one of
		// its addresses should match it (the self-report case).
		// For workers reported by another node, fall back to the last address
		// (Talos lists VIP first, actual IP last for multi-address nodes).
		ip := pickMemberIP(e.Node, e.Spec.Addresses)

		// Display IP: node IP only (VIP would be confusing in the UI).
		displayIP := ip

		version := e.Spec.OperatingSystem
		if i := strings.Index(version, "("); i >= 0 {
			version = strings.TrimRight(version[i+1:], ")")
		}
		nodes = append(nodes, Node{
			Hostname:  e.Spec.Hostname,
			IP:        ip,        // used for talosctl -n <ip>
			DisplayIP: displayIP, // shown in the UI
			Role:      e.Spec.MachineType,
			Version:   version,
		})
	}
	return nodes, nil
}

// pickMemberIP chooses the address t9s targets for a member. An address equal
// to the responding node wins; otherwise the last IPv4 address (a VIP is listed
// first), otherwise the last address. Link-local addresses are skipped unless
// nothing else is left.
func pickMemberIP(node string, addrs []string) string {
	for _, a := range addrs {
		if a == node {
			return a
		}
	}
	var lastV4, lastAny, lastRaw string
	for _, a := range addrs {
		lastRaw = a
		ip, err := netip.ParseAddr(a)
		if err != nil || ip.IsLinkLocalUnicast() {
			continue
		}
		lastAny = a
		if ip.Is4() {
			lastV4 = a
		}
	}
	switch {
	case lastV4 != "":
		return lastV4
	case lastAny != "":
		return lastAny
	}
	return lastRaw
}

// --- Services ---

func (c *Client) GetServices(ctx context.Context, node string) ([]Service, error) {
	// `talosctl get servicestatuses` is not registered; use `talosctl services`.
	// Output: NODE  SERVICE  STATE  HEALTH  LAST CHANGE  LAST EVENT
	data, err := c.run(ctx, "services", "-n", node)
	if err != nil {
		return nil, err
	}
	return parseServiceLines(data), nil
}

// parseServiceLines parses the tabular output of `talosctl services`.
// Fields: NODE  SERVICE  STATE  HEALTH  [LAST CHANGE  LAST EVENT]
func parseServiceLines(data []byte) []Service {
	var services []Service
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header and blank lines
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		services = append(services, Service{
			ID:      fields[1],
			State:   fields[2],
			Healthy: fields[3],
		})
	}
	return services
}

// --- Extensions ---

type extensionEnvelope struct {
	Spec struct {
		Metadata struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			Description string `json:"description"`
		} `json:"metadata"`
	} `json:"spec"`
}

func (c *Client) GetExtensions(ctx context.Context, node string) ([]Extension, error) {
	data, err := c.run(ctx, "get", "extensions", "-n", node, "-o", "json")
	if err != nil {
		return nil, err
	}
	envs, err := parseJSONStream[extensionEnvelope](data)
	if err != nil {
		return nil, err
	}
	var exts []Extension
	for _, e := range envs {
		exts = append(exts, Extension{
			Name:        e.Spec.Metadata.Name,
			Version:     e.Spec.Metadata.Version,
			Description: e.Spec.Metadata.Description,
		})
	}
	return exts, nil
}

// --- Machine Config ---

func (c *Client) GetMachineConfig(ctx context.Context, node string) (string, error) {
	data, err := c.run(ctx, "get", "machineconfig", "v1alpha1", "-n", node, "-o", "yaml")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// --- Stats ---

func (c *Client) GetStats(ctx context.Context, node string) ([]StatsResult, error) {
	data, err := c.run(ctx, "stats", "-n", node)
	if err != nil {
		return nil, err
	}
	var results []StatsResult
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		// Output: NODE  NAMESPACE  ID  MEMORY(MB)  CPU
		if len(fields) < 5 {
			continue
		}
		memMB, _ := strconv.ParseFloat(fields[3], 64)
		cpuNanos, _ := strconv.ParseInt(fields[4], 10, 64)
		results = append(results, StatsResult{
			ID:       fields[2],
			MemoryMB: memMB,
			CPUNanos: cpuNanos,
		})
	}
	return results, nil
}

// --- Streaming ---

func (c *Client) StreamLogs(ctx context.Context, node, service string, ch chan<- string) {
	cmdArgs := append(c.baseArgs(), "logs", "-n", node, "-f", "--tail", "500", service)
	cmd := exec.CommandContext(ctx, "talosctl", cmdArgs...)
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

// StripNodePrefix removes the "<node>: " prefix `logs` puts on every line.
func StripNodePrefix(node, line string) string {
	return strings.TrimPrefix(line, node+": ")
}

func (c *Client) StreamDmesg(ctx context.Context, node string, ch chan<- string) {
	cmdArgs := append(c.baseArgs(), "dmesg", "-n", node, "-f")
	cmd := exec.CommandContext(ctx, "talosctl", cmdArgs...)
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

// UpgradeOptions controls `talosctl upgrade`.
type UpgradeOptions struct {
	Image    string
	Drain    bool // talosctl >= 1.14: cordon+drain the node first (needs Kubernetes)
	Preserve bool // talosctl < 1.14 only; a no-op against Talos >= 1.14 nodes
}

func (c *Client) UpgradeTalos(ctx context.Context, node string, opts UpgradeOptions, ch chan<- string) error {
	args := append(c.baseArgs(), upgradeArgs(node, opts, c.ModernCLI())...)
	return c.runStreaming(ctx, ch, args...)
}

func upgradeArgs(node string, opts UpgradeOptions, modern bool) []string {
	args := []string{"upgrade", "-n", node, "--image", opts.Image}
	if modern {
		// talosctl 1.14 upgrades through LifecycleService: image pull and
		// install progress are streamed, then it waits for the node to come
		// back. Plain progress keeps the output free of spinners/ANSI.
		return append(args, "--progress", "plain", fmt.Sprintf("--drain=%t", opts.Drain))
	}
	if opts.Preserve {
		args = append(args, "--preserve")
	}
	return args
}

// UpgradeK8s runs `talosctl upgrade-k8s`. talosctl 1.14 requires exactly one
// (controlplane) node.
func (c *Client) UpgradeK8s(ctx context.Context, node, version string, ch chan<- string) error {
	cmdArgs := append(c.baseArgs(), "upgrade-k8s", "--to", version)
	if node != "" {
		cmdArgs = append(cmdArgs, "-n", node)
	}
	return c.runStreaming(ctx, ch, cmdArgs...)
}

// --- Extension Catalog ---

// GetExtensionCatalog fetches the list of available Talos extensions for a given Talos version
// by pulling the ghcr.io/siderolabs/extensions:<talosVersion> image and reading descriptions.yaml.
func (c *Client) GetExtensionCatalog(ctx context.Context, talosVersion string) ([]CatalogExtension, error) {
	image := "ghcr.io/siderolabs/extensions:" + talosVersion
	cmd := exec.CommandContext(ctx, "crane", "export", "--platform", "linux/amd64", image, "-")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("crane not found: %w", err)
	}

	tr := tar.NewReader(stdout)
	var descData []byte
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cmd.Wait() //nolint:errcheck
			return nil, fmt.Errorf("read catalog: %w", err)
		}
		if hdr.Name == "descriptions.yaml" || strings.HasSuffix(hdr.Name, "/descriptions.yaml") {
			descData, err = io.ReadAll(tr)
			if err != nil {
				cmd.Wait() //nolint:errcheck
				return nil, err
			}
			break
		}
	}

	if err := cmd.Wait(); err != nil && descData == nil {
		return nil, fmt.Errorf("crane export: %s", strings.TrimSpace(stderr.String()))
	}
	if descData == nil {
		return nil, fmt.Errorf("descriptions.yaml not found for %s", talosVersion)
	}

	return parseExtensionCatalog(descData)
}

func parseExtensionCatalog(data []byte) ([]CatalogExtension, error) {
	var result []CatalogExtension
	lines := strings.Split(string(data), "\n")

	var cur *CatalogExtension
	inDesc := false

	flush := func() {
		if cur != nil {
			cur.Description = strings.TrimSpace(cur.Description)
			result = append(result, *cur)
			cur = nil
		}
	}

	for _, line := range lines {
		if line == "" {
			continue
		}
		// Top-level key: image ref (no leading whitespace, or YAML explicit key '? ...')
		if line[0] != ' ' && line[0] != '\t' && line[0] != ':' {
			flush()
			ref := strings.TrimSuffix(line, ":")
			ref = strings.TrimPrefix(ref, "? ") // YAML explicit key marker
			name := ref
			if i := strings.LastIndex(ref, "/"); i >= 0 {
				name = ref[i+1:]
			}
			if i := strings.Index(name, ":"); i >= 0 {
				name = name[:i]
			}
			cur = &CatalogExtension{Name: name, ImageRef: ref}
			inDesc = false
			continue
		}
		if cur == nil {
			continue
		}
		// YAML explicit value indicator ': field: val' — strip the leading ': '
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ": ") {
			trimmed = trimmed[2:]
		}
		if strings.HasPrefix(trimmed, "author:") {
			cur.Author = strings.TrimSpace(strings.TrimPrefix(trimmed, "author:"))
			inDesc = false
		} else if strings.HasPrefix(trimmed, "description:") {
			inDesc = true
		} else if inDesc {
			// Strip up to 4 spaces of leading indent from description continuation
			cur.Description += strings.TrimPrefix(strings.TrimPrefix(line, "    "), "  ") + " "
		}
	}
	flush()

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// --- Node actions ---

func (c *Client) Reboot(ctx context.Context, node string) error {
	// --wait=false sends the request and returns immediately.
	// Default (--wait=true) would wait for the node to come back up,
	// which exceeds any reasonable timeout and always returns an error.
	_, err := c.run(ctx, "reboot", "-n", node, "--wait=false")
	return err
}

func (c *Client) Shutdown(ctx context.Context, node string) error {
	_, err := c.run(ctx, "shutdown", "-n", node, "--wait=false")
	return err
}

// FormatBytes converts a byte count to a compact human-readable string.
func FormatBytes(b uint64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// --- Processes ---

func (c *Client) GetProcesses(ctx context.Context, node string) ([]ProcessInfo, error) {
	data, err := c.run(ctx, "processes", "-n", node)
	if err != nil {
		return nil, err
	}
	return parseProcessLines(data), nil
}

// parseProcessLines parses `talosctl processes`. Columns are looked up by
// header name: talosctl 1.10+ prints
//
//	NODE PID STATE THREADS CPU-TIME VIRTMEM RESMEM LABEL COMMAND
//
// where LABEL (SELinux) is empty on most nodes.
func parseProcessLines(data []byte) []ProcessInfo {
	var result []ProcessInfo
	for _, row := range parseTable(data) {
		if row["PID"] == "" {
			continue
		}
		result = append(result, ProcessInfo{
			PID:     row["PID"],
			State:   row["STATE"],
			CPUTime: row["CPU-TIME"],
			ResMem:  row["RESMEM"],
			Command: row["COMMAND"],
		})
	}
	return result
}

// --- Containers ---

func normalizeContainerStatus(s string) string {
	switch s {
	case "CONTAINER_RUNNING":
		return "RUNNING"
	case "CONTAINER_STOPPED", "CONTAINER_EXITED":
		return "STOPPED"
	case "SANDBOX_READY":
		return "READY"
	case "SANDBOX_NOTREADY":
		return "NOT_READY"
	default:
		return s
	}
}

func parseContainerLines(data []byte) []ContainerInfo {
	var result []ContainerInfo
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		// NODE NAMESPACE [└─] ID [IMAGE] PID STATUS
		// k8s child containers have a "└─" tree marker before the ID,
		// shifting all remaining fields by one.
		// System containers may have no IMAGE field.
		if len(f) < 5 {
			continue
		}
		idIdx := 2
		if strings.HasPrefix(f[2], "└") {
			idIdx = 3
		}
		if idIdx >= len(f) {
			continue
		}
		id := f[idIdx]
		rest := f[idIdx+1:]
		var img, pid, status string
		switch len(rest) {
		case 0, 1:
			continue
		case 2:
			// No image: PID STATUS
			pid, status = rest[0], rest[1]
		default:
			// IMAGE PID STATUS
			img, pid, status = rest[0], rest[1], rest[2]
		}
		result = append(result, ContainerInfo{
			Namespace: f[1],
			ID:        id,
			Image:     img,
			PID:       pid,
			Status:    normalizeContainerStatus(status),
		})
	}
	return result
}

func (c *Client) GetContainers(ctx context.Context, node string) ([]ContainerInfo, error) {
	// Query system namespace (talos services).
	data1, err1 := c.run(ctx, "containers", "-n", node)
	// Query k8s.io namespace (kubernetes pods).
	// talosctl 1.14 deprecates -k in favour of --namespace cri.
	k8sFlag := "-k"
	if c.ModernCLI() {
		k8sFlag = "--namespace=cri"
	}
	data2, err2 := c.run(ctx, "containers", k8sFlag, "-n", node)

	if err1 != nil && err2 != nil {
		return nil, err1
	}
	var result []ContainerInfo
	if err1 == nil {
		result = append(result, parseContainerLines(data1)...)
	}
	if err2 == nil {
		result = append(result, parseContainerLines(data2)...)
	}
	return result, nil
}

// --- Addresses ---

type addressEnvelope struct {
	Metadata struct {
		ID string `json:"id"`
	} `json:"metadata"`
	Spec struct {
		Address  string `json:"address"`
		LinkName string `json:"linkName"`
		Family   string `json:"family"`
		Scope    string `json:"scope"`
	} `json:"spec"`
}

func (c *Client) GetAddresses(ctx context.Context, node string) ([]AddressInfo, error) {
	data, err := c.run(ctx, "get", "addresses", "-n", node, "-o", "json")
	if err != nil {
		return nil, err
	}
	envs, err := parseJSONStream[addressEnvelope](data)
	if err != nil {
		return nil, err
	}
	var result []AddressInfo
	for _, e := range envs {
		iface := e.Spec.LinkName
		if iface == "" {
			// fall back: parse "eth0/10.0.0.1/24" from ID
			if parts := strings.SplitN(e.Metadata.ID, "/", 2); len(parts) > 0 {
				iface = parts[0]
			}
		}
		result = append(result, AddressInfo{
			Interface: iface,
			Address:   e.Spec.Address,
			Family:    e.Spec.Family,
			Scope:     e.Spec.Scope,
		})
	}
	return result, nil
}

// --- Health ---

// StreamHealth runs `talosctl health` against node (talosctl 1.14 requires
// exactly one node; empty uses the talosconfig context's nodes).
func (c *Client) StreamHealth(ctx context.Context, node string, ch chan<- string) {
	cmdArgs := append(c.baseArgs(), "health")
	if node != "" {
		cmdArgs = append(cmdArgs, "-n", node)
	}
	cmd := exec.CommandContext(ctx, "talosctl", cmdArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ch <- fmt.Sprintf("ERROR: %v", err)
		return
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		ch <- fmt.Sprintf("ERROR: %v", err)
		return
	}
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			select {
			case <-ctx.Done():
				return
			case ch <- sc.Text():
			}
		}
	}()
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			cmd.Process.Kill() //nolint:errcheck
			return
		case ch <- scanner.Text():
		}
	}
	cmd.Wait() //nolint:errcheck
}

func (c *Client) runStreaming(ctx context.Context, ch chan<- string, args ...string) error {
	cmd := exec.CommandContext(ctx, "talosctl", args...)
	outR, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errR, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	scanAll := func(r interface{ Read([]byte) (int, error) }) {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			select {
			case <-ctx.Done():
				return
			case ch <- sc.Text():
			}
		}
	}
	go scanAll(errR)
	scanAll(outR)
	return cmd.Wait()
}

// --- Volume status ---

// MountUsage is one row of the mounts table: used and free space of a mounted filesystem.
type MountUsage struct {
	Filesystem string
	MountedOn  string
	Size       uint64 // bytes
	Used       uint64
	Avail      uint64
}

// GetMounts reads the filesystem usage of a node from the mounts table
// (NODE FILESYSTEM SIZE(GB) USED(GB) AVAILABLE(GB) PERCENT USED MOUNTED ON).
// Sizes are decimal gigabytes with two decimals, so they are approximate.
func (c *Client) GetMounts(ctx context.Context, node string) ([]MountUsage, error) {
	data, err := c.run(ctx, "mounts", "-n", node)
	if err != nil {
		return nil, err
	}
	return ParseMounts(string(data)), nil
}

// ParseMounts parses the mounts table. Rows it cannot read are skipped.
func ParseMounts(out string) []MountUsage {
	var res []MountUsage
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 7 || f[0] == "NODE" {
			continue
		}
		size, e1 := strconv.ParseFloat(f[2], 64)
		used, e2 := strconv.ParseFloat(f[3], 64)
		avail, e3 := strconv.ParseFloat(f[4], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		res = append(res, MountUsage{
			Filesystem: f[1], MountedOn: strings.Join(f[6:], " "),
			Size: uint64(size * 1e9), Used: uint64(used * 1e9), Avail: uint64(avail * 1e9),
		})
	}
	return res
}

// --- Machine config ---

// ApplyConfig applies a full machine config file using talosctl apply-config.
// Mode "auto" picks the least disruptive method (no reboot if not required).
func (c *Client) ApplyConfig(ctx context.Context, node, file string) error {
	args := append(c.baseArgs(),
		"apply-config",
		"-n", node,
		"--file", file,
		"--mode", "auto",
	)
	out, err := exec.CommandContext(ctx, "talosctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PatchMachineConfig applies a strategic merge patch to the machine config.
// The patch file should contain only the fields to change (e.g. machine: section).
func (c *Client) PatchMachineConfig(ctx context.Context, node, file string) error {
	args := append(c.baseArgs(),
		"patch", "machineconfig",
		"-n", node,
		"--patch", "@"+file,
	)
	out, err := exec.CommandContext(ctx, "talosctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
