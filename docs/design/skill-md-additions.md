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

## Gotchas

- Node-list `a` is the browser; the addresses view moved to `A`.
- `lipgloss` emits no escapes in tests unless you call `lipgloss.SetColorProfile(termenv.ANSI256)`
  (restore it after). `TestRenderBrowserGreyedRowIsDim` shows the pattern.
