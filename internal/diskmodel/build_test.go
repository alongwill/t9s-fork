package diskmodel

import (
	"strings"
	"testing"
)

func disk(t *testing.T, m Model, id string) Disk {
	t.Helper()
	for _, d := range m.Disks {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no disk %q in %+v", id, m.Disks)
	return Disk{}
}

func labels(d Disk) string {
	var out []string
	for _, s := range d.Segments {
		out = append(out, s.Label)
	}
	return strings.Join(out, ",")
}

func TestSingleDiskControlPlane(t *testing.T) {
	m := Build(Fixture("single-disk-cp"))
	if len(m.Disks) != 3 || len(m.Shown(false)) != 1 || len(m.Shown(true)) != 3 {
		t.Fatalf("disks %d, shown %d, all %d: loop and cdrom are hidden unless a", len(m.Disks), len(m.Shown(false)), len(m.Shown(true)))
	}
	sda := disk(t, m, "sda")
	if !sda.System || sda.Type != "SSD" || sda.Model != "Samsung 870 EVO" || sda.Hidden || sda.Unused {
		t.Fatalf("sda = %+v", sda)
	}
	if got := labels(sda); got != "EFI,BIOS,BOOT,META,STATE,EPHEMERAL,unallocated" {
		t.Fatalf("segments = %s", got)
	}
	// 12 MiB left at the end is a gap, shown as unallocated
	if sda.Free != 12*MiB {
		t.Errorf("free = %d, want 12 MiB", sda.Free)
	}
	last := sda.Segments[len(sda.Segments)-1]
	if last.Role != RoleFree || last.Size != 12*MiB {
		t.Errorf("last segment = %+v, want the unallocated tail", last)
	}
	eph := sda.Segments[len(sda.Segments)-2]
	if eph.Role != RoleEphemeral || eph.FS != "xfs" || eph.Mount != "/var" || eph.Volume == nil || eph.Volume.Phase != "ready" {
		t.Errorf("EPHEMERAL = %+v", eph)
	}
	if eph.Usage == nil || eph.Usage.Used != 72*GiB {
		t.Errorf("EPHEMERAL usage = %+v", eph.Usage)
	}
	if !m.UsageKnown {
		t.Error("usage was given")
	}
	roles := map[string]Role{"EFI": RoleEFI, "BIOS": RoleBIOS, "BOOT": RoleBoot, "META": RoleMeta, "STATE": RoleState}
	for _, s := range sda.Segments {
		if want, ok := roles[s.Label]; ok && s.Role != want {
			t.Errorf("%s role = %s, want %s", s.Label, s.Role, want)
		}
	}
	// the 1 MiB at the start of the disk is alignment, not a gap
	if sda.Segments[0].Role == RoleFree {
		t.Error("the leading 1 MiB must not show as unallocated")
	}
	// partitions are in offset order and do not overlap
	var end uint64
	for _, s := range sda.Segments {
		if s.Offset < end {
			t.Errorf("segment %s starts at %d before the previous ends at %d", s.Label, s.Offset, end)
		}
		end = s.Offset + s.Size
	}
}

func TestWorkerWithEncryptedUserVolume(t *testing.T) {
	m := Build(Fixture("worker-encrypted"))
	if got := len(m.Disks); got != 3 {
		t.Fatalf("%d disks, want sda, nvme0n1, sdb (dm-0 is a mapper, not a disk)", got)
	}
	if m.Disks[0].ID != "sda" {
		t.Errorf("the system disk sorts first, got %s", m.Disks[0].ID)
	}
	nv := disk(t, m, "nvme0n1")
	if nv.System || nv.Type != "NVMe" {
		t.Fatalf("nvme = %+v", nv)
	}
	if got := labels(nv); got != "u-data,unallocated" {
		t.Fatalf("nvme segments = %s", got)
	}
	u := nv.Segments[0]
	if u.Role != RoleUser || !u.Encrypted || u.Provider != "LUKS2" || u.Mount != "/var/mnt/data" || u.FS != "xfs" {
		t.Errorf("u-data = %+v", u)
	}
	if u.Usage == nil || u.Usage.Used != 108*GiB {
		t.Errorf("u-data usage = %+v", u.Usage)
	}
	if nv.Free != 900*GiB-MiB && nv.Free != 899*GiB+1023*MiB {
		t.Errorf("free = %d GiB", nv.Free/GiB)
	}
	// the dm device hangs under its backing disk
	if len(nv.Mappers) != 1 || nv.Mappers[0].Dev != "/dev/dm-0" || nv.Mappers[0].Kind != "crypt" || nv.Mappers[0].FS != "xfs" {
		t.Errorf("mappers = %+v", nv.Mappers)
	}
	if len(m.Orphans) != 0 {
		t.Errorf("orphans = %+v", m.Orphans)
	}
	// a volume waiting for space is listed on its disk, not dropped
	if len(nv.Pending) != 1 || nv.Pending[0].ID != "u-scratch" || nv.Pending[0].Phase != "waiting" || !strings.Contains(nv.Pending[0].Error, "no free space") {
		t.Errorf("pending = %+v", nv.Pending)
	}
	// a disk Talos does not use is still listed
	sdb := disk(t, m, "sdb")
	if !sdb.Unused || sdb.Type != "HDD" || len(sdb.Segments) != 1 || sdb.Segments[0].Role != RoleFree || sdb.Segments[0].Size != 500*GiB {
		t.Errorf("sdb = %+v", sdb)
	}
	n, sys, raw, free := Totals(m.Shown(false))
	if n != 3 || sys != 1 || raw != 120*GiB+1800*GiB+500*GiB || free < 1399*GiB {
		t.Errorf("totals = %d %d %d %d", n, sys, raw/GiB, free/GiB)
	}
}

func TestWholeDiskFilesystem(t *testing.T) {
	m := Build(Fixture("whole-disk-fs"))
	vdb := disk(t, m, "vdb")
	if len(vdb.Segments) != 1 {
		t.Fatalf("vdb segments = %+v", vdb.Segments)
	}
	s := vdb.Segments[0]
	if !s.Whole || s.FS != "xfs" || s.Label != "u-scratch" || s.Role != RoleUser || s.Size != 200*GiB || s.Volume == nil || s.Volume.Phase != "ready" {
		t.Errorf("whole-disk segment = %+v", s)
	}
	if vdb.Unused || vdb.Free != 0 {
		t.Errorf("vdb unused=%v free=%d", vdb.Unused, vdb.Free)
	}
	if vdc := disk(t, m, "vdc"); !vdc.Unused || vdc.Type != "virtio" {
		t.Errorf("vdc = %+v", vdc)
	}
	if m.UsageKnown {
		t.Error("no usage was given")
	}
}

func TestRoleOf(t *testing.T) {
	for _, c := range []struct {
		vol, label, fs string
		want           Role
	}{
		{"EPHEMERAL", "", "xfs", RoleEphemeral}, {"", "STATE", "xfs", RoleState}, {"u-data", "", "xfs", RoleUser},
		{"e-old", "", "", RoleExisting}, {"r-raw", "", "", RoleRaw}, {"", "", "swap", RoleSwap}, {"s-swap", "", "", RoleSwap},
		{"", "", "lvm2-pv", RoleLVM}, {"", "", "linux_raid_member", RoleRAID}, {"IMAGECACHE", "", "xfs", RoleImage},
		{"", "mine", "ext4", RoleOther},
	} {
		if got := roleOf(c.vol, c.label, c.fs); got != c.want {
			t.Errorf("roleOf(%q,%q,%q) = %s, want %s", c.vol, c.label, c.fs, got, c.want)
		}
	}
}

func TestVolumeJoinFallsBackToParentAndIndex(t *testing.T) {
	in := Fixture("single-disk-cp")
	for i := range in.Volumes {
		delete(in.Volumes[i].Spec, "location") // an encrypted volume's location is the mapper device
	}
	m := Build(in)
	eph := disk(t, m, "sda").Segments
	if v := eph[len(eph)-2].Volume; v == nil || v.ID != "EPHEMERAL" {
		t.Errorf("join by parentLocation + partitionIndex failed: %+v", v)
	}
}

func TestBadDataDoesNotPanic(t *testing.T) {
	in := Fixture("single-disk-cp")
	for i := range in.Discovered {
		in.Discovered[i].Spec["size"] = "not a number"
		in.Discovered[i].Spec["offset"] = nil
	}
	in.Disks[0].Spec["size"] = 0
	_ = Build(in)
	_ = Build(Inputs{})
}
