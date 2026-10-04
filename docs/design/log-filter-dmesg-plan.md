# Plan: log filtering, dmesg colour and options, pod namespace in metrics

Branch `feat/log-filter-dmesg`, off `main`, worktree
`/Users/andrewlongwill/sources/github.com/alongwill/worktrees/t9s-fork-logs-dmesg`.

**Another agent works in parallel** on Omni detection, the role badge and read-only mode
(`internal/ui/app.go` header/top bar, `keyrouter.go`, `internal/config/`, `cmd/main.go`, the browser).
Do **not** edit `app.go`'s header/top-bar code, `keyrouter.go`, `internal/config/`, `cmd/main.go`,
or `internal/ui/browser*.go`. If you must add a state or field in `app.go`, keep the change to the
lines you need (field, `AppState` const, one `case`) so the merge stays trivial.

## Read first

1. `docs/design/resource-browser-phase1-plan.md` **Hard rules** (commits not GPG-signed now).
   Never run or name the Talos/Kubernetes/Omni CLIs in a Bash command (a hook blocks it, even in
   `grep` or a path); use Read/Grep/Glob on `~/sources/github.com/siderolabs/talos`. Chain checks
   with `&&`. Do not push.
2. `docs/design/logs-members-containers-plan.md`: how the k9s-style log options were built.
3. `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md` and
   `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md` (§5 text views, §6 filter grammar).
4. Code: `internal/ui/logs.go` (+ its log-option and level-colouring helpers), `dmesg.go`,
   `metrics.go`, `containers.go`/`containerdetail.go` (container ID parsing), `fuzzy.go`.

## Milestone 1: log filter (`/` hides non-matching lines)

k9s's log view filters: lines that do not match disappear. Change the service log view's `/` from
"find and highlight" to **filter**, live as you type:

- Grammar (k9s §6, adapted for log lines, where pure subsequence fuzzy matches almost everything):
  - plain words: every word must appear in the line, any order, case-insensitive (`dns timeout`).
    This is the default "fuzzy" mode.
  - `!word`: the line must not contain `word` (combinable: `error !probe`).
  - `-f term`: true fuzzy (subsequence, ranked by the existing `fuzzyScore`; keeps line order,
    hides lines below a minimum score).
  - `-r regex`: regular expression.
- `enter` keeps the filter, `esc` in the prompt restores the previous filter, `esc` outside the
  prompt clears the filter (k9s).
- The indicator line gains `Filter:<term> (N/M)` when a filter is active.
- New lines arriving while filtered are tested as they arrive; autoscroll follows only matching lines.
- Keep `n`/`N` working: with a filter active they step through the visible lines' matches.
- Tests: each grammar form, live narrowing, esc behaviours, new lines filtered, autoscroll + filter.

## Milestone 2: dmesg gets the log viewer's options and colour

- Make dmesg use the same log component as the service log viewer: the indicator line
  (`Autoscroll FullScreen Timestamps Wrap`, plus `Filter`), keys `s f t w`, `/` filter from
  milestone 1, `G` resume. Prefer extracting a shared `logPane` type used by both views over
  copying code; keep each view's title and stream source.
- Colour: dmesg lines carry facility and level (check the exact format in the Talos source:
  Glob `**/talos/dmesg.go` under `~/sources/github.com/siderolabs/talos/cmd/`, and the kmsg
  formatting it uses). Colour the level token (`emerg/alert/crit/err` red bold, `warning` yellow,
  `notice/info` blue, `debug` dim), the facility dim, and the kernel timestamp dim.
  `t` Timestamps uses the kernel's own timestamp when present (normalised to
  `YYYY-MM-DDTHH:MM:SS.sss`), else arrival time with `~`.
- Tests: level parsing for each level, facility/timestamp extraction, shared component behaves the
  same in both views.

## Milestone 3: pod namespace in container metrics

The container metrics view (metrics for the CRI namespace) shows the container but not the
Kubernetes namespace. Reuse the container-ID parser added for the container detail view to split
pod namespace / pod / container, and add a `NAMESPACE` column (pod namespace; `-` for non-CRI
rows), sortable like the other columns if the view supports sorting. Keep widths within budget at
80 columns (drop or truncate the least useful column first and say which in the report).
Tests: parsing reuse, render budget.

## Milestone 4: docs

README key tables (log filter grammar, dmesg keys), help overlay and hints.

## Done when

All milestones committed; `go build ./... && go vet ./... && go test ./...` green; a short
report to Andrew: what works, what was not verified live, any format he should paste to confirm
(dmesg line shape), deviations with reasons.
