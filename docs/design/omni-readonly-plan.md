# Plan: read-only by default, Omni detection, role badge, padlocks

Branch `feat/omni-readonly`, off `main`, worktree
`/Users/andrewlongwill/sources/github.com/alongwill/worktrees/t9s-fork-resource-browser`.

**Another agent works in parallel** on log filtering, dmesg and container metrics
(`internal/ui/logs.go`, `dmesg.go`, `metrics.go`, a shared log pane). Do **not** edit those files.
You own the top bar/header code in `app.go`, `keyrouter.go`, `internal/config/`, `cmd/main.go`,
and the browser files.

## Read first

1. `docs/design/resource-browser-phase1-plan.md` **Hard rules** (commits not GPG-signed now).
   Never run or name the Talos/Kubernetes/Omni CLIs in a Bash command (a hook blocks it, even in
   `grep` or a path); use Read/Grep/Glob. Chain checks with `&&`. Do not push.
2. `/Users/andrewlongwill/sources/github.com/TEMPORARY/t9s-fork/.claude/SKILL.md`,
   `/Users/andrewlongwill/sources/github.com/TEMPORARY/k9s/.claude/SKILL.md` (§1 `Dangerous`
   actions, §4 "Read-only mode": k9s strips dangerous keys with `ClearDanger()`).
3. Code: `internal/config/config.go` (talosconfig `Context`), `internal/ui/app.go` (header),
   `keyrouter.go`, `nodelist.go`, `machineconfig.go`, `upgrade.go`, `extcatalog.go`,
   `services.go`, `internal/talos/client.go` (mutating calls: `Reboot`, `Shutdown`,
   `ApplyConfig`, `UpgradeTalos`, `UpgradeK8s`, and any service/container restart or
   extension install), `internal/talos/source.go`, browser lock rendering.

## Facts already checked

- talosconfig: a context authenticates either with a client cert (`ca`/`crt`/`key`) or with Omni's
  `auth.siderov1` block (`pkg/machinery/client/config/config.go:69`, `SideroV1{identity, …}`).
- Static talosconfig role: the client cert's subject **Organization** holds the Talos roles
  (`os:admin`, `os:operator`, `os:reader`, `os:etcd:backup`, …; see
  `~/sources/github.com/siderolabs/talos/pkg/machinery/role/role.go`).
- Omni: the Omni proxy picks the Talos role **per call** from the Omni user's role and the API
  method: reader by default, operator for its `operatorMethodSet`, admin only for
  `adminMethodSet`, and never sensitive resource reads (`MachineConfig`, secrets) whatever the
  role (`~/sources/github.com/alongwill/omni/internal/backend/grpc/router/talos_backend.go`,
  `sensitive_read_guard.go`). Read those two files before writing the Omni behaviour.

## Milestone 1: read-only by default

1. `--readonly` flag, default **true**; `--write` (or `--readonly=false`) turns writes on.
   Also honour `T9S_READONLY=false`. Print nothing extra on startup.
2. One guard: a `dangerous` flag on every mutating action (reboot, shutdown, upgrade Talos,
   upgrade Kubernetes, apply/edit machine config, service restart/start/stop, extension
   install/remove, anything else that calls a mutating client method; grep the client for every
   non-read call and list them in the report). In read-only mode:
   - the keys are not shown in hints or help (k9s `ClearDanger`);
   - pressing one shows `read-only mode: start t9s with --write to <action>` and does nothing;
   - the client itself refuses mutating calls (`ErrReadOnly`) as a second safety net, so a bug in
     the UI cannot change a cluster. Test that every mutating client method returns `ErrReadOnly`
     when the client is read-only, and that no subprocess is started.
3. Top bar badge: `RO` (green background) in read-only, `RW` (red background, bold) in write mode.
4. Tests: key hidden + message in RO; works in RW; client-level refusal.

## Milestone 2: Omni detection and indicator

1. Detect Omni, in order: (a) the selected talosconfig context has `auth.siderov1`; (b) its
   endpoints are an Omni URL (contains `omni` and is HTTPS, record the host); (c) fallback, from
   the cluster: the node has SideroLink configured (`SiderolinkConfig` resource or
   `SiderolinkStatuses.siderolink.talos.dev` present, Grep the Talos source for the type names).
   Do (a)/(b) at startup with no network call; (c) lazily after the node list loads.
2. Top bar: `Omni <host>` badge (purple) when detected, `Talos` (plain) otherwise. Show where the
   detection came from in the help overlay's status section (`detected via talosconfig siderov1`).
3. Tests for each detection path with fixture talosconfigs.

## Milestone 3: current role in the top bar

- Cert contexts: parse `crt` (base64 PEM) with `crypto/x509`, read `Subject.Organization`, show the
  roles as a badge: `os:admin` red, `os:operator` yellow, `os:reader` green, other roles dim. Cert
  expiry within 7 days: append `cert expires in Nd` in yellow.
- Omni contexts: show `role: via Omni` and, in the help overlay, explain that Omni maps the Omni
  user's role to a Talos role per call (reader by default, operator/admin for specific methods,
  sensitive reads always denied), with the file references above.
- Tests: cert parsing with a generated test cert (`crypto/x509` + `ecdsa` in the test, no
  checked-in keys), multiple roles, expiry warning, Omni text.

## Milestone 4: padlocks for locked resources

- Replace the browser's `lock` text with a padlock `🔒` everywhere a read was denied (types pane
  counts, config section `requires os:admin` row, describe, related view cells, compare rows,
  palette). Measure width with `lipgloss.Width` (it is 2 cells) and keep columns aligned; fall back
  to `[locked]` when `T9S_ASCII=1` is set.
- The status message on such a row explains *why*: on Omni, `Omni does not forward reads of
  sensitive resources such as MachineConfig, whatever your role`; elsewhere,
  `needs os:admin (you have os:reader)` using the role from milestone 3.
- The old Machine Config view: on permission denied show a padlock panel with the same text
  instead of an error.
- Tests: padlock rendering and alignment, Omni vs non-Omni message, ASCII fallback.

## Milestone 5: docs

README (read-only default, `--write`, badges, Omni notes), help overlay, `docs/design/skill-md-additions.md`.

## Done when

All milestones committed; `go build ./... && go vet ./... && go test ./...` green; report to
Andrew: what works, not verified live (especially Omni detection against his Omni context), the
full list of actions now marked dangerous, deviations with reasons.
