package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/diskmodel"
)

func TestLayoutWidths(t *testing.T) {
	sum := func(w []int) (n int) {
		for _, x := range w {
			n += x
		}
		return
	}
	// a typical system disk: tiny partitions stay visible, EPHEMERAL takes the rest
	sizes := []uint64{100 * diskmodel.MiB, diskmodel.MiB, 1000 * diskmodel.MiB, diskmodel.MiB, 100 * diskmodel.MiB, 118 * diskmodel.GiB}
	for _, total := range []int{40, 76, 110, 200} {
		w := layoutWidths(sizes, total, segMinW)
		if sum(w) != total {
			t.Errorf("total %d: widths %v sum to %d", total, w, sum(w))
		}
		for i, x := range w {
			if x < segMinW {
				t.Errorf("total %d: segment %d is %d wide, min %d", total, i, x, segMinW)
			}
		}
		if w[5] <= w[2] || w[2] < w[1] {
			t.Errorf("total %d: widths %v are not in proportion", total, w)
		}
	}
	// more segments than cells: one cell each for the first ones, the rest 0
	w := layoutWidths(make([]uint64, 10), 6, segMinW)
	if sum(w) != 6 || w[6] != 0 {
		t.Errorf("overflow widths = %v", w)
	}
	// min width shrinks before segments are dropped
	w = layoutWidths([]uint64{1, 1, 1, 1, 1}, 10, 3)
	if sum(w) != 10 || w[4] == 0 {
		t.Errorf("shrunk widths = %v", w)
	}
	if got := layoutWidths(nil, 10, 3); len(got) != 0 {
		t.Errorf("no segments: %v", got)
	}
}

func TestSegCellsAreExactlyWideAndTextFree(t *testing.T) {
	segs := []diskmodel.Segment{
		{Role: diskmodel.RoleEphemeral, Label: "EPHEMERAL", FS: "xfs", Size: 100, Usage: &diskmodel.Usage{Size: 100, Used: 61}},
		{Role: diskmodel.RoleUser, Label: "u-data", FS: "xfs", Encrypted: true, Size: 100},
		{Role: diskmodel.RoleBIOS, Label: "BIOS", Size: 1},
		{Role: diskmodel.RoleFree, Label: "unallocated", Size: 900 << 30},
		{Role: diskmodel.RoleState, Label: "STATE", Volume: &diskmodel.Volume{Phase: "failed"}},
	}
	for _, s := range segs {
		for w := 1; w <= 60; w++ {
			for _, sel := range []bool{false, true} {
				out := segCells(s, w, sel)
				if got := ansi.StringWidth(out); got != w {
					t.Fatalf("%s w=%d sel=%v: %d cells", s.Label, w, sel, got)
				}
				if plain := ansi.Strip(out); strings.ContainsAny(plain, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789%") {
					t.Fatalf("text over the bar: %q", plain)
				}
			}
		}
	}
	// used / free split inside a filesystem; unknown usage is solid; unallocated is dotted
	if out := ansi.Strip(segCells(segs[0], 10, false)); out != "▓▓▓▓▓▓░░░░" {
		t.Errorf("61%% of 10 cells = %q", out)
	}
	if out := ansi.Strip(segCells(segs[1], 4, false)); out != "████" {
		t.Errorf("unknown usage = %q", out)
	}
	if out := ansi.Strip(segCells(segs[3], 6, false)); out != "· · ·" && out != "· · · " {
		t.Errorf("unallocated = %q", out)
	}
}

func TestLegendUnderTheBar(t *testing.T) {
	m := diskmodel.Build(diskmodel.Fixture("qemu-vm"))
	vda := m.Disks[0]
	for _, d := range m.Disks {
		if d.ID == "vda" {
			vda = d
		}
	}
	lines := legendLines(vda, -1, false, 120)
	if len(lines) != 1 {
		t.Fatalf("legend lines = %d: %q", len(lines), lines)
	}
	plain := ansi.Strip(lines[0])
	for _, want := range []string{"● EFI vfat 100MiB", "● META 1MiB", "● STATE xfs 100MiB", "● EPHEMERAL xfs 7.8GiB"} {
		if !strings.Contains(plain, want) {
			t.Errorf("%q missing from %q", want, plain)
		}
	}
	// it wraps at the width instead of overflowing
	narrow := legendLines(vda, -1, false, 30)
	if len(narrow) < 2 {
		t.Fatalf("a 30-cell legend should wrap: %q", narrow)
	}
	for _, l := range narrow {
		if w := ansi.StringWidth(l); w > 30 {
			t.Errorf("legend line is %d cells: %q", w, ansi.Strip(l))
		}
	}
	// the selected entry is marked; decimal units follow u
	if !strings.Contains(strings.Join(legendLines(vda, 2, true, 120), ""), "104.9MB") {
		t.Error("decimal units should read 104.9MB")
	}
	// the dot carries the segment colour: two roles, two styles
	if legendLines(vda, -1, false, 120)[0] == "" {
		t.Error("empty legend")
	}
	for in, want := range map[uint64]string{diskmodel.MiB: "1MiB", 1500 * diskmodel.MiB: "1.5GiB", 100 * diskmodel.GiB: "100GiB"} {
		if got := fmtUnit(in, false); got != want {
			t.Errorf("fmtUnit(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSegColorIsStablePerVolume(t *testing.T) {
	a := diskmodel.Segment{Role: diskmodel.RoleUser, Label: "u-data"}
	if segColor(a) != segColor(a) {
		t.Error("colour changes between calls")
	}
	failed := a
	failed.Volume = &diskmodel.Volume{Phase: "failed"}
	if segColor(failed) != diskBad {
		t.Error("a failed volume is red")
	}
	if segColor(diskmodel.Segment{Role: diskmodel.RoleEphemeral}) == segColor(diskmodel.Segment{Role: diskmodel.RoleState}) {
		t.Error("EPHEMERAL and STATE share a colour")
	}
}

func TestFmtSize(t *testing.T) {
	for _, c := range []struct {
		n     uint64
		si    bool
		full  string
		short string
	}{
		{100 * diskmodel.MiB, false, "100.0 MiB", "100M"},
		{diskmodel.MiB, false, "1.0 MiB", "1M"},
		{1500 * diskmodel.MiB, false, "1.5 GiB", "1.5G"},
		{118 * diskmodel.GiB, false, "118.0 GiB", "118G"},
		{1800 * diskmodel.GiB, false, "1.8 TiB", "1.8T"},
		{2_000_000_000, true, "2.0 GB", "2G"},
		{512, false, "512 B", "512B"},
	} {
		if got := fmtSize(c.n, c.si); got != c.full {
			t.Errorf("fmtSize(%d,%v) = %q, want %q", c.n, c.si, got, c.full)
		}
		if got := fmtShort(c.n, c.si); got != c.short {
			t.Errorf("fmtShort(%d,%v) = %q, want %q", c.n, c.si, got, c.short)
		}
	}
}

func TestDiskRendersEachFixture(t *testing.T) {
	cases := map[string][]string{
		"single-disk-cp":   {"sda", "★ system", "Samsung 870 EVO", "● EPHEMERAL xfs", "1 disk", "1 system", "2 hidden"},
		"worker-encrypted": {"nvme0n1", "u-data xfs 900GiB lock", "unallocated", "not used by Talos", "dm-0", "crypt", "u-scratch waiting"},
		"qemu-vm":          {"vda", "virtio", "● EFI vfat 100MiB", "● META 1MiB", "● STATE xfs 100MiB", "● EPHEMERAL xfs 7.8GiB"},
		"whole-disk-fs":    {"vdb", "u-scratch xfs 200GiB", "not used by Talos 100GiB"},
	}
	for name, wants := range cases {
		t.Run(name, func(t *testing.T) {
			app := diskApp(t, name, 160, 50)
			out := netText(app)
			for _, w := range wants {
				if !strings.Contains(out, w) {
					t.Errorf("%q missing:\n%s", w, out)
				}
			}
		})
	}
}

func TestDiskRendersWithinBudget(t *testing.T) {
	for _, sz := range []struct{ w, h int }{{60, 20}, {80, 24}, {120, 40}, {200, 50}} {
		for _, fx := range diskmodel.FixtureNames() {
			sz, fx := sz, fx
			t.Run(fmt.Sprintf("%s_w%d_h%d", fx, sz.w, sz.h), func(t *testing.T) {
				app := diskApp(t, fx, sz.w, sz.h)
				app = press(t, app, "a") // every device, so every box is drawn
				p, _ := app.diskPane()
				for di := range p.disk.shown() {
					for si := 0; si < len(p.disk.shown()[di].Segments)+1; si++ {
						out := checkBudget(t, app, sz.w)
						for j, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
							if w := ansi.StringWidth(l); w > sz.w {
								t.Fatalf("disk %d seg %d line %d is %d cells, terminal is %d", di, si, j, w, sz.w)
							}
						}
						// the selected disk's box is on screen
						dp, _ := app.diskPane()
						d, _, _, _ := dp.disk.sel()
						if !strings.Contains(ansi.Strip(out), d.ID) {
							t.Fatalf("selected disk %s not visible:\n%s", d.ID, ansi.Strip(out))
						}
						app = press(t, app, "l")
					}
					app = press(t, app, "j")
				}
			})
		}
	}
}

func TestDiskKeys(t *testing.T) {
	app := diskApp(t, "worker-encrypted", 160, 50)
	sel := func(a App) (string, string) {
		p, _ := a.diskPane()
		d, s, ok, _ := p.disk.sel()
		if !ok {
			return d.ID, ""
		}
		return d.ID, s.Label
	}
	if d, s := sel(app); d != "sda" || s != "EFI" {
		t.Fatalf("start = %s/%s", d, s)
	}
	app = press(t, app, "l", "l")
	if _, s := sel(app); s != "BOOT" {
		t.Errorf("l l = %s", s)
	}
	app = press(t, app, "h", "h", "h")
	if _, s := sel(app); s != "EFI" {
		t.Errorf("h clamps at the first partition, got %s", s)
	}
	app = press(t, app, "j")
	if d, s := sel(app); d != "nvme0n1" || s != "u-data" {
		t.Errorf("j = %s/%s", d, s)
	}
	app = press(t, app, "G")
	if d, _ := sel(app); d != "sdb" {
		t.Errorf("G = %s", d)
	}
	app = press(t, app, "j", "j")
	if d, _ := sel(app); d != "sdb" {
		t.Errorf("j past the end moved to %s", d)
	}
	app = press(t, app, "g")
	if d, _ := sel(app); d != "sda" {
		t.Errorf("g = %s", d)
	}
	app = press(t, app, "k")
	if d, _ := sel(app); d != "sda" {
		t.Errorf("k at the top moved to %s", d)
	}

	// u toggles units everywhere
	if !strings.Contains(netText(app), "GiB") {
		t.Fatal("binary units by default")
	}
	u := press(t, app, "u")
	if out := netText(u); strings.Contains(out, "GiB") || !strings.Contains(out, "GB") || u.statusMsg == "" {
		t.Errorf("u did not switch to GB:\n%s", out)
	}
	if out := netText(press(t, u, "u")); !strings.Contains(out, "GiB") {
		t.Error("u again should switch back")
	}
}

func TestDiskShowAllDevices(t *testing.T) {
	app := diskApp(t, "single-disk-cp", 160, 50)
	if out := netText(app); strings.Contains(out, "loop0") || strings.Contains(out, "sr0") || !strings.Contains(out, "2 hidden (a)") {
		t.Fatalf("loop and cdrom should be hidden:\n%s", out)
	}
	all := press(t, app, "a")
	if out := netText(all); !strings.Contains(out, "loop0") || !strings.Contains(out, "sr0") || strings.Contains(out, "hidden") {
		t.Errorf("a should show every device:\n%s", out)
	}
	back := press(t, all, "j", "j", "a") // selection on a hidden disk is clamped when it goes away
	if p, _ := back.diskPane(); p.disk.disk != 0 {
		t.Errorf("selection = %d after hiding", p.disk.disk)
	}
}

func TestDiskEnterOpensTheRightResource(t *testing.T) {
	app := diskApp(t, "worker-encrypted", 160, 50)
	app = press(t, app, "l", "l", "l", "l", "l") // sda: EFI BIOS BOOT META STATE EPHEMERAL
	a, cmd := app.handleKey(key("enter"))
	top, _ := a.browser.top()
	if top.kind != paneYAML || top.def.Type != diskmodel.TypeVolume || top.meta.ID != "EPHEMERAL" || top.meta.Namespace != "runtime" || cmd == nil {
		t.Fatalf("enter on EPHEMERAL: %+v", top)
	}
	if a = press(t, a, "esc"); func() bool { top, _ := a.browser.top(); return top.kind != paneDisks }() {
		t.Fatal("esc should return to the disk view")
	}
	if p, _ := a.diskPane(); p.disk.seg != 5 {
		t.Errorf("selection lost: seg %d", p.disk.seg)
	}
	// a partition without a volume status opens the probed volume
	in := diskmodel.Fixture("worker-encrypted")
	in.Volumes = nil
	b := diskApp(t, "worker-encrypted", 160, 50)
	b = b.handleDiskFetch(diskFetchMsg{node: b.browser.node.IP, seq: 1, res: diskmodel.FetchResult{In: in}})
	if a, _ := b.handleKey(key("enter")); func() bool {
		top, _ := a.browser.top()
		return top.def.Type != diskmodel.TypeDiscovered || top.meta.ID != "sda1"
	}() {
		top, _ := a.browser.top()
		t.Errorf("enter without a volume status: %+v", top)
	}
	// unallocated space and unused disks open the Disk resource
	app = press(t, app, "G")
	a, _ = app.handleKey(key("enter"))
	if top, _ := a.browser.top(); top.def.Type != diskmodel.TypeDisk || top.meta.ID != "sdb" {
		t.Errorf("enter on an unused disk: %+v", top)
	}
}

func TestDiskDescribeAndRelated(t *testing.T) {
	app := diskApp(t, "single-disk-cp", 160, 50)
	a := press(t, app, "d")
	top, _ := a.browser.top()
	if top.kind != paneDescribe || top.sub.def.Type != diskmodel.TypeVolume || !top.sub.hasMeta || top.sub.meta.ID != "EFI" {
		t.Fatalf("d: %+v", top)
	}
	a = press(t, app, "p")
	if top, _ = a.browser.top(); top.kind != paneRelated || top.rel.subject.def.Type != diskmodel.TypeVolume {
		t.Fatalf("p: %+v", top)
	}
	early := diskApp(t, "single-disk-cp", 160, 50)
	early.browser.defs = nil
	if a := press(t, early, "p"); !strings.Contains(a.statusMsg, "definitions") {
		t.Errorf("p without definitions: %q", a.statusMsg)
	}
	if a := press(t, early, "enter"); func() bool { top, _ := a.browser.top(); return top.kind != paneYAML }() {
		t.Error("enter without definitions should still open the YAML")
	}
}

func TestDiskTableDescribesTheSelection(t *testing.T) {
	app := diskApp(t, "single-disk-cp", 200, 50)
	app = press(t, app, "l", "l", "l", "l", "l")
	out := netText(app)
	for _, want := range []string{"sda › EPHEMERAL", "LABEL", "EPHEMERAL", "xfs", "/var", "72G 61%", "ready", "EPHEMERAL: container data"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing:\n%s", want, out)
		}
	}
	// a failed volume shows its error
	in := diskmodel.Fixture("single-disk-cp")
	for i := range in.Volumes {
		if in.Volumes[i].ID == "STATE" {
			in.Volumes[i].Spec["phase"] = "failed"
			in.Volumes[i].Spec["errorMessage"] = "cannot unlock: wrong key"
		}
	}
	app = app.handleDiskFetch(diskFetchMsg{node: app.browser.node.IP, seq: 1, res: diskmodel.FetchResult{In: in}})
	app = press(t, app, "g", "h", "h", "h", "h", "h", "h", "l", "l", "l", "l")
	if out := netText(app); !strings.Contains(out, "cannot unlock: wrong key") || !strings.Contains(out, "failed") {
		t.Errorf("a failed volume should show its phase and error:\n%s", out)
	}
}

func TestDiskEntryPointsAndReload(t *testing.T) {
	app := newTestApp(120, 40)
	app.nodes = makeNodes(3)
	app.nodeCur = 2
	a := press(t, app, "i")
	if top, ok := a.browser.top(); !ok || top.kind != paneDisks || len(a.browser.stack) != 1 || a.browser.node.IP != "10.0.0.3" {
		t.Fatalf("i: stack=%v", a.browser.stack)
	}
	if back := press(t, a, "esc"); back.state != StateNodeList || back.nodeCur != 2 {
		t.Errorf("esc: state=%v nodeCur=%d", back.state, back.nodeCur)
	}
	if c, _ := app.runCommand("disks"); func() bool { top, ok := c.browser.top(); return !ok || top.kind != paneDisks }() {
		t.Error(":disks from the node list did not open the view")
	}
	b := browserApp(120, 40, 2, 10)
	if in, _ := b.runCommand("dk"); func() bool { top, _ := in.browser.top(); return top.kind != paneDisks || len(in.browser.stack) != 3 }() {
		t.Error(":dk in the browser should push the view")
	}

	// reload bumps the sequence; a late reply for the old one is ignored
	d := diskApp(t, "single-disk-cp", 120, 40)
	r, cmd := d.handleKey(key("ctrl+r"))
	p, _ := r.diskPane()
	if p.disk.seq != 2 || !p.disk.loading || cmd == nil {
		t.Fatalf("ctrl+r: seq=%d loading=%v", p.disk.seq, p.disk.loading)
	}
	r = r.handleDiskFetch(diskFetchMsg{node: r.browser.node.IP, seq: 1, res: diskmodel.FetchResult{In: diskmodel.Fixture("whole-disk-fs")}})
	if p, _ = r.diskPane(); len(p.disk.model.Disks) != 3 || !p.disk.loading {
		t.Errorf("a stale fetch replaced the model: %d disks, loading=%v", len(p.disk.model.Disks), p.disk.loading)
	}
	r = r.handleDiskFetch(diskFetchMsg{node: "10.9.9.9", seq: 2, res: diskmodel.FetchResult{}})
	if p, _ = r.diskPane(); !p.disk.loading {
		t.Error("a reply for another node was applied")
	}
}

func TestDiskHintsAndEmpty(t *testing.T) {
	app := diskApp(t, "single-disk-cp", 160, 50)
	var hints []string
	for _, h := range stateHints(app) {
		hints = append(hints, h.key+" "+h.desc)
	}
	joined := strings.Join(hints, "|")
	for _, want := range []string{"YAML", "Describe", "Show all devices", "GiB"} {
		if !strings.Contains(joined, want) {
			t.Errorf("hint bar lacks %q: %s", want, joined)
		}
	}
	if help := buildHelpContent(); !strings.Contains(help, "disk view") || !strings.Contains(help, "Show all devices") {
		t.Error("help does not list the disk view keys")
	}
	// no disks at all
	empty := diskApp(t, "single-disk-cp", 120, 40)
	empty = empty.handleDiskFetch(diskFetchMsg{node: empty.browser.node.IP, seq: 1, res: diskmodel.FetchResult{Denied: []string{"Disk"}}})
	if out := netText(empty); !strings.Contains(out, "no disks found") || !strings.Contains(out, "could not be read") {
		t.Errorf("empty view:\n%s", out)
	}
	checkBudget(t, empty, 120)
}
