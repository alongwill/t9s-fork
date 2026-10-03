package diskmodel

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/florianspk/t9s/internal/netmodel"
)

// Inputs is everything Build reads, already fetched.
type Inputs struct {
	Node        string
	Disks       []netmodel.Res
	SystemDisks []netmodel.Res
	Discovered  []netmodel.Res
	Volumes     []netmodel.Res
	Usage       map[string]Usage // mount point -> usage; nil when it could not be read
}

// gapMin is the smallest unallocated gap worth showing: GPT alignment leaves
// up to 1 MiB at the start and end of every disk.
const gapMin = 1 << 20

// --- generic map accessors ---

func str(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return v
	case int, int64, uint64, float64, bool:
		return fmt.Sprint(v)
	}
	return ""
}

func u64(m map[string]any, key string) uint64 {
	switch v := m[key].(type) {
	case int:
		if v > 0 {
			return uint64(v)
		}
	case int64:
		if v > 0 {
			return uint64(v)
		}
	case uint64:
		return v
	case float64:
		if v > 0 {
			return uint64(v)
		}
	case string:
		n, _ := strconv.ParseUint(v, 10, 64)
		return n
	}
	return 0
}

func boolOf(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

func sub(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func strList(m map[string]any, key string) []string {
	var out []string
	if l, ok := m[key].([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// --- classification ---

func diskType(transport string, rotational, cdrom bool) string {
	switch {
	case cdrom:
		return "CD"
	case transport == "nvme":
		return "NVMe"
	case transport == "usb":
		return "USB"
	case transport == "virtio":
		return "virtio"
	case rotational:
		return "HDD"
	case transport == "":
		return "disk"
	}
	return "SSD"
}

func isJunkDev(dev string) bool {
	base := path.Base(dev)
	for _, p := range []string{"loop", "zram", "ram", "sr", "fd", "nbd"} {
		if strings.HasPrefix(base, p) {
			return true
		}
	}
	return false
}

func isMapperDev(dev, kind string) bool {
	base := path.Base(dev)
	return kind != "" || strings.HasPrefix(base, "dm-") || strings.HasPrefix(base, "md") && len(base) > 2 && base[2] >= '0' && base[2] <= '9'
}

// tableNames are the "filesystem" names a partition table probes as.
var tableNames = map[string]bool{"": true, "gpt": true, "dos": true, "mbr": true, "pmbr": true}

// roleOf decides what a partition is for, from the volume ID when Talos has a
// VolumeStatus for it, else from the partition label and the probed filesystem.
func roleOf(volID, partLabel, fs string) Role {
	id := volID
	if id == "" {
		id = partLabel
	}
	switch strings.ToUpper(id) {
	case "EFI":
		return RoleEFI
	case "BIOS":
		return RoleBIOS
	case "BOOT":
		return RoleBoot
	case "META":
		return RoleMeta
	case "STATE":
		return RoleState
	case "EPHEMERAL":
		return RoleEphemeral
	case "IMAGECACHE":
		return RoleImage
	}
	switch {
	case strings.HasPrefix(id, "u-"):
		return RoleUser
	case strings.HasPrefix(id, "e-"):
		return RoleExisting
	case strings.HasPrefix(id, "r-"):
		return RoleRaw
	case strings.HasPrefix(id, "s-"), fs == "swap":
		return RoleSwap
	case fs == "lvm2-pv", fs == "LVM2_member":
		return RoleLVM
	case fs == "linux_raid_member", fs == "mdraid":
		return RoleRAID
	}
	return RoleOther
}

// Build joins the resources. It never fails: what it cannot place is left out.
func Build(in Inputs) Model {
	m := Model{Node: in.Node, UsageKnown: in.Usage != nil}

	sysDev, sysID := "", ""
	for _, r := range in.SystemDisks {
		sysDev, sysID = str(r.Spec, "devPath"), str(r.Spec, "diskID")
	}

	type dv struct {
		res netmodel.Res
		dev string
	}
	var discovered []dv
	for _, r := range in.Discovered {
		discovered = append(discovered, dv{r, str(r.Spec, "dev_path")})
	}
	volByLoc := map[string]netmodel.Res{}
	for _, r := range in.Volumes {
		t := str(r.Spec, "type")
		if t != "partition" && t != "disk" {
			continue
		}
		if loc := str(r.Spec, "location"); loc != "" {
			volByLoc[loc] = r
		}
	}
	usedVol := map[string]bool{}

	var mappers []struct {
		Mapper
		backing []string
	}

	for _, r := range in.Disks {
		s := r.Spec
		dev := str(s, "dev_path")
		if dev == "" {
			dev = "/dev/" + r.ID
		}
		if isMapperDev(dev, str(s, "device_mapper_kind")) {
			mappers = append(mappers, struct {
				Mapper
				backing []string
			}{Mapper{Dev: dev, Kind: str(s, "device_mapper_kind"), Size: u64(s, "size")}, strList(s, "secondary_disks")})
			continue
		}
		d := Disk{
			ID: r.ID, DevPath: dev, Model: str(s, "model"), Serial: str(s, "serial"), Transport: str(s, "transport"),
			Size: u64(s, "size"), Ref: Ref{Type: TypeDisk, Namespace: r.Namespace, ID: r.ID},
		}
		d.Type = diskType(d.Transport, boolOf(s, "rotational"), boolOf(s, "cdrom"))
		d.Hidden = boolOf(s, "cdrom") || boolOf(s, "readonly") || isJunkDev(dev)
		d.System = (sysDev != "" && sysDev == dev) || (sysID != "" && sysID == r.ID)

		// partitions, by offset
		var parts, whole []dv
		for _, x := range discovered {
			switch str(x.res.Spec, "type") {
			case "partition":
				if str(x.res.Spec, "parent_dev_path") == dev {
					parts = append(parts, x)
				}
			case "disk":
				if x.dev == dev && !tableNames[str(x.res.Spec, "name")] {
					whole = append(whole, x)
				}
			}
		}
		sort.SliceStable(parts, func(i, j int) bool {
			return u64(parts[i].res.Spec, "offset") < u64(parts[j].res.Spec, "offset")
		})
		mk := func(x dv, isWhole bool) Segment {
			xs := x.res.Spec
			seg := Segment{
				Index: int(u64(xs, "partition_index")), DevPath: x.dev, Offset: u64(xs, "offset"), Size: u64(xs, "size"),
				FS: str(xs, "name"), Whole: isWhole,
				Discovered: &Ref{Type: TypeDiscovered, Namespace: x.res.Namespace, ID: x.res.ID},
			}
			if isWhole {
				seg.Offset, seg.Size = 0, d.Size
				if u64(xs, "size") > 0 {
					seg.Size = u64(xs, "size")
				}
			}
			partLabel := str(xs, "partition_label")
			vs, ok := volByLoc[x.dev]
			if !ok && seg.Index > 0 {
				for _, v := range in.Volumes {
					if str(v.Spec, "parentLocation") == dev && int(u64(v.Spec, "partitionIndex")) == seg.Index {
						vs, ok = v, true
					}
				}
			}
			volID := ""
			if ok {
				usedVol[vs.ID] = true
				volID = vs.ID
				vsp := vs.Spec
				seg.Volume = &Volume{
					ID: vs.ID, Type: str(vsp, "type"), Phase: str(vsp, "phase"), Error: str(vsp, "errorMessage"),
					Location: str(vsp, "location"), Ref: Ref{Type: TypeVolume, Namespace: vs.Namespace, ID: vs.ID},
				}
				seg.Mount = str(vsp, "mountLocation")
				if seg.Mount == "" {
					seg.Mount = str(sub(vsp, "mountSpec"), "targetPath")
				}
				seg.Volume.Mount = seg.Mount
				if p := str(vsp, "encryptionProvider"); p != "" && p != "none" {
					seg.Encrypted, seg.Provider = true, strings.ToUpper(p)
				}
				if f := str(vsp, "filesystem"); f != "" && f != "none" {
					seg.FS = f
				}
			}
			if seg.FS == "luks2" || seg.FS == "luks" {
				seg.Encrypted = true
				if seg.Provider == "" {
					seg.Provider = strings.ToUpper(seg.FS)
				}
			}
			seg.Role = roleOf(volID, partLabel, str(xs, "name"))
			switch {
			case volID != "":
				seg.Label = volID
			case partLabel != "":
				seg.Label = partLabel
			case str(xs, "label") != "":
				seg.Label = str(xs, "label")
			case seg.FS != "":
				seg.Label = seg.FS
			default:
				seg.Label = "partition " + strconv.Itoa(seg.Index)
			}
			if seg.Mount != "" && in.Usage != nil {
				if u, ok := in.Usage[seg.Mount]; ok && u.Size > 0 {
					uu := u
					seg.Usage = &uu
				}
			}
			return seg
		}
		for _, x := range parts {
			d.Segments = append(d.Segments, mk(x, false))
		}
		if len(parts) == 0 && len(whole) > 0 {
			d.Segments = append(d.Segments, mk(whole[0], true))
		}

		// unallocated gaps
		var withFree []Segment
		cursor := uint64(0)
		for _, sg := range d.Segments {
			if sg.Offset > cursor+gapMin {
				withFree = append(withFree, freeSeg(cursor, sg.Offset-cursor))
				d.Free += sg.Offset - cursor
			}
			withFree = append(withFree, sg)
			if end := sg.Offset + sg.Size; end > cursor {
				cursor = end
			}
		}
		if d.Size > cursor+gapMin {
			withFree = append(withFree, freeSeg(cursor, d.Size-cursor))
			d.Free += d.Size - cursor
		}
		d.Segments = withFree
		d.Unused = len(parts) == 0 && len(whole) == 0
		m.Disks = append(m.Disks, d)
	}

	// volumes that name a disk but have no partition yet
	for _, v := range in.Volumes {
		t := str(v.Spec, "type")
		if (t != "partition" && t != "disk") || usedVol[v.ID] {
			continue
		}
		parent := str(v.Spec, "parentLocation")
		phase := str(v.Spec, "phase")
		for i := range m.Disks {
			if m.Disks[i].DevPath == parent && phase != "ready" {
				m.Disks[i].Pending = append(m.Disks[i].Pending, Volume{
					ID: v.ID, Type: t, Phase: phase, Error: str(v.Spec, "errorMessage"),
					Ref: Ref{Type: TypeVolume, Namespace: v.Namespace, ID: v.ID},
				})
				m.Disks[i].Unused = false
			}
		}
	}

	// device-mapper and md devices hang under the disk they are built on
	for _, mp := range mappers {
		mp.Backing = strings.Join(mp.backing, ", ")
		for _, x := range discovered {
			if x.dev == mp.Dev {
				mp.FS = str(x.res.Spec, "name")
			}
		}
		attached := false
		for i := range m.Disks {
			if backedBy(m.Disks[i], mp.backing) {
				m.Disks[i].Mappers = append(m.Disks[i].Mappers, mp.Mapper)
				attached = true
				break
			}
		}
		if !attached {
			m.Orphans = append(m.Orphans, mp.Mapper)
		}
	}

	sort.SliceStable(m.Disks, func(i, j int) bool {
		a, b := m.Disks[i], m.Disks[j]
		if a.System != b.System {
			return a.System
		}
		return a.ID < b.ID
	})
	return m
}

func freeSeg(offset, size uint64) Segment {
	return Segment{Role: RoleFree, Label: "unallocated", Offset: offset, Size: size}
}

// backedBy reports whether a mapper's backing devices name this disk or one of
// its partitions (by device path or base name).
func backedBy(d Disk, backing []string) bool {
	for _, b := range backing {
		bb := path.Base(b)
		if bb == path.Base(d.DevPath) {
			return true
		}
		for _, s := range d.Segments {
			if s.DevPath != "" && path.Base(s.DevPath) == bb {
				return true
			}
		}
	}
	return false
}
