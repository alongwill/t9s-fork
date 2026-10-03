# Additions for `.claude/SKILL.md` (untracked in the main checkout)

Paste these into the t9s SKILL. They are kept here because SKILL.md is not tracked.

## Layout table: new rows

| Need | File |
|---|---|
| Resource browser state, pane stack, row models, layout, matcher | `internal/ui/browser.go` |
| Browser key table (`keyAction`), prompts, enter/back/reload | `internal/ui/browserkeys.go` |
| Browser loaders and reply handlers (`loadResourceDefs`, `loadCounts`, …) | `internal/ui/browserload.go` |
| Browser renderer (Miller columns, YAML pane) | `internal/ui/browserrender.go` |
| `resource*Msg` types | `internal/ui/messages.go` |
| COSI wrappers: `GetResourceDefinitions`, `ListResources`, `GetResourceYAML`, `IsPermissionDenied` | `internal/talos/resources.go` |
| `ResourceDef`, `ResourceMeta` | `internal/talos/types.go` |
| Type → category mapping | `internal/catalog/categories.go` |
| Config-kind catalogue (`go:embed config-kinds.json`), `ConfigKinds`, `KindsAvailable`, `ConfigCategoryFor` | `internal/catalog/configkinds.go` |
| Regenerate the catalogue from the Talos source | `hack/gen-config-kinds.py` |
| `SplitConfigDocs`, `ConfigDoc` (machine config → documents) | `internal/talos/configdocs.go` |
| Config section: row model (`typeEntries`, `typeVisual`), per-node doc cache, `configDocsMsg` handler | `internal/ui/browserconfig.go` |
| `d` describe pane (`paneDescribe`) | `internal/ui/browserdescribe.go` |
| `ctrl+a` palette (`paneAliases`), `jumpTo`, `jumpCategory`, `categoryStack` | `internal/ui/browserpalette.go` |
| `:` command mode (`cmdPrompt`, suggestions, history, grammar) | `internal/ui/command.go` |
| `ResourceSource` interface, `cliSource`, `WatchEvent`, `ErrWatchUnsupported` | `internal/talos/source.go` |
| gRPC source (`NewGRPCSource`, mapping helpers, CLI-identical YAML) | `internal/talos/grpcsource.go` |
| `--source` modes, background dial, fallback status, `app.src()` | `internal/ui/source.go` |
| Live watch (`syncWatch`, `waitForResourceWatch`, flash, `W`) | `internal/ui/browserwatch.go` |
| Cross-node compare (`c`): subjects, normalisation, fetch, rows, render | `internal/ui/compare.go`, `internal/ui/compare_render.go` |
| LCS line diff + unified diff | `internal/ui/diff.go` |
| Next-step line (`nextStep`, `nextStepRows`: one row of the browser height budget) | `internal/ui/nextstep.go` |
| Tips (`browserTips`, `showTip`, `:tips on/off`) | `internal/ui/tips.go` |
| Dependency graph cache per node (`depEntry`, `depsMsg`, `ensureDeps`, `reloadDeps`) | `internal/ui/browserdeps.go` |
| Knowledge notes (`go:embed resource-notes.yaml`), `NoteFor`, `NoteTypes` | `internal/catalog/notes.go`, `internal/catalog/resource-notes.yaml` |
| `DepGraph`, `DepEdge`, `Producers` / `Consumers` / `Inputs` / `Outputs`, `DepGraphFromProto`, `ParseDepDOT`, `ErrNeedsGRPC` | `internal/talos/deps.go` |
| Test fakes: in-memory `fakeSource`, `collectMsgs` / `feed` | `internal/ui/fakesource_test.go`, `internal/ui/countall_test.go` |

## Patterns

- **Browser state is one struct** (`App.browser`). The pane stack decides what is shown;
  `StateCategories` = one pane, `StateBrowser` = deeper (`syncBrowserState`). Mutate through
  `push` / `pop` / `withTop` / `setCount` / `setLoading` / `setSingle`: they clone slices and
  maps, so never write to `app.browser.stack[i]` or `.counts[k]` directly.
- **Keys come from one table.** `browserActionsFor(kind)` drives dispatch, the hint bar
  (`visible` entries) and the help overlay. Add a binding there and nowhere else.
- **Prompt first.** `handleKey` routes to `handleBrowserPrompt` before the global `x` / `?`
  handlers while `browser.prompting` is set, so typed letters are not stolen.
- **Replies carry their node.** `resource*Msg` has `node`; handlers drop replies whose node is not
  `browser.node.IP` or when the stack is empty (user left the browser).
- **Counts are lazy**, per category, one cmd per type, capped by `App.resSem` (8). `-1` locked
  (permission denied), `-2` error, missing key = not loaded. A count of exactly 1 also stores the
  instance in `browser.singles` so Enter can open list + YAML without a second call.
- Browser render tests need `newTestApp` + `browserApp(w, h, depth, nTypes)` from
  `browserkeys_test.go`; `renderHeader` needs `cfg`, so test `resourceLine` / `renderBrowser` instead.

## talosctl contract: new rows

| t9s call | Notes |
|---|---|
| `get rd -n <node> -o json` | One object per type. `spec.{type,displayType,aliases,allAliases,defaultNamespace,sensitivity}` (COSI `ResourceDefinitionSpec` yaml tags, checked against cosi-project/runtime v1.16.2). `sensitivity` is `""` or `"sensitive"`. Not yet checked against a live Talos 1.14 node. |
| `get <type> -n <node> --namespace <ns> -o json` | Stream of objects; `metadata.{namespace,type,id,version,phase}`. `version` may be a number or a string. Empty stdout = no instances. Sensitive types fail with `PermissionDenied` without `os:admin`. |
| `get <type> <id> -n <node> --namespace <ns> -o yaml` | Full YAML of one resource. |
| `get <type> -n <node> ... -o json` (`metadata.owner`) | `ResourceMeta.Owner` is `metadata.owner`: the controller that wrote the resource. |
| `inspect dependencies -n <node>` | Graphviz DOT of the controller-resource graph (see Learning patterns). Single node only. |
| `get machineconfig v1alpha1 -n <node> -o yaml` | Config documents. `spec` is a string holding the multi-document stream; `SplitConfigDocs` also accepts a bare stream. Needs `os:admin` (`PermissionDenied` → `requires os:admin` row). Not yet checked against a live Talos 1.14 node. |

- **Types pane has two sections.** `typeEntries` is the list of *selectable* rows (config kinds, then
  resources); `pane.cur` indexes it. `typeVisual` adds the `CONFIG` / `RESOURCES` header and note rows,
  and `pane.scroll` is in those visual lines (`visualIndex` converts). Headers are never selectable, so
  no cursor-skipping code is needed. Config kinds are shown only while the machine config is readable
  (`configShown`); `cfgDenied` shows a `requires os:admin` note instead.
- **Config documents** load once per node when a category opens (`ensureConfig`), cached in
  `App.configDocs`; `ctrl+r` on a config pane or the types pane drops the cache entry and refetches
  (`reloadConfig`). A config instances/YAML pane has `pane.cfgKind` set; its YAML is the document text, not a fetch.
- **Describe, palette and jumps are panes on the same stack.** Closing = `popPane`. `jumpTo` / `jumpCategory`
  *replace* the stack (`categoryStack`) so `esc` lands in the category.
- **Prompts:** `app.cmd` (`:`) is checked first in `handleKey`, then `browser.prompting` (`/` filter, YAML find;
  the palette reuses the filter prompt, with `handlePalettePromptKey` for `esc`/`enter`). A `:` command that
  needs the resource definitions before they have arrived is parked in `browser.pendingCmd` and run by
  `handleResourceDefs`.
- **Describe has no per-field docs:** it shows the `rd` fields, the knowledge note and the relationships.

## Phase 3 patterns

- **gRPC exception.** The "no gRPC client" rule in SKILL.md now reads: only the resource browser may use
  `pkg/machinery/client`, through `talos.ResourceSource`; every other view uses the subprocess `Client`. The CLI
  source is the fallback and is used whenever the dial fails. `app.src()` returns it when `app.source` is nil
  (tests, and before the background dial answers).
- **Watch lifecycle** goes through `syncWatch` only, called from `Update` after every key and after `sourceReadyMsg`
  (not from `handleKey`, so watch tests use `app.Update`). It starts/keeps/stops from the stack and compares
  `watchKey` (node|type). `stopWatch` (pointer receiver, like `stopLogs`) is also called from `cleanup()` and
  ctrl+r. Events carry `watchSeq`; old ones are dropped without re-arming. After bootstrap the watch is the
  source of truth: late `resourceInstancesMsg` replies do not overwrite items.
- **Compare** replies carry `compareSeq`. The key is `c`: the global context switcher takes `x` before the
  browser key table. `paneCompare` and `paneDiff` use the full width. Config documents come from
  `app.getConfig` (nil = CLI client) so tests can inject machine configs.
- gRPC YAML is rendered like the CLI's `-o yaml` (`node:` line, then `metadata` / `spec`, machine-config special case).

## Learning patterns (PR A)

- **Key table + feedback.** `browserKeyTable` (browserkeys.go) lists every pane-specific key with the panes it
  works in and a "where" text. A key pressed in a pane that does not bind it sets a status note
  (`unboundKeyNote`). A test compares the table with `browserActionsFor`, so adding a binding means adding
  or updating the table row. `c` / `W` on a types row go through `keyOnType`: open the instance list, then
  `pick one, then press c`.
- **Describe rows are computed, not stored.** `pane.sub` (`descSubject`) holds what the pane is about;
  `describeRows` rebuilds the rows on every render from the subject, the notes and the cached graph, so a graph
  that arrives late fills in. Selectable rows (`drow.sel`) are those naming a type in `browser.defs`;
  `pane.cur` indexes them, `describeMove` keeps the selected line in view, `describeJump` reuses `jumpTo`.
- **Height budget.** `nextStepRows()` is 1 when the next-step line shows; `paneInnerRows` and `renderBrowser`
  both subtract it. Any new browser chrome must go through the same function.
- **Tips** use `App.statusMsg`; a leftover tip (`App.tipMsg`) does not count as "another message".
- **Dependency graph.** gRPC: `Inspect.ControllerRuntimeDependencies` per node. CLI: the DOT that
  `inspect dependencies` prints (emicklei/dot layout: `nK[label=…,shape="box"]` controllers, `shape="note"` types,
  `nA->nB[style=…]` edges; controller to type = output, type to controller = input, `dotted` = weak input,
  edge label = resource ID). The CLI graph has no namespaces. Unparsable output gives `ErrNeedsGRPC`.
  Not yet checked against real output from a node.
- **Notes source of truth** is the Talos skill (`agent-skills/talos/knowledge/resource-notes.yaml`). Never edit
  `internal/catalog/resource-notes.yaml`: edit the skill copy, then run `hack/sync-resource-notes.sh`
  (`--check` exits 1 when the copy is stale; `$TALOS_SKILL_DIR` overrides the skill path).
- **Notes format** (`resource-notes.yaml`): a map keyed by display type (`LinkStatus`) with `what` (one sentence),
  optional `ubuntu`, optional `lookWhen` list. Source the text from the Talos source (`pkg/machinery/resources/`)
  and skill references; several upstream doc comments are copy-paste wrong, so check the Spec fields. A test
  checks every key against a hard-coded list of real display types: add the type there too.
- **Regenerate `config-kinds.json`:**
  `python3 hack/gen-config-kinds.py <talos skill>/tutorials/config-explorer-v2/data.js > internal/catalog/config-kinds.json`
  (now includes the `ubuntu` field).

## Gotchas

- Node-list `a` is the browser; the addresses view moved to `A`.
- `lipgloss` emits no escapes in tests unless you call `lipgloss.SetColorProfile(termenv.ANSI256)`
  (restore it after). `TestRenderBrowserGreyedRowIsDim` shows the pattern.
