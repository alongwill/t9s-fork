# Phase 3 implementation plan: resource browser

You are implementing **phase 3** (the final phase) of `docs/design/resource-browser.md` in this
worktree (branch `feat/resource-browser-phase3`, off `main`, which contains phases 1 and 2).
Read these first, in order:

1. `docs/design/resource-browser.md`: the design. Phase 3 row, and "Open questions" 1.
2. `docs/design/resource-browser-phase1-plan.md`: its **Hard rules** apply to you, except that
   commits are not GPG-signed (`commit.gpgsign=false` on the repo). Commit normally.
3. `docs/design/skill-md-additions.md`: file map after phases 1 and 2.
4. `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md`: t9s conventions,
   especially the **Streaming** pattern (buffered chan, `waitForX`, `sessionSeq`, cancel in
   `cleanup()`/`goBack()`).
5. `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md`: k9s keymap.
6. Phase 1/2 code: `internal/ui/browser*.go`, `internal/talos/resources.go`,
   `internal/talos/configdocs.go`, `internal/catalog/`.

Reminders: never run the Talos/Kubernetes/Omni CLIs and never put their names in a Bash command
(a hook blocks it, even in `grep` or a path). Use the Read/Grep/Glob tools for the Talos source at
`~/sources/github.com/siderolabs/talos`. Chain verification with `&&`, never `;`. Do not push.
Do not run `go get -u` on unrelated modules.

## Decision taken for open question 1

Andrew approved phase 3, so a gRPC client is allowed, **for the resource browser only**. Every
other t9s view keeps using the subprocess client. The subprocess source stays as the fallback:
the browser must still work when gRPC cannot connect (e.g. a talosconfig auth mode the library
does not support).

## Facts already checked (Talos source, v1.14.2)

- Module: `github.com/siderolabs/talos/pkg/machinery`, tag `pkg/machinery/v1.14.2`
  (`go get github.com/siderolabs/talos/pkg/machinery@v1.14.2`). It pins
  `github.com/cosi-project/runtime v1.16.3` and `go 1.26.5`; t9s is `go 1.26.8`, so no bump needed.
- `pkg/machinery/client`: `client.New(ctx, opts...)` with `WithConfigFromFile(path)`,
  `WithContextName(name)`, `WithDefaultConfig()`. `c.COSI` is a `state.State`
  (`client.go:56`). Target one node with `client.WithNode(ctx, node)` (`context.go:32`).
- How the CLI lists and watches resources: read the `get` command implementation under
  `~/sources/github.com/siderolabs/talos/cmd/` (find it with Glob `**/talos/get.go`). Copy its
  approach for resolving a type/alias via `ResourceDefinitions`, `COSI.List`, and `COSI.Watch`
  / `WatchKind` with bootstrap contents. Read it, do not guess the API.

## Milestone 1: `ResourceSource` interface

1. `internal/talos/source.go`:
   ```go
   type ResourceSource interface {
       Name() string // "grpc" or "cli", shown in the header
       Definitions(ctx context.Context, node string) ([]ResourceDef, error)
       List(ctx context.Context, node, ns, typ string) ([]ResourceMeta, error)
       GetYAML(ctx context.Context, node, ns, typ, id string) (string, error)
       Watch(ctx context.Context, node, ns, typ string, out chan<- WatchEvent) error
       Close() error
   }
   type WatchEvent struct { Kind string /* created|updated|destroyed|bootstrapped|error */; Meta ResourceMeta; Err error }
   ```
2. `cliSource` wraps the existing `Client` methods. Its `Watch` returns
   `ErrWatchUnsupported` (the CLI `--watch` output is a table, not worth parsing).
3. Switch every browser loader from direct `Client` calls to `app.source`. No behaviour change.
   Commit with all tests still green.

## Milestone 2: gRPC source

1. `internal/talos/grpcsource.go`: `NewGRPCSource(ctx, cfgPath, contextName) (ResourceSource, error)`.
   Use the same talosconfig path and context t9s already resolved (`internal/config`).
2. `Definitions`: list `ResourceDefinitions` (namespace `meta`, check the CLI code for the exact
   type and namespace constants) and map spec → `ResourceDef`, same fields as the CLI parser.
3. `List`: `COSI.List` → `ResourceMeta` (id, namespace, type, version, phase).
4. `GetYAML`: `COSI.Get`, then marshal the same way the CLI's yaml output does (find the
   printer it uses for `-o yaml` and reuse it so the YAML pane looks identical between sources).
5. Permission errors: map gRPC `codes.PermissionDenied` so `IsPermissionDenied` keeps working
   for both sources (the UI shows the `lock` marker).
6. Selection at startup (`cmd/main.go` / app init): try gRPC with a 5 s timeout. On failure, log
   the reason to the status line once (`gRPC unavailable (<short reason>), using CLI`) and use
   `cliSource`. Add a `--source=auto|grpc|cli` flag (default `auto`).
7. Show the active source in the browser header: `src: grpc` / `src: cli`.
8. Tests: an in-memory COSI state (`github.com/cosi-project/runtime/pkg/state/impl/inmem` +
   `namespaced`) with a few resources, exercising List/Get/Watch mapping without a network.
   If the generic resource types make that too heavy, test the mapping functions directly.

## Milestone 3: count everything in one go

With `grpc` active, opening the browser counts **all** types up front (no lazy per-category
step): the categories pane shows real `present/known` immediately. Keep the semaphore at 8. With
`cli` active, keep phase 1's lazy behaviour. Test both paths with a fake source.

## Milestone 4: live watch of the open type

- When an instances pane is on top and the source supports `Watch`, start a watch for that type
  on that node. Apply events to the instance list (add/update/remove rows; keep the cursor on the
  same ID if it still exists). Flash changed rows for ~1 s with an existing highlight style.
- If the YAML pane for an instance is open and that instance is updated, reload its YAML
  (keep the scroll position if the line count is unchanged).
- Follow the t9s streaming pattern exactly: buffered channel, `waitForResourceWatch(ch)`
  re-armed per message, `context.CancelFunc` stored on App, a `watchSeq` so events from an old
  watch are dropped, cancelled on pop, on `ctrl+r`, on node change, and in `cleanup()`.
- Header indicator: `watch` (or `watch off`). **`W`** (`Shift-W`) toggles the watch on/off.
  Not `ctrl+w`: k9s uses that for wide columns. List `W` in hints.
- With `cli` source: `W` shows `watch needs the gRPC source`.
- Tests: event application (add, update, destroy, cursor kept), stale `watchSeq` dropped,
  watch cancelled on Esc.

## Milestone 5: cross-node compare

- In an instances pane (or the types pane for a type/config kind with exactly one instance),
  **`x`** ("cross-node"; k9s uses `x` only for secret decode) opens the compare pane. Check the
  browser keymap for a conflict first; if `x` is taken, pick a free key and say which in your report.
- It opens a compare pane: one row per node in the cluster (`NODE  ROLE  PRESENT  VERSION  SAME?`)
  for the selected type + ID (for config kinds: kind + name). Load the YAML from every node
  concurrently (semaphore 8, 10 s each).
- Normalise before comparing: drop `metadata.version`, `metadata.created`, `metadata.updated`,
  and the `node` field. `SAME?` compares against the node the browser was opened on.
- `enter` on a node row opens a **diff pane**: unified diff of the normalised YAML, the browser
  node vs that node, `+`/`-` lines coloured. Implement the diff with a small LCS line diff in
  `internal/ui/diff.go` (no new dependency), or `github.com/pmezard/go-difflib` if it is already
  in `go.sum`.
- `esc`/`q` back out one level as everywhere else.
- Tests: normalisation, diff output for a known pair, compare rows for present/absent/different.

## Milestone 6: docs and polish

- README: the source flag, `W`, the compare key, and what needs gRPC.
- Help overlay and hints include the new keys (from the `browserActions` table).
- Update `docs/design/resource-browser.md`: mark phases 1–3 done, record the answer to open
  question 1, list anything deferred.
- Update `docs/design/skill-md-additions.md` with the new files and the gRPC exception to the
  "subprocess only" rule.
- Render tests at 80×24, 120×40, 200×50 for the compare pane and the diff pane.

## Done when

1. All milestones committed, `go build ./... && go vet ./... && go test ./...` green.
2. A short report to Andrew in this pane: what works, what was not verified against a live
   cluster, how to run t9s to test it (`--source=grpc`, then `W` on an instances pane, then the
   compare key on a resource that exists on every node), and any deviation from this plan with
   the reason. If the gRPC source fails against Omni-managed talosconfigs, say so plainly: the
   CLI fallback must still work there.
