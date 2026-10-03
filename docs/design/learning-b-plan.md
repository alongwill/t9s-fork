# PR B plan: related-resources view, notes from the Talos skill, colour

Branch `feat/related-view`, off `main` (contains browser phases 1–3 and learning PR A).
Learning roadmap: A done; **B = this**; C dropped; D (network tree + HTML diagram) comes after B.

**A second agent is working in parallel** in another worktree on logs, the members IP, and the
containers view (`internal/ui/logs.go`, `nodelist.go`, `containers.go`, `internal/talos/client.go`
`GetNodes`). Do **not** edit those files. You own `internal/ui/styles.go`; keep existing style
names working (rename nothing), only add.

## Goal

Answer "how do these related resources fit together, and how do they differ?" e.g. for `eth0`:
`LinkConfig` (what you wrote) → `LinkSpec` per source in `network-config` (DHCP, platform, config)
→ merged `LinkSpec` (what Talos decided) → `LinkStatus` (what the kernel reports).

## Read first

1. `docs/design/learning-a-plan.md` (what PR A built), `docs/design/resource-browser.md`
   ("Learning roadmap"), `docs/design/skill-md-additions.md` (file map).
2. `docs/design/resource-browser-phase1-plan.md` **Hard rules**: unchanged, except commits are
   not GPG-signed. Never run or name the Talos/Kubernetes/Omni CLIs in a Bash command (a hook
   blocks it, even in `grep` or a path); use Read/Grep/Glob on `~/sources/github.com/siderolabs/talos`.
   Chain checks with `&&`. Do not push.
3. t9s and k9s skills: `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md`,
   `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md`.
4. Code: `internal/talos/deps.go` (`DepGraph`), `internal/ui/browserdescribe.go`,
   `internal/ui/browserkeys.go`, `internal/ui/compare.go` + `diff.go`, `internal/ui/styles.go`.

## Milestone 1: resource notes come from the Talos skill

The notes moved. Source of truth is now
`/Users/andrewlongwill/sources/github.com/alongwill/agent-skills/talos/knowledge/resource-notes.yaml`
(identical to `internal/catalog/resource-notes.yaml` today). Do not edit the skill copy.

1. `hack/sync-resource-notes.sh [path]`: copies that file (default: the path above, or
   `$TALOS_SKILL_DIR/knowledge/resource-notes.yaml`) to `internal/catalog/resource-notes.yaml`,
   prepending `# GENERATED from the talos skill (knowledge/resource-notes.yaml). Edit it there,
   then rerun hack/sync-resource-notes.sh.` Keep the file embedded as now.
2. `hack/sync-resource-notes.sh --check` exits 1 if the copy differs (ignoring the header).
3. README and `skill-md-additions.md`: say where to edit notes.

## Milestone 2: relationship model (`internal/ui/related.go` + `internal/talos/deps.go`)

1. **Pipeline for a type**: walk the `DepGraph` from the selected type up to 3 producer hops and
   3 consumer hops. Result: ordered stages, each `{controllers []string; types []string}`.
   Include `MachineConfigs.config.talos.dev` as the root when it appears, and map it to the
   config kinds of the same category (PR A already links config kinds through it).
2. **Family for a type**: types that share a stem with it. Stem = display type with the trailing
   `Spec|Status|Config|Statuses|Specs|Request|Info` removed (`LinkStatus` → `Link`,
   `LinkConfig` → `Link`, `AddressSpec` → `Address`). Include config kinds with the same stem
   (`LinkConfig`, `LinkAliasConfig` → stem `Link`… only if the stem matches exactly; do not
   include `LinkAliasConfig` in `Link`). Test the stem function with a table.
3. **ID join** across the family: the merged `network` namespace uses plain IDs (`eth0`); the
   `network-config` namespace uses layered IDs. Find the real format in the Talos source
   (Grep `LayeredID` under `pkg/machinery/resources/network/`) and join on the part after the
   layer prefix; record the layer (`configuration`, `dhcp4`, `platform`, `cmdline`, `default`…)
   as a column qualifier. Config documents join on their `name`.

## Milestone 3: related view (`p`)

`p` on a type row, an instance row, or a YAML/describe pane opens the **related view** full
width. (k9s has no `p` in read-only list views; t9s's browser does not use it yet. If it is
taken, pick another free key and report it.) Two sections, `tab` switches focus between them:

**Pipeline** (top): one lipgloss box per type, laid out left→right by stage, joined with
`lipgloss.JoinHorizontal` and arrow connectors `──▶`; controllers shown as small dim labels on
the connector. Box colour by role: Config = magenta/purple, Spec = blue, Status = green, other =
grey; rounded border; the selected type's box gets a thick/bright border. Each box shows the
display type and a count badge (instances on this node, dim `0` if none). Narrow terminals:
stack stages vertically (`JoinVertical`, `▼` connectors) below 120 columns.
`←/→` (and `h/l`) move between boxes; `enter` jumps to that type (palette jump, so `esc` returns
to the related view).

**Family table** (bottom): rows = joined IDs (`eth0`, `eth1`, `lo`), columns = family members in
pipeline order (`LinkConfig`, `LinkSpec@configuration`, `LinkSpec@dhcp4`, …, `LinkSpec`,
`LinkStatus`). Cells: `●` present (coloured by role), `·` absent (dim), `lock` when denied.
Use `github.com/charmbracelet/lipgloss/table` for it (header style, column borders, no row
borders). `enter` on a cell opens its YAML pane. **`space` marks a cell; with two marked, `c`
diffs them** (reuse `diff.go`, normalise both: drop `metadata` except `id`/`namespace`/`type`).
This answers "how does the LinkSpec Talos decided differ from what the kernel reports".

Load: one `List` per family type through the existing loaders/semaphore (counts may already be
cached). Watch is not needed here. Tests: stage walk on a hand-built `DepGraph`, family + join on
fixture IDs including layered ones, two-cell diff, render at 80×24, 120×40, 200×50.

## Milestone 4: `J` jump to writer

k9s `Shift-J` = jump to owner. In an instances or YAML pane, `J` jumps to the type(s) the
selected instance's writer controller (`ResourceMeta.Owner`) reads: if one input type, jump
there; if several, open the related view with that controller's inputs highlighted. Status line
explains when there is no owner. Tests.

## Milestone 5: colour and clarity (browser views only)

Use more of lipgloss, consistently, to make structure readable:

1. **Category accents**: one colour per category (Networking cyan, Block orange, Storage yellow,
   Kubernetes blue, Cluster magenta, Security red, Runtime green, …) in `styles.go`, used for the
   category name in the categories pane, the breadcrumb segment, and the active pane's border.
2. **Pane borders**: `lipgloss.RoundedBorder()`; active pane border in the category accent,
   inactive panes dim grey. Pane title rendered into the top border line.
3. **Badges**: small `Background`-coloured chips with padding 0 1 for: role (`CP` blue / `W` grey)
   in the browser header, source (`grpc` green / `cli` yellow), `watch` (green) / `watch off`
   (dim), counts in the types pane (present = accent, absent = dim), `lock` (red).
4. **Kind colouring** in the types pane: Config / Spec / Status suffixes coloured with the same
   role colours as the related view, so the three layers are recognisable everywhere.
5. Use `lipgloss.AdaptiveColor` for every new colour so light terminals stay readable.
6. Render tests must still pass the width/height budgets; add a test that no rendered line
   exceeds the terminal width with ANSI stripped.

## Milestone 6: docs

README (related view, `p`, `J`, marking/diffing cells, colours legend), `skill-md-additions.md`,
and mark B done in the design doc's "Learning roadmap".

## Done when

1. All milestones committed, `go build ./... && go vet ./... && go test ./...` green,
   `hack/sync-resource-notes.sh --check` passes.
2. A short report to Andrew in this pane: what works, what was not verified live, how to try it
   (`./t9s --source=grpc` → `a` → Networking → LinkStatuses → `p`; mark `LinkSpec` and `LinkStatus`
   for eth0 with `space`, press `c`), and any deviation from this plan with the reason.
