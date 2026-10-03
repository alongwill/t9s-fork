// Package diskmodel joins a node's block resources (Disks, SystemDisks,
// DiscoveredVolumes, VolumeStatuses) and mount usage into one picture: every
// physical disk split into its partitions, sized in proportion, with what Talos
// uses each for. Build is pure; Fetch gathers the resources through a
// talos.ResourceSource. The TUI disk view draws from Model.
package diskmodel

import "github.com/florianspk/t9s/internal/netmodel"

// Resource types and the namespace they live in.
const (
	NS = "runtime"

	TypeDisk       = "Disks.block.talos.dev"
	TypeSystemDisk = "SystemDisks.block.talos.dev"
	TypeDiscovered = "DiscoveredVolumes.block.talos.dev"
	TypeVolume     = "VolumeStatuses.block.talos.dev"
)

// Ref points at one Talos resource, so the UI can open its YAML.
type Ref = netmodel.Ref

// Role is what Talos uses a partition for. It drives colour.
type Role string

const (
	RoleEFI       Role = "efi"
	RoleBIOS      Role = "bios"
	RoleBoot      Role = "boot"
	RoleMeta      Role = "meta"
	RoleState     Role = "state"
	RoleEphemeral Role = "ephemeral"
	RoleImage     Role = "imagecache"
	RoleUser      Role = "user"     // u-* user volume
	RoleExisting  Role = "existing" // e-* existing volume
	RoleRaw       Role = "raw"      // r-* raw volume
	RoleSwap      Role = "swap"
	RoleLVM       Role = "lvm"
	RoleRAID      Role = "raid"
	RoleOther     Role = "other" // a partition Talos did not make
	RoleFree      Role = "free"  // unallocated space
)

// Usage is the used and free space inside a mounted filesystem, in bytes.
type Usage struct{ Size, Used, Avail uint64 }

// Volume is the VolumeStatus joined to a segment.
type Volume struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Phase    string `json:"phase"`
	Mount    string `json:"mount,omitempty"`
	Error    string `json:"error,omitempty"`
	Ref      Ref    `json:"ref"`
	Location string `json:"location,omitempty"`
}

// Segment is one slice of a disk: a partition, a filesystem on the whole disk,
// or unallocated space.
type Segment struct {
	Role      Role   `json:"role"`
	Label     string `json:"label"`           // volume ID, else partition label, else filesystem
	Index     int    `json:"index,omitempty"` // partition number
	DevPath   string `json:"devPath,omitempty"`
	Offset    uint64 `json:"offset"`
	Size      uint64 `json:"size"`
	FS        string `json:"fs,omitempty"`
	Whole     bool   `json:"whole,omitempty"` // a filesystem on the whole disk, no partition table
	Encrypted bool   `json:"encrypted,omitempty"`
	Provider  string `json:"provider,omitempty"` // LUKS2
	Suffix    string `json:"suffix,omitempty"`   // → vg0 / → md0
	Mount     string `json:"mount,omitempty"`
	Usage     *Usage `json:"usage,omitempty"`

	Volume     *Volume `json:"volume,omitempty"`
	Discovered *Ref    `json:"discovered,omitempty"`
}

// Failed reports a volume that is not ready (failed or missing).
func (s Segment) Failed() bool {
	if s.Volume == nil {
		return false
	}
	return s.Volume.Phase == "failed" || s.Volume.Phase == "missing"
}

// Mapper is a device-mapper or md device built on a disk (LUKS, LVM, RAID).
type Mapper struct {
	Dev     string `json:"dev"`
	Kind    string `json:"kind,omitempty"`
	Size    uint64 `json:"size"`
	FS      string `json:"fs,omitempty"`
	Backing string `json:"backing,omitempty"`
}

// Disk is one block device with its segments in offset order.
type Disk struct {
	ID        string    `json:"id"`
	DevPath   string    `json:"devPath"`
	Model     string    `json:"model,omitempty"`
	Serial    string    `json:"serial,omitempty"`
	Transport string    `json:"transport,omitempty"`
	Type      string    `json:"type"` // SSD, HDD, NVMe, USB, virtio, CD
	Size      uint64    `json:"size"`
	System    bool      `json:"system,omitempty"` // Talos is installed here
	Hidden    bool      `json:"hidden,omitempty"` // loop, zram, cdrom, read-only: shown only with `a`
	Unused    bool      `json:"unused,omitempty"` // Talos does not use it
	Free      uint64    `json:"free"`             // unallocated bytes
	Segments  []Segment `json:"segments"`
	Mappers   []Mapper  `json:"mappers,omitempty"`
	Pending   []Volume  `json:"pending,omitempty"` // volumes waiting for space on this disk
	Ref       Ref       `json:"ref"`
}

// Model is the whole picture for one node.
type Model struct {
	Node       string   `json:"node"`
	Disks      []Disk   `json:"disks"`
	UsageKnown bool     `json:"usageKnown"`        // mount usage was available
	Orphans    []Mapper `json:"orphans,omitempty"` // mapper devices with no known backing disk
}

// Shown returns the disks to draw: hidden ones only when all is set.
func (m Model) Shown(all bool) []Disk {
	var out []Disk
	for _, d := range m.Disks {
		if all || !d.Hidden {
			out = append(out, d)
		}
	}
	return out
}

// Totals sums the shown disks.
func Totals(ds []Disk) (n, system int, raw, free uint64) {
	for _, d := range ds {
		n++
		if d.System {
			system++
		}
		raw += d.Size
		free += d.Free
	}
	return
}
