# Phase 2 implementation plan: resource browser

You are implementing **phase 2** of `docs/design/resource-browser.md` in this worktree
(branch `feat/resource-browser-phase2`, off `main`, which already contains phase 1).
Read these first, in order:

1. `docs/design/resource-browser.md`: the design. Phase 2 row of the Phases table.
2. `docs/design/resource-browser-phase1-plan.md`: how phase 1 was specified. Its **Hard rules**
   section applies to you unchanged, with one exception: commits are no longer GPG-signed
   (`commit.gpgsign=false` is set on the repo), so commit normally.
3. `docs/design/skill-md-additions.md`: the phase 1 file map.
4. `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md`: t9s conventions.
5. `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md`: k9s keymap,
   especially §5 (default keys) and §6 (`:` and `/` prompts). Match k9s wherever the concept exists.
6. Phase 1 code: `internal/ui/browser*.go`, `internal/ui/fuzzy.go`, `internal/catalog/`,
   `internal/talos/resources.go`. Reuse it; do not rewrite it.

Reminders from the hard rules: never run the Talos/Kubernetes/Omni CLIs, and never put their
names in a Bash command (a hook blocks it, even in `grep` or a path such as
`cmd/<cli>/cmd/talos/`). Use the Read/Grep/Glob tools for the Talos source at
`~/sources/github.com/siderolabs/talos`. Chain verification with `&&`, never `;` (phase 1
committed once with failing tests because of `;`). Do not push.

## Already done (do not redo)

- The `/` live fuzzy filter in list panes (phase 1 follow-up, `fuzzy.go`).
- `internal/catalog/config-kinds.json` (96 config kinds: `kind`, `group`, `since`, `desc`) and
  `hack/gen-config-kinds.py` that regenerates it. Both are uncommitted: commit them in milestone 1.

## Milestone 1: config-kind catalogue (`internal/catalog/`)

1. `configkinds.go`: `//go:embed config-kinds.json`; `type ConfigKind struct{ Kind, Group, Since, Desc string }`;
   `ConfigKinds() []ConfigKind` (parsed once, `sync.Once`).
2. `ConfigCategoryFor(group string) string` mapping config groups to the existing category keys:
   `network→networking`, `container→containers`, every other group maps to itself
   (`kubernetes`, `block`, `storage`, `cri`, `runtime`, `hardware`, `hypervisor`, `cluster`,
   `security`, `extensions`, `siderolink`). Unknown → `other`.
3. `KindsAvailable(talosVersion string) []ConfigKind`: drop kinds whose `since` is newer than the
   node's `vMAJOR.MINOR`. Unknown/empty node version → keep all. Kinds whose `since` is a dev
   line newer than the node are hidden, not greyed.
4. Tests: every group maps to a known category; version filtering (`v1.13` hides `v1.14` kinds,
   empty keeps all, `v1.15.0-alpha.0` parses as `v1.15`).

## Milestone 2: machine-config documents (`internal/talos/`)

1. `SplitConfigDocs(raw string) ([]ConfigDoc, error)` in `configdocs.go`. `raw` is what the
   existing `GetMachineConfig` returns. Check what it returns (the full resource YAML, or the
   `spec` string already extracted) and handle that case. Split the multi-document stream on
   YAML document boundaries with `gopkg.in/yaml.v3` `Decoder` (already a dependency? check
   `go.mod`; add it if not). For each document: `Kind` (`kind:`; the legacy v1alpha1 document
   has no `kind`, so label it `v1alpha1` / `Config`), `Name` (`name:` if present), and `YAML`
   (that document re-encoded, or the original text slice, preferred so comments and order survive).
2. `ConfigDoc struct{ Kind, Name, YAML string }`.
3. Tests with a hand-written fixture: v1alpha1 doc + two `LinkConfig` (names `eth0`, `eth1`) +
   one `DHCPv4Config` + a trailing `---` and an empty document.

## Milestone 3: config section in the browser

1. On entering a category, also load (once per node, cached like the defs, `ctrl+r` clears) the
   node's machine config via `GetMachineConfig`. It needs `os:admin`: on permission error, show
   the Config section header with one dim row `requires os:admin` and carry on.
2. Types pane shows two sections, each with a dim header row that the cursor skips:
   `CONFIG` (catalogue kinds for this category, filtered by `KindsAvailable`, present ones
   first then absent ones greyed, each sorted by name) then `RESOURCES` (phase 1 rows).
   Count column for a config kind = number of documents of that kind.
3. Categories pane count becomes `present/known` across both sections.
4. `enter` on a config kind: 0 docs → status message `no <Kind> in this node's config`;
   1 doc → push instances pane + YAML pane (same depth rule as phase 1 resources);
   several → instances pane listing `NAME` (or `#1`, `#2` when unnamed), then YAML.
   The YAML shown is that one document, not the whole machine config.
5. `/` filter in the types pane matches across both sections; section headers stay visible only
   if their section has matches.
6. Tests: section rendering, cursor skipping headers, greyed absent kinds, permission-denied row,
   Esc depth is the same as for resources.

## Milestone 4: `d` describe

- In the types and instances panes, `d` opens (or toggles, if already open) a **describe** pane on
  the right in place of YAML:
  - Config kind: the `desc` from the catalogue, plus `since`, plus the group label.
  - Resource type: the `rd` definition (type, display type, aliases, default namespace,
    sensitivity) plus field docs from the Talos `explain` subcommand. First check that the
    subcommand exists and its arguments/output by reading the Talos source with Read/Grep
    (`~/sources/github.com/siderolabs/talos/cmd/`, search for `explain`). If it does not exist on
    v1.14, show the `rd` fields only and say so in your report. Wrap the call in
    `internal/talos/resources.go` (`ExplainResource`), cache per type, 10 s timeout.
- k9s parity: `d` = describe, `y` = YAML. In the describe pane, `y` switches to YAML and `d`
  switches back. `esc`/`q` close it.
- Tests for the toggling and for the config-kind describe text.

## Milestone 5: `ctrl+a` palette (k9s aliases view)

- `ctrl+a` from any browser pane **or the node list** (node list: uses the selected node; open
  the browser first) opens a full-width palette pane listing every config kind and resource type
  for the node. Columns: `NAME  ALIASES  CATEGORY  KIND (config|resource)  COUNT`.
- Typing filters immediately with the phase 1 fuzzy ranker (the palette is a list pane with its
  filter prompt already open). Exact alias match ranks first (k9s: `svc` is exact).
- Opening the palette starts a background count of every type not yet counted (reuse the
  phase 1 semaphore loader; keep concurrency 8). Rows fill in as counts arrive.
- `enter` jumps to that type: replace the stack with `[categories, types(category), …]`, with
  the cursor on the chosen row in each pane, then act as if `enter` was pressed on the type. So
  `esc` lands in its category, not back in the palette.
- `esc` with an empty filter closes the palette and restores the previous stack unchanged.
- Tests: open/close restores stack; jump builds the right stack and cursor; exact alias first.

## Milestone 6: `:` command mode

- `:` from the node list or any browser pane opens a command prompt in the status line (k9s §6):
  `enter`/`ctrl+e` submit, `esc` cancel, `ctrl+u`/`ctrl+w` clear, `backspace`, `tab`/`right`
  accept the inline suggestion (best fuzzy match, shown dim after the cursor), `up`/`down` cycle
  suggestions.
- Grammar for phase 2 (keep it this small):
  - `:nodes` / `:no` → node list.
  - `:<category key or label prefix>` (e.g. `:net`, `:block`) → that category on the current node.
  - `:<alias | display type | full type | config kind>` (case-insensitive, e.g. `:addr`,
    `:dhcpv4config`, `:addressstatuses.net.talos.dev`) → same jump as the palette.
  - `:a`, `:alias`, `:aliases` → palette. `:q`, `:q!`, `:quit` → quit. `:?`, `:h`, `:help` → help.
  - Unknown → status-line error `unknown command: <x>`, prompt closes.
- From the node list, commands other than `:nodes`/`:q`/`:help` need a node: use the selected one.
- Keep a command history (in memory) for `up`/`down` when the prompt is empty, like k9s.
- Tests: each grammar row, suggestion accept, history cycling, unknown command.

## Milestone 7: docs and polish

- Help overlay and hints: new keys appear automatically from the `browserActions` table; make
  sure `ctrl+a`, `:`, `d` are listed, and the node-list help mentions `ctrl+a` and `:`.
- README key table; update `docs/design/skill-md-additions.md` with the new files.
- Render tests at 80×24, 120×40, 200×50 for the palette, the describe pane, and the two-section
  types pane (height budget, width ≤ terminal, cursor visible).

## Done when

1. All milestones committed, `go build ./... && go vet ./... && go test ./...` green.
2. A short report to Andrew in this pane: what works, what was not verified against a live
   cluster, the exact CLI commands he should run and paste to check the machine-config shape
   and the `explain` output, and any deviation from this plan with the reason.
