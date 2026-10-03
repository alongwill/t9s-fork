# Plan: k9s-style logs, members IP fix, containers drill-down

Branch `feat/logs-members-containers`, off `main`, in worktree
`/Users/andrewlongwill/sources/github.com/alongwill/worktrees/t9s-fork-logs-containers`.

**A second agent is working in parallel** in another worktree on the resource browser
(`internal/ui/browser*.go`, `related.go`, `compare.go`, `internal/catalog/`, `internal/ui/styles.go`).
Do **not** edit those files. Put any new styles in your own files (e.g. `internal/ui/logstyles.go`),
built from the existing colours in `styles.go`.

## Read first

1. `docs/design/resource-browser-phase1-plan.md` **Hard rules**: they apply, except commits are not
   GPG-signed. Never run or name the Talos/Kubernetes/Omni CLIs in a Bash command (a hook blocks it,
   even in `grep` or a path); use Read/Grep/Glob on `~/sources/github.com/siderolabs/talos`.
   Chain checks with `&&`, never `;`. Do not push.
2. `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md`: t9s conventions,
   especially streaming (logs) and the talosctl contract table.
3. `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md` §5 "Text views": the
   k9s log keys.
4. Code: `internal/ui/logs.go`, `internal/ui/nodelist.go`, `internal/ui/containers.go`,
   `internal/talos/client.go` (`GetNodes`, `GetContainers`, `GetStats`, `GetProcesses`, the logs
   streamer), `internal/ui/hints.go`, `internal/ui/helpview.go`.

## Milestone 1: member IP prefers IPv4

Bug: `GetNodes` picks the address equal to the responding `node`, else the **last** address. A
worker reported by a control plane has `addresses: ["172.30.0.3", "fd51:…:e340"]`, so t9s shows
and targets the IPv6 address. Andrew's real output:

```
NODE         … ID                             … MACHINE TYPE   ADDRESSES
172.30.0.2   … vanilla-talos-controlplane-1   … controlplane   ["172.30.0.2","fd51:f3ce:3650:a4dc:50bf:56ff:fe80:1e65"]
172.30.0.2   … vanilla-talos-worker-1         … worker         ["172.30.0.3","fd51:f3ce:3650:a4dc:a004:64ff:fea2:e340"]
```

New rule: exact match with `node` first (unchanged); otherwise the **last IPv4** address (the VIP,
when present, is listed first); otherwise the last address. Skip link-local (`169.254/16`,
`fe80::/10`). Use `net/netip`. Table test with the fixture above, a VIP case
(`[vip, ip, ipv6]`), an IPv6-only node, and an empty list.

## Milestone 2: Enter on a node opens containers

`enter` on the node list opens the containers view (was services). `s` still opens services.
Update hints, help, README key table, and any test that relies on Enter → services.

## Milestone 3: k9s-style logs

1. **Indicator line** directly under the logs title
   (`Logs: apid on vanilla-talos-controlplane-1 [streaming]`):
   `Autoscroll:On     FullScreen:Off     Timestamps:Off     Wrap:Off`
   Label dim, `On` green, `Off` dim, fixed column spacing like k9s. (k9s also shows `ColumnLock`;
   t9s has no horizontal scrolling, so leave it out.)
2. Keys (match k9s; check they do not clash with the existing logs keys, e.g. `/` find, `n`/`N`):
   - `s` Autoscroll: on = follow the tail as lines arrive; off = freeze the viewport where it is
     while lines keep buffering. Show `+N new` in the indicator line while frozen. Scrolling up
     (`k`, `up`, `pgup`, `g`) turns autoscroll off automatically; `G`/`end` turns it back on (k9s).
   - `f` FullScreen: hide the header, hints and footer; logs use the whole terminal.
   - `t` Timestamps: prefix each line with `YYYY-MM-DDTHH:MM:SS.sss` in dim grey. The logs API
     has no per-line timestamp (`talos logs` only has follow/tail/namespace flags; confirm in the
     Talos source under `cmd/`, Glob `**/talos/logs.go`). So: first parse a timestamp already in
     the line (RFC 3339 / `2006/01/02 15:04:05` / klog `I1003 12:00:00.123456` / JSON `ts` or
     `time` fields, the formats Talos services emit) and normalise it to the prefix format;
     otherwise use the time the line arrived, marked with a leading `~`. Store the arrival time
     per line when it arrives, not at render time.
   - `w` Wrap: soft-wrap long lines (today they are cut). The cursor and find must still work
     on logical lines.
3. **Colour**: colour the level token itself, not the whole line: `ERROR`/`FATAL`/`CRIT`/`error`
   red bold, `WARN`/`WARNING`/`warn` yellow, `INFO`/`info` blue, `DEBUG`/`TRACE` dim. Recognise
   `level=info`, `"level":"info"`, `[INFO]`, klog prefixes `E1003`/`W1003`/`I1003`, and the bare
   words. Also dim the timestamp already embedded in a line and colour `key=` names cyan in
   logfmt lines. Keep the existing whole-line dimming for DEBUG. The find highlight must stay
   visible over the colours.
4. Tests: indicator text for each state, autoscroll freeze + `+N new` + `G` resume, timestamp
   parsing table (each format + fallback `~`), wrap keeps line count right, level colouring
   (strip ANSI and compare positions), full screen height budget.

## Milestone 4: container detail on Enter

`enter` on a row in the containers view opens a **container detail** view (new
`StateContainerDetail`, `internal/ui/containerdetail.go`):

- Header block (lipgloss rounded box): container ID; for CRI containers parse the ID into pod
  namespace / pod name / container name (check the ID format the containers command prints, in the
  Talos source, and in existing fixtures); image (full ref; split registry / repo / tag / digest,
  tag bold); PID; status (coloured as `colorStatus`); containerd namespace.
- Resources: CPU and memory for this container from the stats call (match by ID; `stats` uses the
  same namespace flag as containers; see the talosctl contract table in the t9s SKILL).
  Show memory as a lipgloss bar against the node's total memory if known, else plain.
- Processes: the rows of the processes call whose PID is the container PID or a descendant
  (if parent PIDs are not available, just the matching PID row; say so in the report).
- Recent logs: the last 30 lines (`logs` with the CRI namespace and `--tail 30`, not following),
  using the new level colouring.
- Keys: `l` opens the full streaming logs view for this container (k9s `l`), `r` refresh,
  `esc`/`q` back to the containers list with the cursor kept. Loads run concurrently, each with a
  10 s timeout, each section showing its own loading/error line.
- Tests: ID parsing table, render budgets at 80×24 and 120×40, `l` and `esc` transitions.

## Milestone 5: docs

README key tables (node `enter`, logs `s/f/t/w`, container detail), hints and the help overlay.

## Done when

1. All milestones committed, `go build ./... && go vet ./... && go test ./...` green.
2. A short report to Andrew in this pane: what works, what was not verified live, the exact
   commands he should run and paste if a format (container IDs, stats, embedded log timestamps)
   needs confirming, and any deviation from this plan with the reason.
