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
- 🎓 **Learning aids** — `d` explains a type (what it is, its Ubuntu equivalent, which controllers write and read it), a next-step hint line and tips
- 🗂️ **Resource browser** — browse a node's machine-config documents and COSI resources by category (Networking, Block, …), greyed when absent or empty, with YAML and describe panes, a `ctrl+a` all-types palette and `:` jump commands (`a` on a node)
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
| Resource browser | `talosctl get rd -o json`, `get <type> --namespace <ns> -o json`, `get <type> <id> --namespace <ns> -o yaml`; config documents from `get machineconfig v1alpha1 -o yaml` (needs `os:admin`) | 1.0 |
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

# Resource browser data source: auto (default), grpc or cli
t9s --source=grpc

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
| <kbd>Ctrl</kbd>+<kbd>A</kbd> | All-types palette for the selected node | | <kbd>:</kbd> | Command mode (`:nodes`, `:net`, `:addr`, `:q`, …) |

> **Changed:** <kbd>a</kbd> now opens the resource browser. The network addresses view moved to <kbd>A</kbd>.

### Resource browser

Opened with <kbd>a</kbd> on a node. Panes open to the right on <kbd>Enter</kbd> (node → categories → types → instances → YAML) and close one at a time on <kbd>Esc</kbd>. Keys follow k9s.

The types pane has two sections. **CONFIG** lists the machine-config document kinds (`LinkConfig`, `DHCPv4Config`, … from an embedded catalogue, hidden when newer than the node's Talos version); the count is the number of documents of that kind in the node's config, and kinds the node does not use are greyed. **RESOURCES** lists the COSI resource types; types with no instances are greyed and types that need `os:admin` show `lock`. Reading the machine config needs `os:admin`; without it the CONFIG section shows `requires os:admin`. Enter on a config kind opens that one document (a list of names first when there are several).

| Key | Pane | Action |
|-----|------|--------|
| <kbd>Ctrl</kbd>+<kbd>A</kbd> | all, node list | All-types palette (see below) |
| <kbd>:</kbd> | all, node list | Command mode (see below) |
| <kbd>d</kbd> | types, instances | Describe the selected config kind or resource type; <kbd>d</kbd> again, <kbd>Esc</kbd> or <kbd>q</kbd> closes, <kbd>y</kbd> switches to YAML |
| <kbd>p</kbd> | types, instances, YAML, describe | Related view: the pipeline and family of the selected type (see [Learning Talos with t9s](#learning-talos-with-t9s)) |
| <kbd>J</kbd> | instances, YAML, describe | Jump to what the controller that wrote this instance reads: straight there if it reads one type, else the related view with those inputs highlighted |
| <kbd>↑</kbd><kbd>↓</kbd> / <kbd>j</kbd><kbd>k</kbd> | all | Move / scroll |
| <kbd>g</kbd> / <kbd>G</kbd>, <kbd>Home</kbd> / <kbd>End</kbd> | all | Top / bottom |
| <kbd>Ctrl</kbd>+<kbd>F</kbd> / <kbd>Ctrl</kbd>+<kbd>B</kbd>, <kbd>PgDn</kbd> / <kbd>PgUp</kbd> | all | Page down / up |
| <kbd>Enter</kbd> | lists | Open |
| <kbd>y</kbd> | instances | Open YAML (same as <kbd>Enter</kbd>) |
| <kbd>/</kbd> | lists | Fuzzy filter, live as you type: `adst` finds `AddressStatus`, best matches first, `!term` inverts. <kbd>↑</kbd><kbd>↓</kbd> move while typing, <kbd>Enter</kbd> keeps it, <kbd>Esc</kbd> cancels |
| <kbd>/</kbd>, <kbd>n</kbd> / <kbd>N</kbd> | YAML | Search (regex), next / previous match |
| <kbd>w</kbd> | YAML | Toggle wrap (YAML is syntax-coloured: keys, strings, numbers, booleans, comments) |
| <kbd>f</kbd> | YAML | Toggle full screen |
| <kbd>W</kbd> | types, instances, YAML | Live watch on/off (gRPC source only). On a types row it opens the instance list first |
| <kbd>c</kbd> | types, instances, YAML | Compare the selected resource or config document on every node (see below). On a type with several instances it opens the instance list first |
| <kbd>Esc</kbd> / <kbd>q</kbd> | all | Clear the filter or search first, then go back one pane; from the first pane back to the node list |
| <kbd>Ctrl</kbd>+<kbd>R</kbd> | all | Reload the data behind the current pane |

Describe is the learning page of the browser, see [Learning Talos with t9s](#learning-talos-with-t9s).

**Data source (`--source=auto|grpc|cli`).** The resource browser can read COSI resources over Talos' gRPC API (`pkg/machinery/client`, same talosconfig and context) or through subprocesses of the Talos CLI. `auto` (default) dials gRPC in the background with a 5 s timeout and falls back to the CLI, noting `gRPC unavailable (<reason>), using CLI` in the status line, so the browser still works when the talosconfig auth mode is not supported by the library. The header shows `src: grpc` or `src: cli`. Every other view always uses the CLI.

What needs gRPC: counting every type up front (categories show real `present/known` at once; the CLI source counts lazily per category) and the live watch. Compare works with both sources.

**Live watch (<kbd>W</kbd>).** On an instances pane (and under its YAML pane) the browser watches that type on that node: rows appear, change and disappear as it happens, changed rows flash for about a second, the cursor stays on its ID, and an open YAML reloads when its resource is updated. The header shows `watch` or `watch off`. <kbd>W</kbd> toggles it; with the CLI source it says `watch needs the gRPC source`. <kbd>Ctrl</kbd>+<kbd>R</kbd> restarts it; leaving the pane, changing node or quitting cancels it.

**Cross-node compare (<kbd>c</kbd>).** Opens one row per cluster node (`NODE ROLE PRESENT VERSION SAME?`) for the selected resource, or config document (kind + name), loaded from every node concurrently. `metadata.version`, `created`, `updated` and the `node` field are ignored; `SAME?` compares each node with the node the browser was opened on (`*`). <kbd>Enter</kbd> on a row opens a unified diff (`-` browser node, `+` other node); <kbd>Esc</kbd> / <kbd>q</kbd> back out one level, <kbd>Ctrl</kbd>+<kbd>R</kbd> refetches. It is <kbd>c</kbd>, not <kbd>x</kbd>, because <kbd>x</kbd> is t9s' global context switcher. Config documents need `os:admin` on every node.

**All-types palette (<kbd>Ctrl</kbd>+<kbd>A</kbd>).** One full-width list of every config kind and resource type on the node (`NAME ALIASES CATEGORY KIND COUNT`), with the filter already open: type to narrow it, an exact alias (`addr`) ranks first. Counts of types not yet counted fill in as they arrive. <kbd>Enter</kbd> jumps to the type with the usual panes behind it, so <kbd>Esc</kbd> lands in its category; <kbd>Esc</kbd> clears the filter, then closes the palette. On the node list it opens the browser for the selected node first.

**Command mode (<kbd>:</kbd>).** Prompt in the status line, as in k9s: <kbd>Enter</kbd> / <kbd>Ctrl</kbd>+<kbd>E</kbd> run, <kbd>Esc</kbd> cancels, <kbd>Ctrl</kbd>+<kbd>U</kbd> / <kbd>Ctrl</kbd>+<kbd>W</kbd> clear, <kbd>Tab</kbd> / <kbd>→</kbd> accept the dim suggestion, <kbd>↑</kbd><kbd>↓</kbd> cycle suggestions (or history on an empty prompt).

| Command | Action |
|---------|--------|
| `:nodes`, `:no` | Node list |
| `:net`, `:block`, … | That category (key or label prefix) on the current node |
| `:addr`, `:dhcpv4config`, `:addressstatuses.net.talos.dev` | Jump to that alias, display type, full type or config kind |
| `:a`, `:alias`, `:aliases` | All-types palette |
| `:tips on`, `:tips off` | Show or silence the tips in the status line (this session) |
| `:q`, `:q!`, `:quit` | Quit |
| `:?`, `:h`, `:help` | Help |

### Learning Talos with t9s

The browser tries to answer four questions without leaving t9s: what is this resource, what is it on Ubuntu, what feeds it and what does it feed, and which key do I press next.

- **Describe (<kbd>d</kbd>)** on a type or instance shows these sections, leaving out any that have no data:
  `WHAT` (one sentence), `ON UBUNTU` (the closest Ubuntu tool or file), `LOOK HERE` (when this is the resource to check), `WRITTEN BY` (the controller that owns the selected instance), `FED BY` (the controllers that write this type, with the types they read) and `FEEDS` (the controllers that read it, with the types they write). On a config kind it shows `WHAT`, `ON UBUNTU`, `SINCE` and the controllers that read the machine config. In the trees, `◀` marks what a controller reads and `▶` what it writes; the type name is highlighted and its group suffix (`.kubespan.talos.dev`) is grey. Rows that name a resource type are selectable with <kbd>j</kbd> / <kbd>k</kbd>; <kbd>Enter</kbd> jumps there, and <kbd>Esc</kbd> behaves as after a palette jump. Types the node does not have are dim and cannot be selected. <kbd>Ctrl</kbd>+<kbd>R</kbd> reloads the relationships.
- **Related view (<kbd>p</kbd>)** answers "how do these related resources fit together, and how do they differ?". It has two sections; <kbd>Tab</kbd> switches the focus.
  - **Pipeline** (top): one box per type along the controller graph, up to three hops to each side, with the controllers as small labels on the arrows: `MachineConfig ──▶ LinkSpec ──▶ LinkStatus`. Box colour is the role: Config purple, Spec blue, Status green, anything else grey; the selected box has a thick border and each box shows its instance count on this node. At 120 columns or more the stages run left to right (`──▶`); below that they stack (`▼`) as one-line chips. Move with <kbd>←</kbd><kbd>→</kbd><kbd>↑</kbd><kbd>↓</kbd> (or <kbd>h</kbd><kbd>j</kbd><kbd>k</kbd><kbd>l</kbd>), <kbd>Enter</kbd> opens that type's instances on top, <kbd>Esc</kbd> comes back.
  - **Family** (bottom): the types that share the stem of the selected one (`LinkStatus`, `LinkSpec`, `LinkConfig` → `Link`; `LinkAliasConfig` is a different family). One row per ID (`eth0`, `eth1`, `lo`), one column per member in pipeline order: config documents (joined on `name`), each layer of the unmerged `LinkSpec` in `network-config` (`@configuration`, `@operator`, `@platform`, `@cmdline`, `@default`), the merged `LinkSpec`, then `LinkStatus`. `●` is present, `·` absent, `lock` needs `os:admin`. <kbd>Enter</kbd> opens a cell's YAML; <kbd>Space</kbd> marks a cell (two at most) and <kbd>c</kbd> diffs the two marked cells side by side with metadata ignored. When a config kind and a resource type share a name (`VolumeConfig`), the columns read `VolumeConfig@document` and `VolumeConfig@resource`. Mark `LinkSpec` and `LinkStatus` of `eth0` to see what Talos decided versus what the kernel reports. <kbd>Esc</kbd> clears the marks first.
- **Jump to the writer (<kbd>J</kbd>).** Every resource has an owner controller (`WRITTEN BY` in describe). <kbd>J</kbd> on an instance goes to the types that controller reads. An instance with no owner (created through the API or by apply-config) says so.
- **Colour legend.** Each category has an accent (Networking cyan, Block and volumes orange, Storage yellow, Kubernetes blue, Cluster magenta, Security red, Runtime green, …) used for the category name, the breadcrumb and the active pane's border; inactive panes are grey. In the types pane the `Config` / `Spec` / `Status` suffix of a name uses the role colours above, so the three layers are recognisable everywhere. Chips in the header: `CP` (control plane, blue) or `W` (worker, grey), source `grpc` (green) or `cli` (yellow), `watch` (green) or `watch off` (dim); `lock` is red.
- **Relationships need the gRPC source** (`--source=grpc`, the default `auto` uses it when it can dial). With the CLI source t9s parses the CLI's graphviz output instead, and says `relationships need the gRPC source (--source=grpc)` if that fails.
- **Next-step line.** One dim line under the panes describes the selected row and the keys worth pressing next, for example `↵ 3 instances · d what is this · c compare (on an instance) · W watch (on an instance)`, or `not on this node · d what is this` on a greyed row. It is hidden below 20 terminal rows.
- **Keys explain themselves.** A browser key pressed where it does not work says where it does, naming the selected type: `W works on an instance list: press Enter on LinkStatuses first`.
- **Tips.** A short tip about the browser or a Talos concept shows in the status line when the browser opens and when a category opens, if nothing else is shown. `:tips off` silences them for the session.

The notes are written from the Talos source and skill references. The source of truth is `knowledge/resource-notes.yaml` in the Talos skill (`agent-skills/talos`); `internal/catalog/resource-notes.yaml` is a generated copy. Edit the skill file, then run `hack/sync-resource-notes.sh` (`--check` fails when the copy is stale). Types without a note still show their definition fields and relationships.

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

- Wraps the Talos CLI as a subprocess, authentication is inherited automatically. The one exception is the resource browser, which can use Talos' gRPC client (`--source`) and falls back to the subprocess
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
