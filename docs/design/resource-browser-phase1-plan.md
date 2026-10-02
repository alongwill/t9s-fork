# Phase 1 implementation plan: resource browser

You are implementing **phase 1** of `docs/design/resource-browser.md` in this worktree
(branch `feat/resource-browser`, based on `feat/talos-1.14`). Read these first, in order:

1. `docs/design/resource-browser.md`: the design. Phase 1 = resources only.
2. `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md`: t9s
   conventions (value-receiver App, async `loadX()`, render tests, talosctl contract).
   It is not in this worktree (untracked in the main checkout). Read it by absolute path.
3. `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md`: the k9s keymap.
   The browser must behave like k9s wherever the same concept exists (§5 and §6 of that file).

## Hard rules

- **Never run `talosctl`, `kubectl`, `omnictl`, `talosctl`-wrapping scripts, or any Bash command
  whose text contains those names** (a hook blocks it, even in `grep`). Use the Read/Grep tools to
  read Talos source; write fixtures by hand. If you need real output, stop and ask Andrew for the
  exact command to run.
- Stay in this worktree. Do not touch `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork`.
- Bubble Tea stays. Do not port to tcell/tview. Do not refactor views outside the browser.
- Subprocess only (the existing `Client.run`). No gRPC client in phase 1.
- Commit in small steps (one per milestone below). End every commit message with:
  `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`. Do not push.
  Commits are GPG-signed. If signing fails (timeout / pinentry), **do not** use `--no-gpg-sign`
  or change git config: tell Andrew in one line, keep working, and retry the commit at the next
  milestone. The first commit should include `docs/design/` (already written, uncommitted).
- `go build ./... && go vet ./... && go test ./...` must pass at every commit.

## Scope

In: node → categories → types (greyed if empty) → instances → YAML pane; k9s keys below.
Out (phase 2/3, do not build): config documents, embedded config-kind catalogue, `ctrl-a`
palette, `:` command mode, `d` describe, gRPC, watch, cross-node compare.

## Milestone 1: data layer (`internal/talos/`)

1. `types.go`: add
   ```go
   type ResourceDef struct {
       Type             string   // e.g. "AddressStatuses.net.talos.dev"
       DisplayType      string   // e.g. "AddressStatus"
       Aliases          []string
       DefaultNamespace string
       Sensitive        bool
   }
   type ResourceMeta struct { Namespace, Type, ID, Version, Phase string }
   ```
2. `resources.go` (new):
   - `GetResourceDefinitions(ctx, node) ([]ResourceDef, error)`: runs
     `get rd -n <node> -o json`, parses with `parseJSONStream`. Each object is
     `{"node":…, "metadata":{…}, "spec":{…}}`. Expected spec keys (verify against the COSI
     `ResourceDefinitionSpec` yaml tags if you can find the module source with
     `find "$(go env GOMODCACHE)/github.com/cosi-project" -name 'resource_definition*.go'`;
     if not found, keep these and make parsing tolerant): `type`, `displayType`, `aliases`,
     `allAliases`, `defaultNamespace`, `sensitivity` (treat any non-zero / non-empty /
     `"sensitive"` value as sensitive; log nothing, just set the flag).
   - `ListResources(ctx, node, ns, typ) ([]ResourceMeta, error)`: `get <typ> -n <node>
     --namespace <ns> -o json`. Empty stdout ⇒ empty slice, no error.
   - `GetResourceYAML(ctx, node, ns, typ, id) (string, error)`: `get <typ> <id> -n <node>
     --namespace <ns> -o yaml`.
   - `IsPermissionDenied(err) bool`: matches `PermissionDenied` / `not authorized` in stderr.
   - Check how existing wrappers pass the node (`-n`/`--nodes`) and copy that exactly.
3. `internal/talos/resources_test.go`: fixture tests. Hand-write an `rd` JSON stream fixture
   (3–4 objects, pretty-printed like the other fixtures), a list fixture with two instances,
   an empty-output case, and a permission-denied error string.

## Milestone 2: categories (`internal/catalog/`)

1. `categories.go`: `type Category struct{ Key, Label string }` and an ordered slice matching the
   table in the design doc (Networking … Runtime and OS, Other).
2. `CategoryFor(def talos.ResourceDef) string`: split `Type` on `.`; the second segment is the
   suffix (`net`, `block`, …). Map via the design-doc table. Overrides by display type first:
   any `Extension*` type → `extensions`. Unknown suffix → `other`.
3. `categories_test.go`: table test covering every suffix in the design doc, the block-in-runtime
   case (`Disks.block.talos.dev` → Block even though namespace is `runtime`), an extension
   override, and an unknown suffix.

## Milestone 3: pane stack + keymap (`internal/ui/`)

New states in `app.go`: `StateCategories`, `StateBrowser` (one state for the whole browser; the
pane stack decides what is shown). Keep browser state in one struct field on `App`:

```go
type paneKind int // paneCategories, paneTypes, paneInstances, paneYAML
type pane struct {
    kind   paneKind
    title  string
    cur    int
    scroll int
    filter string // active `/` filter for this pane
    // payload: category key, type def, instance meta, yaml text, as needed
}
type browser struct {
    node      talos.Node
    defs      []talos.ResourceDef
    counts    map[string]int   // type → instances; -1 = locked; missing = not loaded
    loading   map[string]bool
    stack     []pane            // push on Enter, pop on Esc/q
    fullscreen bool             // YAML pane `f`
    wrap      bool              // YAML pane `w`
    find      string; findHits []int; findIdx int // YAML `/` search
}
```

Values only (App is a value receiver): copy maps before mutating them in `Update`, or replace the
map wholesale from the message. Never store pointers into App.

**Keymap (`internal/ui/browserkeys.go`)**: a small k9s-style table so hints and help come from one
place. Do not change other views.

```go
type keyAction struct { keys []string; desc string; visible bool; fn func(App) (App, tea.Cmd) }
func (app App) browserActions() []keyAction // depends on top pane kind
```

`handleBrowserKey` walks the slice; `stateHints` renders the `visible` ones; the help overlay
lists all of them. Bindings (match k9s; `keys` are bubbletea `msg.String()` values):

| Keys | Pane | Action |
|---|---|---|
| `up` `k` / `down` `j` | lists | move |
| `g` `home` / `G` `end` | lists, YAML | top / bottom |
| `ctrl+f` `pgdown` / `ctrl+b` `pgup` | lists, YAML | page |
| `enter` | lists | drill in (push) |
| `esc` | all | if a filter/search is set: clear it. Else pop one pane. Popping the last pane returns to the node list with the node still selected (cursor unchanged). |
| `q` | all | same as `esc` (k9s: back in detail views) |
| `/` | lists | filter this pane (prompt in the status line; `enter` applies, `esc` cancels). Plain text = case-insensitive regex, fall back to substring if the regex is invalid; `!term` = inverse. |
| `/` then `n` / `N` | YAML | search, next / previous match |
| `y` | instances | open YAML pane for the selection (same as `enter`) |
| `w` | YAML | toggle wrap |
| `f` | YAML | toggle full screen |
| `ctrl+r` | all | reload the data behind the top pane |
| `?` | all | help (existing global handler; make sure it lists browser keys in this state) |
| `ctrl+c` | all | quit (existing) |

The existing global `/` and `w` handlers in `keyrouter.go` run before per-state dispatch. Make
sure `StateBrowser`/`StateCategories` are not in their state lists, so the browser handles its own.

**Node list entry**: in `nodelist.go`, `a` now opens the browser for the selected node. Move the
existing addresses view to `A` (update hints, help, README key table). Rationale: Andrew asked for
`a`; k9s has no conflicting meaning on a node row.

Commit after milestone 3 with a stub renderer (titles only) so the stack is testable.

## Milestone 4: loading

- On `a`: set `browser.node`, push a `paneCategories` pane, fire `loadResourceDefs()`. Cache defs
  per node IP on App (`map[string][]talos.ResourceDef`) so re-entering is instant; `ctrl+r` clears.
- On `enter` on a category: push `paneTypes`, fire `loadCounts(types)`: `tea.Batch` of one cmd per
  type not already counted, concurrency limited to 8 by a semaphore channel (`make(chan struct{}, 8)`)
  created once and captured in every closure. Each cmd calls `ListResources` and returns
  `resourceCountMsg{node, typ, n, locked, err}`, so rows fill in one by one.
  Drop messages whose `node` no longer matches `browser.node` (stale).
- Timeouts: 10 s per call (`context.WithTimeout`), like the other loaders.
- On `enter` on a type: if count == 1 → push `paneInstances` *and* immediately `paneYAML`
  (so Esc from YAML lands on the one-row instance list — consistent depth). If count > 1 → push
  `paneInstances` and fire `loadInstances`. Count 0 or locked → no-op with a status message
  (`no <DisplayType> on this node` / `requires os:admin`).
- On `enter`/`y` on an instance: push `paneYAML`, fire `loadYAML`.

## Milestone 5: rendering (`internal/ui/browser.go`)

- Header breadcrumb: `node: <hostname> (<role>) > <Category> > <DisplayType> > <id>`.
- Miller columns: render the stack left to right. Width ≥ 120: up to 3 panes visible (the last
  three in the stack); below 120: last two; `fullscreen`: YAML only. Fixed widths for list panes
  (categories 26, types 40, instances 36), YAML takes the rest.
- Categories rows: `Label  present/known` (`?/N` until counted). Types rows:
  `DisplayType  aliases[0]  count`; count 0 → whole row `dimStyle` and `-`; locked → dim with
  `lock` text marker (no emoji, width-safe); loading → `…`.
- Instances rows: `ID  NAMESPACE  VERSION  PHASE`.
- YAML pane: viewport-style scrolling with line numbers off, `w` wrap, search hits highlighted.
  Colour top-level keys with an existing style; no new dependencies.
- Follow the SKILL list rules: compute widths from `app.width`, pad with `col()` before
  colourising, `clampScrollStart`, pad every line to full width, keep the cursor visible.

## Milestone 6: tests, docs

- `internal/ui/render_test.go` (copy existing patterns): browser at 80×24, 120×40, 200×50 —
  height budget, max line width ≤ terminal width, cursor row visible after moving past the
  fold, greyed row rendered dim, 2-pane collapse below 120 cols, full screen YAML.
- `internal/ui/browserkeys_test.go`: Esc clears filter before popping; Esc on the last pane
  returns to `StateNodeList` with `nodeCur` unchanged; `q` == `esc`; `enter` on a 0-count type
  does not push; stale `resourceCountMsg` for another node is ignored; `a` on node list opens
  the browser and `A` opens addresses.
- README: add the browser keys to the key table, note `a`/`A` change.
- Update the t9s `.claude/SKILL.md` layout table with the new files (write the change into
  `docs/design/skill-md-additions.md` in this worktree instead, since SKILL.md is untracked).

## Done when

1. All milestones committed, `go build ./... && go vet ./... && go test ./...` green.
2. A short report to Andrew in this pane: what works, what was not verified against a live
   cluster, exact `talosctl` commands he should run to sanity check the `rd` / list JSON shapes
   (so fixtures can be corrected), and anything you deviated from in this plan with the reason.
