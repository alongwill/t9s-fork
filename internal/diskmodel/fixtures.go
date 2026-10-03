package diskmodel

import (
	"fmt"
	"strings"

	"github.com/florianspk/t9s/internal/netmodel"
)

// Hand-written inputs for tests, the TUI's render tests and examples. They
// follow the shapes Talos prints for the block resources
// (pkg/machinery/resources/block); they are not captured from a cluster.

const (
	KiB = uint64(1) << 10
	MiB = KiB << 10
	GiB = MiB << 10
	TiB = GiB << 10
)

// FixtureNames lists the fixtures Fixture knows.
func FixtureNames() []string {
	return []string{"single-disk-cp", "worker-encrypted", "whole-disk-fs"}
}

// Fixture returns the named fixture; it panics on a name FixtureNames lacks.
func Fixture(name string) Inputs {
	switch name {
	case "single-disk-cp":
		return fixSingleDisk()
	case "worker-encrypted":
		return fixWorkerEncrypted()
	case "whole-disk-fs":
		return fixWholeDisk()
	}
	panic("unknown diskmodel fixture " + name)
}

func fixRes(id, spec string) netmodel.Res {
	var sb strings.Builder
	for _, l := range strings.Split(strings.Trim(spec, "\n"), "\n") {
		sb.WriteString("  " + l + "\n")
	}
	r, err := netmodel.ParseRes(fmt.Sprintf("metadata:\n  namespace: %s\n  id: %s\nspec:\n%s", NS, id, sb.String()), NS)
	if err != nil {
		panic(err)
	}
	return r
}

func fixDisk(id string, size uint64, model, serial, transport string, rotational bool, extra string) netmodel.Res {
	return fixRes(id, fmt.Sprintf("dev_path: /dev/%s\nsize: %d\nmodel: %s\nserial: %s\ntransport: %s\nrotational: %v\n%s", id, size, model, serial, transport, rotational, extra))
}

// fixTable is the DiscoveredVolume of a disk with a GPT.
func fixTable(id string, size uint64) netmodel.Res {
	return fixRes(id, fmt.Sprintf("dev_path: /dev/%s\ntype: disk\nname: gpt\nsize: %d", id, size))
}

func fixPart(disk string, idx int, off, size uint64, fs, label string) netmodel.Res {
	dev := partDev(disk, idx)
	return fixRes(strings.TrimPrefix(dev, "/dev/"), fmt.Sprintf("dev_path: %s\ntype: partition\nparent: %s\nparent_dev_path: /dev/%s\noffset: %d\nsize: %d\nname: %s\npartition_label: %s\npartition_index: %d",
		dev, disk, disk, off, size, fs, label, idx))
}

func partDev(disk string, idx int) string {
	sep := ""
	if strings.HasPrefix(disk, "nvme") {
		sep = "p"
	}
	return fmt.Sprintf("/dev/%s%s%d", disk, sep, idx)
}

func fixVol(id, typ, phase, disk string, idx int, size uint64, fs, mount, enc string) netmodel.Res {
	loc := partDev(disk, idx)
	if typ == "disk" {
		loc = "/dev/" + disk
	}
	s := fmt.Sprintf("type: %s\nphase: %s\nlocation: %s\nparentLocation: /dev/%s\npartitionIndex: %d\nsize: %d\nfilesystem: %s\n", typ, phase, loc, disk, idx, size, fs)
	if mount != "" {
		s += "mountLocation: " + mount + "\nmountSpec:\n  targetPath: " + mount + "\n"
	}
	if enc != "" {
		s += "encryptionProvider: " + enc + "\n"
	}
	return fixRes(id, s)
}

// systemLayout adds the Talos system partitions to disk, returns the offset where
// EPHEMERAL starts and the resources.
func systemLayout(disk string, diskSize, ephemeralTail uint64) (dvs, vols []netmodel.Res, used uint64) {
	type p struct {
		label, fs, mount string
		size             uint64
	}
	parts := []p{
		{"EFI", "vfat", "/system/efi", 100 * MiB}, {"BIOS", "", "", MiB}, {"BOOT", "xfs", "/boot", 1000 * MiB},
		{"META", "", "", MiB}, {"STATE", "xfs", "/system/state", 100 * MiB},
	}
	off := MiB
	dvs = append(dvs, fixTable(disk, diskSize))
	for i, x := range parts {
		dvs = append(dvs, fixPart(disk, i+1, off, x.size, orNone(x.fs), x.label))
		vols = append(vols, fixVol(x.label, "partition", "ready", disk, i+1, x.size, orNone(x.fs), x.mount, ""))
		off += x.size
	}
	n := len(parts) + 1
	eph := diskSize - off - ephemeralTail
	dvs = append(dvs, fixPart(disk, n, off, eph, "xfs", "EPHEMERAL"))
	vols = append(vols, fixVol("EPHEMERAL", "partition", "ready", disk, n, eph, "xfs", "/var", ""))
	return dvs, vols, off + eph
}

func orNone(fs string) string {
	if fs == "" {
		return "none"
	}
	return fs
}

func fixSingleDisk() Inputs {
	dvs, vols, _ := systemLayout("sda", 120*GiB, 12*MiB)
	return Inputs{
		Node: "10.0.0.5",
		Disks: []netmodel.Res{
			fixDisk("sda", 120*GiB, "Samsung 870 EVO", "S3Z3NX0M123456", "sata", false, ""),
			fixDisk("loop0", 64*MiB, "", "", "", false, "readonly: true"),
			fixDisk("sr0", 2*GiB, "QEMU DVD-ROM", "", "sata", false, "cdrom: true"),
		},
		SystemDisks: []netmodel.Res{fixRes("system-disk", "diskID: sda\ndevPath: /dev/sda")},
		Discovered:  dvs,
		Volumes:     vols,
		Usage:       map[string]Usage{"/var": {Size: 118 * GiB, Used: 72 * GiB, Avail: 46 * GiB}, "/system/state": {Size: 100 * MiB, Used: 12 * MiB, Avail: 88 * MiB}},
	}
}

func fixWorkerEncrypted() Inputs {
	dvs, vols, _ := systemLayout("sda", 120*GiB, 0)
	dvs = append(dvs,
		fixTable("nvme0n1", 1800*GiB),
		fixPart("nvme0n1", 1, MiB, 900*GiB, "luks2", "u-data"),
		fixRes("dm-0", "dev_path: /dev/dm-0\ntype: disk\nname: xfs\nsize: "+fmt.Sprint(899*GiB+512*MiB)))
	vols = append(vols, fixVol("u-data", "partition", "ready", "nvme0n1", 1, 900*GiB, "xfs", "/var/mnt/data", "luks2"))
	vols = append(vols, fixRes("u-scratch", "type: partition\nphase: waiting\nparentLocation: /dev/nvme0n1\nerrorMessage: no free space for a 1TiB partition"))
	return Inputs{
		Node: "10.0.0.6",
		Disks: []netmodel.Res{
			fixDisk("sda", 120*GiB, "Samsung 870 EVO", "S3Z3NX0M123457", "sata", false, ""),
			fixDisk("nvme0n1", 1800*GiB, "WD SN850", "WD-WX11A1234567", "nvme", false, ""),
			fixDisk("sdb", 500*GiB, "WDC WD5000", "WD-0003", "sata", true, ""),
			fixDisk("dm-0", 899*GiB+512*MiB, "", "", "", false, "device_mapper_name: u-data\ndevice_mapper_kind: crypt\nsecondary_disks:\n  - nvme0n1p1"),
		},
		SystemDisks: []netmodel.Res{fixRes("system-disk", "diskID: sda\ndevPath: /dev/sda")},
		Discovered:  dvs,
		Volumes:     vols,
		Usage: map[string]Usage{
			"/var":          {Size: 118 * GiB, Used: 40 * GiB, Avail: 78 * GiB},
			"/var/mnt/data": {Size: 899 * GiB, Used: 108 * GiB, Avail: 791 * GiB},
		},
	}
}

func fixWholeDisk() Inputs {
	dvs, vols, _ := systemLayout("vda", 40*GiB, 0)
	dvs = append(dvs, fixRes("vdb", fmt.Sprintf("dev_path: /dev/vdb\ntype: disk\nname: xfs\nlabel: scratch\nsize: %d", 200*GiB)))
	vols = append(vols, fixRes("u-scratch", fmt.Sprintf("type: disk\nphase: ready\nlocation: /dev/vdb\nparentLocation: /dev/vdb\nsize: %d\nfilesystem: xfs\nmountLocation: /var/mnt/scratch\nmountSpec:\n  targetPath: /var/mnt/scratch", 200*GiB)))
	return Inputs{
		Node: "10.0.0.7",
		Disks: []netmodel.Res{
			fixDisk("vda", 40*GiB, "", "", "virtio", false, ""),
			fixDisk("vdb", 200*GiB, "", "", "virtio", false, ""),
			fixDisk("vdc", 100*GiB, "", "", "virtio", true, ""),
		},
		SystemDisks: []netmodel.Res{fixRes("system-disk", "diskID: vda\ndevPath: /dev/vda")},
		Discovered:  dvs,
		Volumes:     vols,
	}
}
