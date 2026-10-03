# PR A plan: learning hints, knowledge notes, relationship-aware describe

Branch `feat/learning-hints`, off `main` (contains resource browser phases 1–3).
First of four learning PRs (A hints + knowledge, B pipeline/related view, C symptom guides,
D network tree + HTML diagram). Only A is in scope here.

## Goal

A user who opens a resource should learn **what it is**, **what it is on Ubuntu**, **what feeds
it and what it feeds**, and **which keys work here and what to press next**, without leaving t9s.

Two problems this fixes, seen during phase 3 testing:
- `W` and `c` silently did nothing on the types pane: they only work one level deeper.
- Nothing tells a user which resource to look at, or how it relates to the config they wrote.

## Read first

1. `docs/design/resource-browser.md` and `docs/design/skill-md-additions.md` (current file map).
2. `docs/design/resource-browser-phase1-plan.md` **Hard rules**: they apply unchanged, except
   commits are not GPG-signed (`commit.gpgsign=false`), so commit normally.
3. `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md` (t9s conventions)
   and `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md` (k9s keymap).
4. Code: `internal/ui/browserkeys.go` (`browserActions`), `internal/ui/browserdescribe.go`,
   `internal/ui/hints.go`, `internal/talos/source.go` + `grpcsource.go`, `internal/catalog/`.

Reminders: never run the Talos/Kubernetes/Omni CLIs and never put their names in a Bash command
(a hook blocks it, even in `grep` or a path). Use Read/Grep/Glob on
`~/sources/github.com/siderolabs/talos`. Chain checks with `&&`, never `;`. Do not push.

## Facts already checked (Talos source)

- `api/inspect/inspect.proto`: `InspectService.ControllerRuntimeDependencies(Empty)` returns
  `messages[].edges[]` of `{controller_name, edge_type, resource_namespace, resource_type, resource_id}`.
  `edge_type`: `OUTPUT_EXCLUSIVE=0`, `INPUT_STRONG=1`, `INPUT_WEAK=2`, `OUTPUT_SHARED=3`,
  `INPUT_DESTROY_READY=4`. `resource_id` is empty unless the controller watches one ID.
- Go client: `c.Inspect.ControllerRuntimeDependencies(ctx)` (`pkg/machinery/client/inspect.go:22`),
  per node via `client.WithNode(ctx, node)`.
- Permission: allowed for `os:reader`, `os:operator`, `os:admin`
  (`internal/app/machined/pkg/system/services/machined.go:42`).
- `resource_type` is the full type string (`LinkStatuses.net.talos.dev`), same as `ResourceDef.Type`.
- Each resource's `metadata.owner` is the controller that wrote it (e.g. `k8s.KubeletSpecController`).
- `lipgloss` v1.1 (already a dependency) has the `github.com/charmbracelet/lipgloss/tree` package.
  Use it; add no new UI dependency (no glamour in this PR).
- Ubuntu equivalents for all 96 config kinds exist in the `ubuntu` field of
  `/Users/andrewlongwill/sources/github.com/alongwill/agent-skills/talos/tutorials/config-explorer-v2/data.js`.

## Milestone 1: key feedback everywhere (fixes the dead-key problem)

1. In `browserkeys.go`, build a table of every browser key with the panes it works in and a
   one-line "where" text, e.g. `W` → "on an instance list (Enter on a type first), gRPC source".
2. When a key is pressed in a pane where it is not bound, but it is a browser key elsewhere, show
   it in the status line instead of doing nothing: `W works on an instance list: press Enter on
   LinkStatuses first`. Name the selected type when there is one.
3. `c` / `W` on a types-pane row with several instances: open its instance list (as Enter would)
   and set the status line to `pick one, then press c` / `then press W`. One key press less, and
   the user learns the path.
4. Tests: every key in the table either acts or sets a non-empty status message in every pane kind.

## Milestone 2: context hint line and tips

1. **Next-step line**: one dim line directly under the active list pane, built from the selected
   row, e.g. types pane on a 3-instance type:
   `↵ 3 instances · d what is this · c compare (on an instance) · W watch (on an instance)`.
   On a greyed (absent) row: `not on this node · d what is this`. It replaces nothing; account for
   it in the height budget. Hidden below 20 rows of terminal height.
2. **Tips**: `internal/ui/tips.go` holds an embedded list of ~20 short tips covering browser
   features and Talos concepts (e.g. `Config is what you asked for; Specs are what Talos decided;
   Statuses are what the kernel reports`, `ctrl+a lists every type on this node`,
   `:net jumps straight to Networking`). Show one in the status line when the browser opens and
   each time a category opens, when no other status message is set. Rotate in order, starting
   from a random index. `:tips off` / `:tips on` toggles them (session only).
3. Help overlay: add a short "Learning" section listing `d`, the next-step line and `:tips`.
4. Tests: next-step text per pane kind and row state; tip rotation; `:tips off` silences tips;
   render budget at 80×24 and 120×40 with the extra line.

## Milestone 3: knowledge notes

1. Extend `hack/gen-config-kinds.py` to copy the `ubuntu` field into `config-kinds.json`, rerun it
   against the `data.js` path above, and add `Ubuntu` to `catalog.ConfigKind`.
2. New `internal/catalog/resource-notes.yaml` (embedded), keyed by **display type**:
   ```yaml
   LinkStatus:
     what: "The live state of a network link as the kernel reports it: up/down, MTU, MAC, driver."
     ubuntu: "`ip -d link show`, `/sys/class/net/<if>/`, `ethtool <if>`"
     lookWhen: ["link is down or has the wrong MTU", "a bond or VLAN member is missing"]
   ```
   Seed about 40 types: the network ones (Link/Address/Route Spec+Status, OperatorSpec,
   ResolverStatus, HostnameStatus, NodeAddress, TimeServerStatus, KubeSpanPeerStatus, …), block
   (Disk, DiscoveredVolume, VolumeConfig, VolumeStatus, MountStatus, SystemDisk), runtime
   (MachineStatus, Service, ExtensionStatus, KernelParamStatus), cluster (Member, Affiliate,
   Identity), etcd (Member, Spec), k8s (KubeletSpec, StaticPodStatus, NodenameStatus).
   Source the text from the Talos skill references under
   `/Users/andrewlongwill/sources/github.com/alongwill/agent-skills/talos/references/` and the
   Talos source (resource doc comments in `pkg/machinery/resources/`). Do not invent behaviour:
   if unsure, keep `what` to what the doc comment says and omit `ubuntu`.
   Add a header comment saying where the text came from. Keep each field to one sentence.
3. `catalog.NoteFor(displayType)`; a test that every key in the YAML is a display type that exists
   in the phase 1 `rd` fixture or the real Talos source (Grep `pkg/machinery/resources/` for
   `DisplayType`/type names; hard-code the checked list in the test).

## Milestone 4: dependency graph

1. Add `Dependencies(ctx, node) (DepGraph, error)` to `ResourceSource`. gRPC: call
   `Inspect.ControllerRuntimeDependencies`. CLI: check (Read/Grep, Talos `cmd/` tree, search
   `inspect` and `dependencies`) what the CLI's `inspect dependencies` command prints. If it is
   graphviz DOT with stable node labels, parse it; otherwise return `ErrNeedsGRPC` and say which
   in the report.
2. `DepGraph` in `internal/talos/deps.go`:
   ```go
   type DepEdge struct { Controller, Namespace, Type, ID string; Output bool; Weak bool }
   type DepGraph struct { Edges []DepEdge }
   func (g DepGraph) Producers(typ string) []string          // controllers with an output edge on typ
   func (g DepGraph) Inputs(controller string) []DepEdge     // input edges of a controller
   func (g DepGraph) Consumers(typ string) []string          // controllers with an input edge on typ
   func (g DepGraph) Outputs(controller string) []DepEdge
   ```
   Cache per node; `ctrl+r` on the describe pane reloads it.
3. Add `Owner` to `talos.ResourceMeta` (both sources: `metadata.owner`).
4. Tests: mapping from the proto response (build one by hand), the four query functions, DOT
   parsing if implemented.

## Milestone 5: richer describe pane

Rework `browserdescribe.go` so `d` shows these sections, omitting any with no data:

```
LinkStatus  (LinkStatuses.net.talos.dev · ns network · aliases link, links)
WHAT        The live state of a network link as the kernel reports it …
ON UBUNTU   ip -d link show, /sys/class/net/<if>/, ethtool <if>
LOOK HERE   link is down or has the wrong MTU · a bond or VLAN member is missing
WRITTEN BY  network.LinkStatusController                      (owner of eth0, when an instance is selected)
FED BY      network.LinkStatusController
            ├─ reads LinkSpecs.net.talos.dev
            └─ reads …
FEEDS       network.AddressStatusController
            └─ writes AddressStatuses.net.talos.dev
```

- Use `lipgloss/tree` for FED BY / FEEDS, one level of controllers and their inputs/outputs.
  Collapse more than 6 children into `… N more`.
- Config kinds: WHAT (`desc`), ON UBUNTU (`ubuntu`), SINCE, and FEEDS from the graph via the
  `MachineConfigs.config.talos.dev` consumers.
- Rows that name a resource type are **selectable** (j/k move between them when the describe pane
  is focused); `enter` jumps there using the palette's jump function, so `esc` behaves as for a
  palette jump. Types that do not exist on the node are dim and not selectable.
- When the graph is unavailable (CLI without DOT parsing), show one dim line:
  `relationships need the gRPC source (--source=grpc)`.
- Tests: section rendering with and without notes/graph, tree collapse, jump from a FED BY row,
  80-column wrapping.

## Milestone 6: docs

- README: a short "Learning Talos with t9s" section (describe sections, next-step line, tips).
- `docs/design/skill-md-additions.md`: new files, the knowledge-notes format, and how to regenerate
  `config-kinds.json`.
- `docs/design/resource-browser.md`: add a "Learning roadmap" section listing PRs A–D (B: pipeline
  / related-resources view, C: symptom guides, D: network tree + HTML stack diagram) with A done.

## Done when

1. All milestones committed, `go build ./... && go vet ./... && go test ./...` green.
2. A short report to Andrew in this pane: what works, what was not verified live, how to try it
   (`./t9s --source=grpc`, `a` on a node, Networking, `d` on LinkStatuses, `enter` on a FED BY row),
   which resource types got notes, and any deviation from this plan with the reason.
