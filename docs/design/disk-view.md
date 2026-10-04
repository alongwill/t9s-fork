# Design: graphical disk view

Status: implemented on `feat/disk-view` (see "As built" at the end). Replaces the body of the existing t9s Disks view (`i` on a node).

## Goal

Show, for one node, every physical disk as a horizontal bar split into its partitions, sized in
proportion, coloured by what Talos uses each one for, with usage inside each filesystem. A user
should see at a glance: which disk Talos is installed on, where EPHEMERAL and user volumes live,
what is encrypted, and how much space is free or unallocated.

## Mock (120 columns)

```
 Disks  node: cp-1 [CP]   3 disks · 1 system · 2.1 TiB raw · 1.2 TiB free          src: grpc
╭ sda ★ system · 120 GiB · Samsung 870 EVO · SSD · sata ────────────────────────────────────────╮
│ █EFI█▕B▕██BOOT██▕M▕STATE▕▓▓▓▓▓▓▓▓▓▓▓▓▓ EPHEMERAL xfs 61% ▓▓▓▓▓▓▓▓░░░░░░░░░░░░░░░░░▕ free │
│ 100M  1M  1.0G   1M 100M                   118.8 GiB                                   12M │
╰────────────────────────────────────────────────────────────────────────────────────────────────╯
╭ nvme0n1 · 1.8 TiB · WD SN850 · nvme ───────────────────────────────────────────────────────────╮
│ ▓▓▓ u-data xfs  LUKS2 lock  12% ▓▓░░░░░░░░░░░░░░░░▕· · · · · unallocated 900 GiB · · · · · · · │
╰────────────────────────────────────────────────────────────────────────────────────────────────╯
╭ sdb · 500 GiB · virtio · HDD ──────────────────────────────────────────────────────────────────╮
│ · · · · · · · · · · · · · · · · · ·  not used by Talos  · · · · · · · · · · · · · · · · · · · ·│
╰────────────────────────────────────────────────────────────────────────────────────────────────╯
 sda › EPHEMERAL
 #  LABEL       VOLUME      FS    SIZE       OFFSET    MOUNT              USED     PHASE  ENC
 6  EPHEMERAL   EPHEMERAL   xfs   118.8 GiB  1.2 GiB   /var               72 GiB   ready  -
 ↑↓ disk  ←→ partition  ↵ YAML  d describe  p related  a show all devices  u units  esc back
```

- `▓` = used part of a filesystem, `░` = free inside it, `·` = unallocated space on the disk.
- Each segment has a minimum width of 3 cells so 1 MiB partitions (BIOS, META) stay visible;
  the rest of the width is shared in proportion to size. Labels go inside the segment when they
  fit, otherwise on the size line underneath, otherwise only in the table.
- The selected segment gets a bright border colour (underline row + bold label); the table below
  always describes the selected segment.

## Colours (lipgloss `AdaptiveColor`, background-filled segments)

| What | Colour |
|---|---|
| Talos system partitions: EFI, BIOS, BOOT (A/B), META, STATE | slate / blue-grey shades |
| EPHEMERAL | green |
| User volumes (`u-*`), existing volumes (`e-*`) | one palette colour per volume name, stable hash |
| Raw volumes (`r-*`), swap | orange |
| LVM / RAID members | purple, with a `→ vg0` / `→ md0` suffix |
| Encrypted | small red `lock` badge after the label |
| Unallocated | dim `·` pattern, no background |
| Failed / missing volume (`VolumeStatus.phase` not `ready`) | red label, phase shown in the table |

Category accent for the border = the Block accent from the resource browser (PR B), so the view
matches the browser.

## Data (one node, through the browser's `ResourceSource`, so gRPC or CLI)

| Need | Resource | Fields (verify the yaml tags in `pkg/machinery/resources/block/`) |
|---|---|---|
| Disks | `Disks.block.talos.dev` | dev path, size, model, serial, transport, rotational, readonly, cdrom |
| Which disk is the install disk | `SystemDisks.block.talos.dev` | dev path |
| Partitions and whole-disk filesystems | `DiscoveredVolumes.block.talos.dev` | dev path, type (disk/partition), parent dev path, offset, size, filesystem name, label, partition label/index/type/UUID |
| What Talos does with each | `VolumeStatuses.block.talos.dev` | id (EPHEMERAL, `u-data`, …), type, phase, location, mount location, filesystem, encryption provider, size |
| Used / free inside a filesystem | existing `GetVolumeStatus` (mounts usage) | used, available |
| LVM / RAID (later) | `storage.talos.dev` resources | members → array/VG mapping |

Joining: `DiscoveredVolume.parent_dev_path` → disk; `VolumeStatus.location` = partition dev
path → volume id. Unallocated = gaps between `offset+size` of consecutive partitions and the disk
size (sort by offset). Device-mapper and md devices appear as discovered volumes of their own; show
them as an extra line under their parent disk ("dm-0 · LUKS2 · → u-data") rather than as a disk.

## Keys (k9s-consistent)

`↑/↓ j/k` disk, `←/→ h/l` partition, `enter` YAML of the selected `DiscoveredVolume` (or
`VolumeStatus` when it has one) via the browser's jump, `d` describe (notes: what is EPHEMERAL,
STATE, META…, with the Ubuntu equivalent such as `/var` on its own LV), `p` related view (PR B),
`a` show all devices (loop, cdrom, zram are hidden by default), `u` toggle GiB/GB, `ctrl+r`
refresh, `esc`/`q` back.

## Edge cases

- Disk with a filesystem and no partition table: one full-width segment.
- Disk Talos does not use: dim "not used by Talos" bar, still listed (people look for it).
- Read-only/CD-ROM/loop/zram: hidden unless `a`.
- Tiny terminal (< 80 columns): drop the size line, keep the bar and the table.
- CLI source: same data, just slower; no watch needed.
- v1.15 `diskfree` gives usage for more volumes; use it when present (version-gated), else mounts.

## Size

About 1.5–2 days for a Sonnet agent: 0.5 day data join + tests (fixtures for a typical
single-disk control plane, a two-disk worker with an encrypted user volume, and a whole-disk
filesystem), 1 day rendering + keys, 0.5 day docs and render tests.

## Decisions (Andrew, 2026-10-03)

1. **Replace** the current Disks view; the table under the bars keeps today's information.
2. **LVM / RAID later.** First version shows dm/md devices only as a line under their parent disk.

Queued after learning PR D.

## As built

- Model in `internal/diskmodel`, view in `internal/ui/diskview*.go`, a `paneDisks` on the browser stack (like the
  network view), so `StateDisks`, `disks.go`, `GetDisks` and `GetVolumeStatus` are removed. `i` and `:disks` open it.
- All block resources are read through the `ResourceSource` (gRPC or CLI), namespace `runtime`.
- **Usage is not a resource.** The design said "existing `GetVolumeStatus`", but that never carried usage. Usage
  comes from the `mounts` table (`Client.GetMounts`, one subprocess call, decimal GB with two decimals), matched to a
  volume by `VolumeStatus.mountLocation` (else `mountSpec.targetPath`). v1.15 `diskfree` is not used yet.
- Checked against real output from a QEMU control plane (Talos 1.14, `get disks`, `get volumestatus`, `mounts`):
  `mountLocation` is the **device** (`/dev/vda4`) and the mount point is `mountSpec.targetPath` (`/var`); directory,
  overlay and symlink volumes have no `location` and are not partitions; the mounts table names `/dev/vda4` for
  `/var`, so usage is matched by mount point, then by device. Fixture `qemu-vm` follows that output.
- Not verified against a real node: the `secondary_disks` format on device-mapper disks and the `type` of a
  device-mapper `DiscoveredVolume`. Those fixtures are hand-written.
- **Labels are not drawn on the bar.** Text over narrow partitions (STATE, META) was unreadable, so the bar is fill
  glyphs only and a legend under it lists `● name fs size` per segment (Andrew, 2026-10-04). The mock above predates this.
- Not built: LVM / RAID mapping beyond the `↳` line (as decided), `diskfree`.
