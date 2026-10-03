# Design: t9s resource browser

Status: phases 1-3 implemented, 2026-10-03. Target: Talos v1.14.x (v1.15 dev line tolerated).

## Goal

Help a user learn what a Talos cluster is made of: which nodes exist, what role each has,
and which **config documents** and **COSI resources** exist on each node, grouped by area
(Networking, Block, Storage, …). Movement and key bindings follow k9s.

## Key finding: two kinds of "Talos CRD"

The example workflow drills into `DHCPv4Config`. That is a **machine config document**, not a
COSI resource. Talos has two separate things, and the browser has to show both:

| | Config documents | COSI resources |
|---|---|---|
| Examples | `DHCPv4Config`, `LinkConfig`, `VolumeConfig` | `LinkStatuses.net.talos.dev`, `Disks.block.talos.dev` |
| What it is | What the operator *asked for* | What controllers *produced* (spec/status) |
| Source on a node | The multi-doc YAML stream in `get machineconfig v1alpha1` | `get rd` lists every type; `get <type>` lists instances |
| Catalogue of all kinds | Not on the node. Must be embedded (≈96 kinds) | On the node (`ResourceDefinitions`) |
| "Greyed out" means | Kind not present in this node's config | Type has 0 instances on this node |
| Permission | `os:admin` (machine config is sensitive) | Most `os:reader`; `secrets` and some specs need `os:admin` |

Each category screen shows both, in two sections: **Config** on top, **Resources** below. This
makes the request→result pipeline visible, which is the point of the tool:
`DHCPv4Config` (config) → `OperatorSpecs` (network-config) → `AddressStatuses`, `RouteStatuses`.

## Categories

Use the document-map groups (`talos/tutorials/config-explorer-v2/to-docs-page.py` `GROUP_LABEL`),
which match `pkg/machinery/config/types/<group>/` in the Talos source.

Resources do **not** map cleanly by namespace: block resources live in namespace `runtime`. Map
them by the **type suffix** (`<Kind>.<suffix>.talos.dev`) instead, plus a small override table.

| Category | Config group | Resource type suffixes |
|---|---|---|
| Networking | `network` | `net`, `kubespan` |
| SideroLink | `siderolink` | `siderolink` |
| Kubernetes | `k8s` | `kubernetes`, `k8s` |
| Cluster and discovery | `cluster` | `cluster`, `etcd` |
| Block and volumes | `block` | `block` |
| Storage (LVM/RAID) | `storage` | `storage` |
| CRI and registries | `cri` | `cri` |
| Containers | `container` | `containers` |
| Hypervisor and VM images | `hypervisor` | `hypervisor` |
| Hardware | `hardware` | `hardware` |
| Security | `security` | `security`, `secrets` |
| Extensions | (`ExtensionServiceConfig`, override) | overrides: `ExtensionStatuses`, `ExtensionServiceConfigs`, … |
| Runtime and OS | `runtime` | `runtime`, `v1alpha1`, `files`, `perf`, `config` |
| Other | — | anything unmapped (so new upstream types still appear) |

Suffix counts in Talos `main` today: runtime 37, net 36, kubernetes 31, block 22, storage 13,
secrets 12, hardware 9, containers 8, hypervisor 7, the rest ≤6.

## Screens and navigation

Miller-column layout. Each Enter opens a pane to the right; Esc closes the rightmost pane.
Below 120 columns, only the two rightmost panes are shown.

```
 ctx: prod  node: cp-1 (controlplane)  > Networking > DHCPv4Config            ? help
┌ Categories ─────────┐┌ Networking ─────────────────────┐┌ DHCPv4Config: eth0 ─────────┐
│ Networking     12/41││ CONFIG                          ││ apiVersion: v1alpha1        │
│ Block           6/22││ > DHCPv4Config          1       ││ kind: DHCPv4Config          │
│ Storage         0/13││   LinkConfig            2       ││ name: eth0                  │
│ Kubernetes     18/35││   BondConfig            -       ││ clientIdentifier: mac       │
│ …                   ││ RESOURCES                       ││                             │
│                     ││   AddressStatuses  addr  7      ││                             │
│                     ││   DHCPv6...        -            ││                             │
└─────────────────────┘└─────────────────────────────────┘└─────────────────────────────┘
 <enter> open  <esc> back  <y> yaml  <d> describe  <ctrl-a> all types  </> filter
```

- Rows that exist: normal colour plus instance count. Rows that don't: dim grey, count `-`.
- `12/41` on a category = types present / types known.
- Locked types (sensitive, permission denied): dim with a `🔒`-style marker, not an error.

### Workflow (matches the request)

1. Start → **Nodes** (existing t9s node list; `ROLE` column already comes from
   `get members` `spec.machineType`). Add a `CP`/`W` badge and sort controlplanes first.
2. Arrows/`j`/`k` select a node. Press `a` → **Categories** for that node.
3. Select a category, Enter → **Category** screen (config + resources, greyed where absent).
4. Enter on a type:
   - exactly one instance → **YAML pane** opens on the right;
   - several instances (e.g. `LinkStatuses`) → **Instances** pane (ID, namespace, version,
     phase), Enter on one opens YAML.
5. Esc closes one pane at a time; from Categories, Esc returns to Nodes.

### Key bindings (k9s-aligned)

| Key | Action | k9s equivalent |
|---|---|---|
| `↑↓` / `j k` | move | same |
| `g` / `G`, `ctrl-f` / `ctrl-b` | top / bottom, page | same |
| `enter` / `esc` | drill in / up one level | same |
| `a` (on a node) | open categories. The old addresses view moves to `A` | — (new) |
| `ctrl-a` | **All types** palette: fuzzy list of every type on this node, across categories | `ctrl-a` aliases |
| `:` | command mode: `:addr`, `:dhcpv4config`, `:nodes`, `:net` jump straight there (aliases from `get rd`, plus config kind names, plus category names) | `:pod` |
| `/` | filter the current list (fuzzy) | same |
| `y` | YAML of selection (default in the right pane) | same |
| `d` | toggle **describe** (explain) in the right pane: field docs for the type | `d` describe |
| `w` | toggle line wrap in YAML pane | `w` |
| `f` | YAML pane full screen | `f` |
| `q` | back one level (same as Esc) | `q` |
| `ctrl-r` | refresh current pane | same |
| `?` | help | same |

`ctrl-a` palette columns: `TYPE  ALIASES  CATEGORY  KIND(config|resource)  COUNT`. Typing filters
fuzzily (`dhcp4` matches `DHCPv4Config`). Enter jumps to that type on the current node, with the breadcrumb set as if the user had
navigated there (so Esc goes back to its category, not to the palette).

Fuzzy matching: `github.com/sahilm/fuzzy` against `type`, every alias, and the config kind name.
Rank exact alias matches first (k9s behaviour: `:svc` is exact, not fuzzy).

## Data sources

All through the existing subprocess client (`internal/talos/client.go`), one node per call
(the SKILL notes tabwriter chunking across nodes).

| Data | Call | Cache |
|---|---|---|
| Nodes + role | `get members -o json` (existing `GetNodes`) | per context |
| Resource catalogue | `get rd -o json` → `spec.{type,displayType,aliases,defaultNamespace,sensitivity}` | per node, per Talos version |
| Instances of one type | `get <type> --namespace <defaultNamespace> -o json` | per node+type, 30 s |
| YAML of one instance | `get <type> <id> --namespace <ns> -o yaml` | not cached |
| Config docs present | `get machineconfig v1alpha1 -o yaml` (existing `GetMachineConfig`), split the `spec` string into documents, key by `kind` (+ `name`) | per node, 30 s |
| Config kind catalogue | embedded `catalog.json` (see below) | build time |
| Explain | `explain <type>` (resources). Config docs: the blurb from `catalog.json` | per type |

Verify the `rd` spec field names against the cosi-project/runtime version Talos v1.14 pins
before coding (`pkg/resource/meta/spec`).

### Embedded config-kind catalogue

The node cannot tell you which config kinds *could* exist. Reuse the generator that already knows:
`talos/tutorials/config-explorer-v2/generate-data.py` parses every `registry.Register(...)` call
and records group, first version, and description. Add `--emit-json` to it and commit the output
as `internal/catalog/config-kinds.json` (`go:embed`). Kinds whose first version is newer than the
node's Talos version are hidden, not greyed out.

### "Greyed out" without 200 subprocess calls

`get rd` returns ~200 types. Counting instances of all of them up front is ~200 `talosctl` calls.

- **Lazy, per category.** Count only when a category opens: 10–40 calls, run through a worker
  pool of 8. Roughly 0.5–1 s per category on a LAN. Rows render immediately as "…" and fill in.
- The Categories pane shows `?/41` until a category has been opened once.
- `ctrl-a` triggers a background count of everything, so the palette fills in while the user
  types.
- Put the source behind an interface so a gRPC implementation can replace it later:

```go
type ResourceSource interface {
    Definitions(ctx context.Context, node string) ([]ResourceDef, error)
    List(ctx context.Context, node, ns, typ string) ([]ResourceMeta, error)
    GetYAML(ctx context.Context, node, ns, typ, id string) (string, error)
}
```

A gRPC source (`pkg/machinery/client` → `COSI.List`) would make counting all types one
connection and well under a second. It breaks t9s's "subprocess only" rule, so it is phase 3,
not phase 1.

## Code layout in t9s

| Change | File |
|---|---|
| `ResourceDef`, `ResourceMeta`, `ConfigDoc`, `Category` | `internal/talos/types.go` |
| `GetResourceDefinitions`, `ListResources`, `GetResourceYAML`, `SplitConfigDocs` | `internal/talos/resources.go` (new) |
| Category mapping (suffix table + overrides) | `internal/catalog/categories.go` (new) |
| Config-kind catalogue | `internal/catalog/config-kinds.json` + `catalog.go` (new) |
| States `StateCategories`, `StateCategory`, `StateInstances`, `StateTypePalette` | `internal/ui/app.go` |
| Pane stack (`[]pane`) replacing `prev` for these states, so Esc pops one level | `internal/ui/browser.go` (new) |
| `:` command mode, `ctrl-a` palette | `internal/ui/command.go`, `internal/ui/palette.go` (new) |
| YAML pane: scroll, wrap, syntax colour (`alecthomas/chroma`, optional) | `internal/ui/yamlpane.go` (new) |
| `a` key on node list | `internal/ui/nodelist.go`, `keyrouter.go` |
| Render tests (3-pane width budget, greyed rows, 80-col collapse) | `internal/ui/render_test.go` |
| Fixture tests for `rd` JSON and machine-config splitting | `internal/talos/resources_test.go` |

The existing `goBack()` jumps straight to the node list. The browser needs a real stack: push
on Enter, pop on Esc. Keep it local to the browser states so other views are unchanged.

## Phases

| Phase | Scope | Estimate |
|---|---|---|
| 1 | `rd` + instance list + YAML pane, categories by suffix, lazy counts, `a`/Enter/Esc. Resources only. | 1.5–2 days |
| 2 | Config documents section (embedded catalogue, machine-config split), `ctrl-a` palette, `:` command mode, `/` fuzzy filter, `d` describe | 1.5–2 days |
| 3 | gRPC `ResourceSource`, live watch (`--watch`) of the open type, cross-node compare (same type on all nodes, diff YAML) | 2–3 days |

## Status

| Phase | State |
|---|---|
| 1 | done: `rd` + instances + YAML pane, categories, lazy counts |
| 2 | done: config documents, `ctrl-a` palette, `:` command mode, fuzzy filter, `d` describe |
| 3 | done: gRPC `ResourceSource` (`--source`), count-all, live watch (`W`), cross-node compare (`c`) |

Deferred / not done:

- Not verified against a live cluster or an Omni-managed talosconfig. A failed gRPC dial falls back to the CLI, but a source that dials fine and then fails per call is not retried on the CLI.
- Watch covers resources only, not config documents (they are a snapshot of the machine config).
- Compare diffs against the browser's node only, not pairwise. `d` describe has no field docs (no `explain` in v1.14).
- Compare of config documents always reads the machine config through the CLI client.
- The diff falls back to remove-all/add-all when the differing region exceeds about 4M line pairs.
- Open question 3 (config document to produced resources links) is untouched.

## Learning roadmap

Four PRs turn the browser into a place to learn Talos. Plan for A: `docs/design/learning-a-plan.md`.

| PR | Scope | State |
|---|---|---|
| A | Key feedback in every pane, next-step line, tips, knowledge notes (what / Ubuntu / look here), controller dependency graph, relationship-aware `d` describe | done |
| B | Pipeline / related-resources view: follow a resource through Config → Spec → Status along the graph | planned |
| C | Symptom guides: start from "node has no network" or "disk missing" and walk the relevant resources | planned |
| D | Network tree and an HTML stack diagram (links, addresses, routes, bonds, VLANs) | planned |

Open question 3 below (config documents to the resources they produce) is partly answered by A: describe on a
config kind lists the controllers that read the machine config; B is where the document-to-resource link belongs.

## Open questions

1. ~~Is a gRPC client acceptable in t9s, or must it stay subprocess-only?~~ **Answered (Andrew, phase 3): yes, for the resource browser only.** Every other view keeps the subprocess client, and the subprocess source stays as the fallback (`--source=auto|grpc|cli`, default `auto`).
2. Should the category list for a worker hide controlplane-only types (`etcd`, `controlplane`
   namespace) or show them greyed? Proposal: greyed, with a "controlplane only" hint.
3. Should config documents link to the resources they produce (`DHCPv4Config` →
   `OperatorSpecs`)? Useful for learning, but needs a hand-maintained map.
