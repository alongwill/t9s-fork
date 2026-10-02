<div align="center">

<img src="assets/logo.png" alt="t9s" width="350"/>

# t9s — Talos Linux CLI To Manage Your Clusters In Style!

**t9s provides a terminal UI to interact with your [Talos Linux](https://www.talos.dev) clusters.**
The aim of this project is to make it easier to navigate, observe and manage your Talos nodes in the wild. t9s continually watches your cluster for changes and offers subsequent commands to interact with your observed resources.

*Think [k9s](https://k9scli.io), but for Talos.*

<br/>

[![Go Report Card](https://goreportcard.com/badge/github.com/florianspk/t9s)](https://goreportcard.com/report/github.com/florianspk/t9s)
[![GitHub Release](https://img.shields.io/github/v/release/florianspk/t9s)](https://github.com/florianspk/t9s/releases)
[![License](https://img.shields.io/badge/license-Source--Available-blue)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/florianspk/t9s)](go.mod)
[![Downloads](https://img.shields.io/github/downloads/florianspk/t9s/total)](https://github.com/florianspk/t9s/releases)

<br/>

![t9s demo](assets/t9s.gif)

</div>

---

## Features

- 🖥️ **Full-screen responsive layout** — adapts to any terminal size, columns expand with the window
- 📋 **Node list** — Talos version, Kubernetes version, role and live machine stage/readiness (from `MachineStatus`)
- 📡 **Live streaming:** service logs, dynamically discovered node log streams and dmesg with an interactive ▶ cursor
- 🔍 **Per-node resource views** — disks, processes, containers, network addresses
- 🗂️ **Resource browser** — browse every COSI resource on a node by category (Networking, Block, …), greyed when empty, with a YAML pane (`a` on a node)
- 📊 **Metrics** — CPU/RAM stats with delta, auto-refreshed every 5s
- 📄 **Machine config** — read-only YAML viewer
- 🧩 **Extensions** — installed list + Siderolabs catalog browser (requires `crane`)
- ⬆️ **Upgrades** — Talos and Kubernetes, with version pre-fill and a `--drain` toggle (`--preserve` on older talosctl)
- 🩺 **Health** — streaming cluster health checks
- 🔀 **Multi-context** — switch talosconfig context at runtime (`x`)
- ⚡ **Search** — real-time filtering in every list view (`/`)
- 📐 **Wrap mode** — toggle line wrapping for wide content (`w`)
- 🚨 **Version check** — warns when talosctl client/server versions diverge

---

## Installation

t9s is available on Linux and macOS.

### Homebrew (macOS / Linux)

```bash
brew tap florianspk/tap
brew install --cask t9s
```

### Packages & binaries

Download from the [GitHub Releases](https://github.com/florianspk/t9s/releases) page:

- `.tar.gz` archives for **linux** / **darwin** (amd64, arm64)
- `.deb` / `.rpm` / `.apk` packages for Linux

### From source

Requires **Go 1.22+**.

```bash
git clone https://github.com/florianspk/t9s
cd t9s
go build -o t9s ./cmd/main.go
sudo mv t9s /usr/local/bin/
```

---

## Prerequisites

| Requirement | Notes |
|---|---|
| `talosctl` **≥ 1.8** (1.14 recommended) | must be in `$PATH` |
| A valid talosconfig | `~/.talos/config` or `$TALOSCONFIG` |
| `crane` | optional — only for the extension catalog view (`C`) |

### talosctl compatibility

| talosctl | Status |
|----------|--------|
| **1.14.x** | ✅ Target — uses `--drain`, `--progress plain`, `--namespace cri`, `kubeletstatus` |
| **1.8.x – 1.13.x** | ✅ Supported — falls back to `--preserve`, `-k`, `kubeletspec` |
| < 1.8 | ⚠️ `get disks` / `get volumestatus` not available |

Talos 1.14 specifics handled by t9s:

- **Multi-document machine config** — the viewer/editor keeps every document (`HostnameConfig`, `KubeletConfig`, …), not just `v1alpha1`.
- **Kubernetes-less clusters** (experimental in 1.14) — no K8S version is shown, `K` (upgrade-k8s) is disabled, `--drain` defaults off.
- **Single-node commands** — `health` and `upgrade-k8s` are sent to one controlplane node, as 1.14 requires.
- **`kubeletspec` is sensitive** (needs `os:admin`) — t9s reads the non-sensitive `kubeletstatus` first.

> [!NOTE]
> Your talosctl client version should match your cluster version (±1 minor).
> t9s warns you when they diverge.

<details>
<summary><b>talosctl commands used under the hood</b></summary>

| Feature | Command | Available since |
|---------|---------|-----------------|
| Node list | `talosctl get members -o json` | 1.0 |
| Services | `talosctl services` | 1.0 |
| Log stream discovery | `talosctl __completeNoDesc logs --nodes=<node> ''` (falls back to `__complete`) | n/a |
| Logs | `talosctl logs -f` | 1.0 |
| Dmesg | `talosctl dmesg -f` | 1.0 |
| Machine config | `talosctl get machineconfig v1alpha1 -o yaml` (needs `os:admin`) | 1.0 |
| Edit config | `talosctl apply-config --mode auto` | 1.0 |
| Patch config | `talosctl patch machineconfig --patch @file` | 1.2 |
| Addresses | `talosctl get addresses -o json` | 1.2 |
| Resource browser | `talosctl get rd -o json`, `get <type> --namespace <ns> -o json`, `get <type> <id> --namespace <ns> -o yaml` | 1.0 |
| Extensions | `talosctl get extensions -o json` | 1.3 |
| K8s version | `talosctl get kubeletstatus -o json`, fallback `get kubeletspec` | 1.14 / 1.3 |
| Node stage / readiness | `talosctl get machinestatus -o json` | 1.2 |
| Disks | `talosctl get disks -o json` + `get volumestatus -o json` | **1.8** |
| Processes | `talosctl processes` | 1.0 |
| Containers | `talosctl containers` (`--namespace cri` on 1.14, `-k` before) | 1.0 |
| Stats | `talosctl stats` | 1.0 |
| Health | `talosctl health -n <controlplane>` | 1.0 |
| Upgrade Talos | `talosctl upgrade --progress plain --drain=<bool>` (1.14) | 1.0 |
| Upgrade K8s | `talosctl upgrade-k8s -n <controlplane>` | 1.0 |
| Reboot / Shutdown | `talosctl reboot` / `shutdown` | 1.0 |

</details>

---

## Usage

```bash
# Launch with your default talosconfig
t9s

# Specify a talosconfig and context
t9s --talosconfig ~/.talos/config --context my-cluster

# Print version
t9s --version
```

---

## Key Bindings

t9s uses aliases to navigate most Talos resources — hit `?` at any time for the in-app help overlay.

### Global

| Key | Action |
|-----|--------|
| <kbd>?</kbd> | Help overlay |
| <kbd>/</kbd> | Search / filter |
| <kbd>w</kbd> | Toggle wrap mode |
| <kbd>x</kbd> | Switch talos context |
| <kbd>Ctrl</kbd>+<kbd>C</kbd> | Quit |

### Node list

| Key | Action | | Key | Action |
|-----|--------|-|-----|--------|
| <kbd>↑</kbd><kbd>↓</kbd> / <kbd>j</kbd><kbd>k</kbd> | Navigate | | <kbd>t</kbd> | Metrics |
| <kbd>Enter</kbd> / <kbd>s</kbd> | Services | | <kbd>p</kbd> | Processes |
| <kbd>l</kbd> | Log Streams | | <kbd>c</kbd> | Containers |
| <kbd>e</kbd> | Extensions | | <kbd>a</kbd> | Resource browser |
| <kbd>C</kbd> | Extension catalog | | <kbd>i</kbd> | Disks |
| <kbd>m</kbd> | Machine config | | <kbd>d</kbd> | Dmesg |
| <kbd>H</kbd> | Cluster health | | <kbd>R</kbd> / <kbd>S</kbd> | Reboot / Shutdown |
| <kbd>U</kbd> | Upgrade Talos | | <kbd>K</kbd> | Upgrade Kubernetes |
| <kbd>r</kbd> | Refresh | | <kbd>A</kbd> | Network addresses |

> **Changed:** <kbd>a</kbd> now opens the resource browser. The network addresses view moved to <kbd>A</kbd>.

### Resource browser

Opened with <kbd>a</kbd> on a node. Panes open to the right on <kbd>Enter</kbd> (node → categories → types → instances → YAML) and close one at a time on <kbd>Esc</kbd>. Types with no instances on the node are greyed; types that need `os:admin` show `lock`. Keys follow k9s.

| Key | Pane | Action |
|-----|------|--------|
| <kbd>↑</kbd><kbd>↓</kbd> / <kbd>j</kbd><kbd>k</kbd> | all | Move / scroll |
| <kbd>g</kbd> / <kbd>G</kbd>, <kbd>Home</kbd> / <kbd>End</kbd> | all | Top / bottom |
| <kbd>Ctrl</kbd>+<kbd>F</kbd> / <kbd>Ctrl</kbd>+<kbd>B</kbd>, <kbd>PgDn</kbd> / <kbd>PgUp</kbd> | all | Page down / up |
| <kbd>Enter</kbd> | lists | Open |
| <kbd>y</kbd> | instances | Open YAML (same as <kbd>Enter</kbd>) |
| <kbd>/</kbd> | lists | Filter: case-insensitive regex (literal if invalid), `!term` inverts. <kbd>Enter</kbd> applies, <kbd>Esc</kbd> cancels |
| <kbd>/</kbd>, <kbd>n</kbd> / <kbd>N</kbd> | YAML | Search, next / previous match |
| <kbd>w</kbd> | YAML | Toggle wrap |
| <kbd>f</kbd> | YAML | Toggle full screen |
| <kbd>Esc</kbd> / <kbd>q</kbd> | all | Clear the filter or search first, then go back one pane; from the first pane back to the node list |
| <kbd>Ctrl</kbd>+<kbd>R</kbd> | all | Reload the data behind the current pane |

### Log Streams

| Key | Action |
|-----|--------|
| <kbd>↑</kbd><kbd>↓</kbd> / <kbd>j</kbd><kbd>k</kbd> | Navigate |
| <kbd>PgUp</kbd> / <kbd>PgDn</kbd> | Page up / down |
| <kbd>Home</kbd> / <kbd>End</kbd> / <kbd>g</kbd> / <kbd>G</kbd> | Top / bottom |
| <kbd>Enter</kbd> | Open live logs for the selected stream |
| <kbd>/</kbd> | Filter streams |
| <kbd>r</kbd> | Reload streams |
| <kbd>Esc</kbd> / <kbd>q</kbd> | Back |

### Logs / Dmesg / Health

| Key | Action |
|-----|--------|
| <kbd>↑</kbd><kbd>↓</kbd> | Move cursor |
| <kbd>PgUp</kbd> / <kbd>PgDn</kbd> | Half-page scroll |
| <kbd>g</kbd> / <kbd>G</kbd> | Top / bottom |
| <kbd>Esc</kbd> / <kbd>q</kbd> | Back |

### Upgrade

| Key | Action |
|-----|--------|
| type | Enter image or version (pre-filled with current) |
| <kbd>Tab</kbd> | Toggle `--drain` (talosctl 1.14+, default on unless k8s-less) or `--preserve` (older talosctl, default on) |
| <kbd>Enter</kbd> | Confirm |
| <kbd>y</kbd> / <kbd>n</kbd> | Confirm / cancel |
| <kbd>Esc</kbd> | Back (upgrade keeps running in background) |

---

## Views

| View | Key | What it shows |
|------|-----|---------------|
| Nodes | *default* | Members — Talos + K8s version, role, status |
| Services | <kbd>s</kbd> | Service state and health. <kbd>Enter</kbd> or <kbd>l</kbd> opens that service's live logs |
| Log Streams | <kbd>l</kbd> | Node log targets discovered from talosctl completion |
| Logs | <kbd>Enter</kbd> | Live logs for the selected service or log stream |
| Dmesg | <kbd>d</kbd> | Live kernel log stream |
| Machine Config | <kbd>m</kbd> | Machine config YAML |
| Extensions | <kbd>e</kbd> | Installed Talos extensions |
| Ext. Catalog | <kbd>C</kbd> | Available extensions from the Siderolabs registry |
| Metrics | <kbd>t</kbd> | CPU/RAM per container with delta |
| Processes | <kbd>p</kbd> | Running processes sorted by memory |
| Containers | <kbd>c</kbd> | containerd containers (system + k8s namespaces) |
| Resources | <kbd>a</kbd> | Resource browser: categories, types, instances, YAML |
| Addresses | <kbd>A</kbd> | Network interfaces and addresses |
| Disks | <kbd>i</kbd> | Block devices — model, serial, type, size |
| Health | <kbd>H</kbd> | Cluster health checks (streaming) |
| Upgrade Talos | <kbd>U</kbd> | Upgrade with pre-filled installer image |
| Upgrade K8s | <kbd>K</kbd> | Upgrade with pre-filled version |

---

## Architecture

```
t9s/
├── cmd/main.go               # Entry point, CLI flags, bubbletea setup
├── internal/
│   ├── config/config.go      # Talosconfig loader
│   ├── talos/
│   │   ├── types.go          # Data types (Node, Service, DiskInfo…)
│   │   └── client.go         # talosctl subprocess wrappers
│   └── ui/
│       ├── app.go            # bubbletea Model: Init / Update / View
│       ├── styles.go         # Lipgloss palette and styles
│       ├── messages.go       # tea.Msg types
│       ├── keyrouter.go      # Global key dispatch
│       ├── hints.go          # Context-sensitive hint bar
│       └── <view>.go         # One file per view
├── hack/vagrant/             # VirtualBox test cluster
├── assets/                   # Logo and visual assets
├── go.mod
└── LICENSE
```

**Design notes**

- Wraps `talosctl` as a subprocess — no gRPC dependency, authentication is inherited automatically
- Responsive column widths computed from the terminal width at render time
- Backward line-counting guarantees the cursor is always visible in wrap mode
- Goroutine + channel streaming with context cancellation — no goroutine leaks

---

## Development — local test cluster

Spin up a throwaway QEMU cluster to test upgrades safely:

```bash
# Download Talos assets
mkdir -p _out
curl -L https://github.com/siderolabs/talos/releases/download/v1.7.0/vmlinuz-amd64 -o _out/vmlinuz-amd64
curl -L https://github.com/siderolabs/talos/releases/download/v1.7.0/initramfs-amd64.xz -o _out/initramfs-amd64.xz

# Create QEMU cluster (requires root for the CNI bridge)
sudo -E env TALOSCONFIG=~/.talos/t9s-dev.yaml talosctl cluster create \
  --provisioner qemu --name t9s-dev --controlplanes 1 --workers 1 \
  --vmlinuz-path _out/vmlinuz-amd64 --initrd-path _out/initramfs-amd64.xz \
  --talosconfig ~/.talos/t9s-dev.yaml --skip-kubeconfig

t9s --talosconfig ~/.talos/t9s-dev.yaml
```

A VirtualBox alternative lives in [`hack/vagrant/`](hack/vagrant/).

---

## Contributing

Issues and pull requests are welcome! Note that per the license, all modifications must be contributed back to this repository.

---

## License

[Non-Commercial Source-Available License](LICENSE) — free for personal and open-source use.
Commercial use is prohibited. All modifications must be contributed back to this repository.

---

<div align="center">

Made with ❤️ for the Talos community

</div>
