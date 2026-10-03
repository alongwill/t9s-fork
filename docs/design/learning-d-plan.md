# PR D plan: network view (TUI tree + HTML stack diagram)

Branch `feat/network-view`, off `main` (contains browser phases 1–3, learning PRs A and B, and
the logs / members / containers work). Learning roadmap: A, B done; C dropped; **D = this**.
The graphical disk view (`docs/design/disk-view.md`) comes after this PR: do not start it.

## Goal

For one node, show how the network is built, from the physical NIC up:
NIC → logical links (bond, bridge, VLAN, …) → addresses → routes, plus the config documents that
asked for each piece and the Talos resources that represent it. Two outputs from one model:

1. **TUI network view**: a `lipgloss/tree` the user can move through and drill into.
2. **HTML stack diagram**: a self-contained page t9s writes and opens in the browser, for the
   whole-node picture a terminal cannot lay out.

## Read first

1. `docs/design/learning-b-plan.md` (related view, colours, `J`), `docs/design/learning-a-plan.md`,
   `docs/design/resource-browser.md` ("Learning roadmap"), `docs/design/skill-md-additions.md`.
2. `docs/design/resource-browser-phase1-plan.md` **Hard rules**: unchanged, except commits are not
   GPG-signed. Never run or name the Talos/Kubernetes/Omni CLIs in a Bash command (a hook blocks
   it, even in `grep` or a path); use Read/Grep/Glob on `~/sources/github.com/siderolabs/talos`.
   Chain checks with `&&`. Do not push.
3. t9s and k9s skills: `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md`,
   `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md`.
4. Talos networking knowledge: `/Users/andrewlongwill/sources/github.com/alongwill/agent-skills/talos/references/networking/`
   (`network-configuration.md`, `logical-interfaces.md`, `virtual-ip.md`, `kubespan.md`) for
   what each resource means and how config layers merge.
5. Prior art for the HTML side: `/Users/andrewlongwill/sources/github.com/alongwill/agent-skills/talos/tutorials/network-doctor/web/`
   (Cytoscape topology). Reuse its look and libraries where sensible; do not edit that repo.
6. Code: `internal/ui/related*.go`, `internal/ui/browsercolor.go`, `internal/ui/styles.go`,
   `internal/talos/source.go`, `internal/talos/deps.go`, `internal/catalog/`.

## Facts already checked (Talos `pkg/machinery/resources/network/`)

- `LinkStatus` yaml keys: `alias altNames index type linkIndex flags hardwareAddr permanentAddr
  mtu masterIndex operationalState kind slaveKind busPath pciID driver … linkState speedMbit duplex
  vlan macvlan vxlan bridgeMaster bondMaster vrfMaster wireguard veth`. Parent relationships:
  `masterIndex` (bond/bridge/VRF member → master) and `linkIndex` (VLAN/macvlan/vxlan → lower link),
  both by kernel `index`. Physical NIC = has `busPath`/`pciID`/`driver` and empty `kind`.
- `AddressStatus`: `address local broadcast linkIndex linkName family scope flags priority`.
- `RouteStatus`: `family dst src gateway outLinkIndex outLinkName table priority scope type flags
  protocol mtu nextHops`.
- `network-config` IDs are `LayeredID` = `<layer>/<id>` (`network.go:82`); the layer tells you
  where a spec came from (`configuration`, `dhcp4`, `dhcp6`, `platform`, `cmdline`, `default`).
  PR B's family join already parses this; reuse it.

## Milestone 1: network model (`internal/netmodel/`, pure Go, no UI)

1. `Build(in Inputs) Model` from already-fetched resources: `LinkStatuses`, `AddressStatuses`,
   `RouteStatuses`, `LinkSpecs`/`AddressSpecs`/`RouteSpecs` in `network-config` (for the source
   layer of each), `OperatorSpecs` (DHCP/VIP operators per link), `HostnameStatus`,
   `ResolverStatus`, `NodeAddresses`, KubeSpan link/peer statuses if present, and the machine
   config documents of the networking category (from PR 2's `SplitConfigDocs`).
2. `Model`: links as a forest (roots = physical NICs and links with no lower/master; children =
   links whose `masterIndex`/`linkIndex` points at them), each link with its addresses (with
   source layer: static config, DHCP, platform…), routes out of it, operators on it (DHCPv4,
   DHCPv6, VIP), and the config documents that name it (`LinkConfig`/`BondConfig`/`VLANConfig`/
   `BridgeConfig`/`DHCPv4Config`/`Layer2VIPConfig`… matched by `name`, and for VLANs by parent +
   VLAN ID; read the config types under `pkg/machinery/config/types/network/` to get the
   matching right). Node-wide: hostname, resolvers, default routes, routes with no link.
3. Use the browser's `ResourceSource` for fetching (gRPC or CLI), one node, semaphore 8.
4. Tests with hand-written fixtures for: single NIC + DHCP; bond of two NICs + VLAN on the bond +
   static address + VIP; bridge with a member; a link that exists only in config (not in status,
   e.g. a typo'd name), which must be shown as a **warning** node; a route whose `outLinkName` has
   no link.

## Milestone 2: TUI network view

- Entry: `N` on the node list (check it is free in `nodelist.go`; if not, pick one and report),
  `:netview` / `:nv` in command mode, and `n` inside the browser's Networking category (check
  `n` is free there; it is find-next only in text panes).
- Layout: left = tree (`lipgloss/tree`), right = detail pane for the selected tree node.

```
 Network  node: cp-1 [CP]  hostname cp-1 · dns 1.1.1.1, 8.8.8.8 · default via 10.0.0.1 (eth0)
 ╭ links ───────────────────────────────────────────╮╭ bond0 ──────────────────────────────╮
 │ ● eth0   up 1000Mb/s  e1000e  52:54:00:aa:bb:01  ││ kind     bond (802.3ad)             │
 │ ● eth1   up 1000Mb/s  e1000e  52:54:00:aa:bb:02  ││ mtu      1500                       │
 │ └─▶ ● bond0  bond  up  mtu 1500                  ││ members  eth0, eth1                 │
 │     ├─ 10.0.0.5/24        static  BondConfig/bond0││ config   BondConfig "bond0"         │
 │     ├─ 10.0.0.10/32  VIP  Layer2VIPConfig/10.0.0.10││ specs    LinkSpec@configuration     │
 │     ├─ ⇢ default via 10.0.0.1                    ││ status   LinkStatus bond0  up       │
 │     └─▶ ● bond0.100  vlan 100  up                ││                                     │
 │         └─ 192.168.100.5/24  dhcp4               ││ ↵ YAML  d describe  p related       │
 │ ⚠ eth9  in LinkConfig, no such link on this node ││                                     │
 ╰──────────────────────────────────────────────────╯╰─────────────────────────────────────╯
```

- Colours (reuse `browsercolor.go` role colours and the Networking accent): link state dot green
  up / red down / grey unknown; physical NICs bold; logical links by kind (bond, bridge, vlan,
  wireguard, kubespan); addresses tagged by source layer with a coloured badge (`static`, `dhcp4`,
  `dhcp6`, `platform`, `VIP`); config-only/missing items yellow `⚠`.
- Keys: `j/k` move, `h/l` or `←/→` collapse/expand, `enter` YAML of the selected item's status
  resource (browser jump), `d` describe (PR A notes), `p` related view for its type (PR B), `c`
  show which config document created it (jump to that document), `o` open the HTML diagram
  (milestone 3), `ctrl+r` refresh, `esc`/`q` back. Every key gives status-line feedback when it
  does not apply (PR A pattern).
- Narrow terminals (< 100 cols): detail pane moves below the tree.
- Tests: tree rendering for each fixture, warning node, key handling, render budgets at 80×24,
  120×40, 200×50, ANSI-stripped width check.

## Milestone 3: HTML stack diagram

- `o` in the network view writes a **single self-contained HTML file** to
  `$TMPDIR/t9s-network-<hostname>-<timestamp>.html` and opens it (`open` on macOS, `xdg-open` on
  Linux; print the path in the status line if opening fails). Template embedded with `go:embed`
  under `internal/netmodel/web/`; the model is injected as JSON (`<script type="application/json">`,
  HTML-escaped).
- Diagram: horizontal **layers**, bottom to top: physical NICs → logical links → addresses →
  routes / services (VIP, KubeSpan, kubelet node IP). A left column holds the **config
  documents**; dashed edges go from each document to what it created. Solid edges for link
  hierarchy (member → bond, lower → VLAN), address → link, route → link. Use Cytoscape.js with the
  dagre layout from cdnjs (same as Network Doctor), rank direction bottom-to-top.
- Node colours match the TUI (role and kind colours); warnings yellow; down links red.
- Click a node: side panel with its fields, its source layer, the config document that made it,
  and the Talos resource `type/id` (so the reader can find it in t9s), plus the PR A note (`what`,
  `ubuntu`) when there is one. Include a legend and a short "How to read this" box (config →
  spec → status layers; where this lives on Ubuntu: `/etc/netplan`, `ip link`, `ip addr`,
  `ip route`).
- Works offline except for the CDN scripts; must render with no console errors. Light and dark via
  `prefers-color-scheme`.
- Tests: JSON injection escaping (a `</script>` in a value), template renders with each fixture
  (golden file of the embedded JSON, not the full HTML).
- Andrew will check the page in a browser; say in the report which fixture file to open
  (add a `go test -run TestWriteExampleHTML -update` style helper or a `hack/` script that writes
  the HTML for a fixture without a cluster).

## Milestone 4: notes and docs

- Add notes for any network types the view uses that have none yet (e.g. `OperatorSpec` already
  exists; check `LinkSpec`, `AddressSpec`, `RouteSpec`, VIP/KubeSpan types). **Edit the skill copy**
  `/Users/andrewlongwill/sources/github.com/alongwill/agent-skills/talos/knowledge/resource-notes.yaml`
  (only that one file in that repo; do not commit there, Andrew will), then run
  `hack/sync-resource-notes.sh` and commit the t9s copy. Same sourcing rules as PR A: from the
  Talos source and the skill references, no invented behaviour.
- README (network view, keys, HTML diagram), `skill-md-additions.md`, and mark D done in the
  design doc's "Learning roadmap", with the disk view listed as next.

## Done when

1. All milestones committed, `go build ./... && go vet ./... && go test ./...` green,
   `hack/sync-resource-notes.sh --check` passes.
2. A short report to Andrew in this pane: what works, what was not verified live, how to try it
   (`./t9s --source=grpc` → `N` on a node → move to a bond/VLAN → `o`), the path of an example
   HTML file built from a fixture, which notes were added to the skill file, and any deviation
   from this plan with the reason.
