package ui

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/diskmodel"
)

// Drawing of the disk view: a summary line, one box per disk with a bar split
// into its partitions, and a table for the selected partition.

const (
	segMinW       = 3  // narrowest a segment gets, so 1 MiB partitions stay visible
	diskNoSizeRow = 80 // pane width below which the size line is dropped
)

// --- sizes ---

func fmtSize(n uint64, si bool) string {
	base, units := 1024.0, []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	if si {
		base, units = 1000.0, []string{"B", "KB", "MB", "GB", "TB", "PB"}
	}
	v, i := float64(n), 0
	for v >= base && i < len(units)-1 {
		v /= base
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

// fmtShort is the compact form for the size line: 100M, 1.0G, 118G.
func fmtShort(n uint64, si bool) string {
	base, units := 1024.0, []string{"B", "K", "M", "G", "T", "P"}
	if si {
		base = 1000.0
	}
	v, i := float64(n), 0
	for v >= base && i < len(units)-1 {
		v /= base
		i++
	}
	if i == 0 || v >= 10 || v == math.Floor(v) {
		return fmt.Sprintf("%d%s", int(math.Round(v)), units[i])
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

// --- colours ---

func ad(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

var (
	roleColors = map[diskmodel.Role]lipgloss.AdaptiveColor{
		diskmodel.RoleEFI:       ad("#57606a", "#8b9bb0"), // slate shades for the system partitions
		diskmodel.RoleBIOS:      ad("#6e7781", "#7d8ea3"),
		diskmodel.RoleBoot:      ad("#4c6a8c", "#6a8fb8"),
		diskmodel.RoleMeta:      ad("#7a869a", "#94a3b8"),
		diskmodel.RoleState:     ad("#3b5b84", "#5b87c4"),
		diskmodel.RoleEphemeral: ad("#1a7f37", "#3fb950"),
		diskmodel.RoleImage:     ad("#0a7f8a", "#39c5cf"),
		diskmodel.RoleRaw:       ad("#bc4c00", "#ffa657"),
		diskmodel.RoleSwap:      ad("#bc4c00", "#ffa657"),
		diskmodel.RoleLVM:       ad("#8250df", "#bc8cff"),
		diskmodel.RoleRAID:      ad("#8250df", "#bc8cff"),
		diskmodel.RoleOther:     ad("#6e7781", "#8b949e"),
		diskmodel.RoleFree:      ad("#8c959f", "#6e7681"),
	}
	userPalette = []lipgloss.AdaptiveColor{
		ad("#bf3989", "#f778ba"), ad("#0969da", "#58a6ff"), ad("#9a6700", "#e3b341"),
		ad("#0a7f8a", "#39c5cf"), ad("#8250df", "#d2a8ff"), ad("#a40e26", "#ff9bce"),
	}
	diskBad = ad("#cf222e", "#ff7b72")
)

// segColor is the colour of a segment: by role, one palette colour per user
// or existing volume name (a stable hash), red when the volume is not ready.
func segColor(s diskmodel.Segment) lipgloss.AdaptiveColor {
	if s.Failed() {
		return diskBad
	}
	switch s.Role {
	case diskmodel.RoleUser, diskmodel.RoleExisting:
		h := fnv.New32a()
		h.Write([]byte(s.Label))
		return userPalette[int(h.Sum32())%len(userPalette)]
	}
	if c, ok := roleColors[s.Role]; ok {
		return c
	}
	return roleColors[diskmodel.RoleOther]
}

// roleNote says what a partition is for. Written from the Talos storage
// references (disk-management) and the Talos docs; the Ubuntu part names the
// closest thing a Linux admin knows.
var roleNotes = map[diskmodel.Role]string{
	diskmodel.RoleEFI:       "EFI system partition: the bootloader (sd-boot or GRUB) lives here. Ubuntu: /boot/efi.",
	diskmodel.RoleBIOS:      "BIOS boot partition, used when the machine boots in legacy mode. Ubuntu: the bios_grub partition.",
	diskmodel.RoleBoot:      "Boot assets (kernel and initramfs) the bootloader loads. Ubuntu: /boot.",
	diskmodel.RoleMeta:      "META: Talos metadata, such as encryption config, that outlives a reset of the other volumes. No Ubuntu equivalent.",
	diskmodel.RoleState:     "STATE: machine configuration, secrets and certificates; the most sensitive partition. Ubuntu: the config under /etc, roughly.",
	diskmodel.RoleEphemeral: "EPHEMERAL: container data, images, logs and etcd data; wiped by a reset. Ubuntu: /var on its own partition.",
	diskmodel.RoleImage:     "IMAGECACHE: container images cached for booting without a registry.",
	diskmodel.RoleUser:      "A user volume from a VolumeConfig document, mounted under /var/mnt. Ubuntu: an extra partition in /etc/fstab.",
	diskmodel.RoleExisting:  "An existing volume Talos mounts but did not create.",
	diskmodel.RoleRaw:       "A raw volume: Talos makes the partition and leaves it unformatted.",
	diskmodel.RoleSwap:      "Swap space.",
	diskmodel.RoleLVM:       "An LVM physical volume. Ubuntu: pvs, vgs, lvs.",
	diskmodel.RoleRAID:      "A member of an MD RAID array. Ubuntu: mdadm --detail.",
	diskmodel.RoleOther:     "A partition Talos did not create and does not manage.",
	diskmodel.RoleFree:      "Space no partition uses. A new volume can claim it.",
}

// --- segment widths ---

// layoutWidths shares total cells between the sizes: every segment gets at
// least minW, the rest goes in proportion to size. When even one cell each
// does not fit, the tail gets width 0.
func layoutWidths(sizes []uint64, total, minW int) []int {
	n := len(sizes)
	w := make([]int, n)
	if n == 0 || total <= 0 {
		return w
	}
	for minW > 1 && n*minW > total {
		minW--
	}
	if n*minW > total {
		for i := 0; i < total; i++ {
			w[i] = 1
		}
		return w
	}
	for i := range w {
		w[i] = minW
	}
	rem := total - n*minW
	var sum float64
	for _, s := range sizes {
		sum += float64(s)
	}
	if sum == 0 {
		w[n-1] += rem
		return w
	}
	type fr struct {
		i    int
		frac float64
	}
	var frs []fr
	given := 0
	for i, s := range sizes {
		exact := float64(rem) * float64(s) / sum
		whole := int(exact)
		w[i] += whole
		given += whole
		frs = append(frs, fr{i, exact - float64(whole)})
	}
	for ; given < rem; given++ { // largest remainders get the leftover cells
		best := 0
		for k := range frs {
			if frs[k].frac > frs[best].frac {
				best = k
			}
		}
		w[frs[best].i]++
		frs[best].frac = -1
	}
	return w
}

// --- the bar ---

func segLabelVariants(s diskmodel.Segment, unused, si bool) []string {
	if s.Role == diskmodel.RoleFree {
		if unused {
			return []string{"not used by Talos"}
		}
		return []string{"unallocated " + fmtSize(s.Size, si), "unallocated", "free"}
	}
	pct := ""
	if s.Usage != nil && s.Usage.Size > 0 {
		pct = fmt.Sprintf("%d%%", int(math.Round(100*float64(s.Usage.Used)/float64(s.Usage.Size))))
	}
	join := func(parts ...string) string {
		var out []string
		for _, p := range parts {
			if p != "" {
				out = append(out, p)
			}
		}
		return strings.Join(out, " ")
	}
	lock := ""
	if s.Encrypted {
		lock = "lock"
	}
	vs := []string{join(s.Label, s.FS, lock, pct), join(s.Label, lock, pct), join(s.Label, lock), s.Label}
	return vs
}

// segCells draws one segment: fill glyphs (used ▓, free ░, unknown █,
// unallocated ·) with the label on a coloured chip over them.
func segCells(s diskmodel.Segment, w int, selected, unused, si bool) string {
	if w <= 0 {
		return ""
	}
	col := segColor(s)
	fill := make([]rune, w)
	switch {
	case s.Role == diskmodel.RoleFree:
		for i := range fill {
			fill[i] = '·'
			if i%2 == 1 {
				fill[i] = ' '
			}
		}
	case s.Usage != nil && s.Usage.Size > 0:
		used := int(math.Round(float64(w) * float64(s.Usage.Used) / float64(s.Usage.Size)))
		for i := range fill {
			fill[i] = '░'
			if i < used {
				fill[i] = '▓'
			}
		}
	default:
		for i := range fill {
			fill[i] = '█'
		}
	}

	text := ""
	for _, v := range segLabelVariants(s, unused, si) {
		if len([]rune(v)) <= w-2 {
			text = v
			break
		}
	}
	if text == "" && s.Role != diskmodel.RoleFree {
		lab := []rune(s.Label)
		switch {
		case len(lab) <= w:
			text = s.Label
		case w >= 4:
			text = string(lab[:w-1]) + "…"
		}
	}
	start := 0
	if tl := len([]rune(text)); tl > 0 && w >= tl+2 {
		start = 1
	}
	fillStyle := lipgloss.NewStyle().Foreground(col)
	if s.Role == diskmodel.RoleFree {
		fillStyle = dimStyle
	}
	chip := lipgloss.NewStyle().Background(col).Foreground(colorOnAccent).Bold(true)
	if s.Role == diskmodel.RoleFree {
		chip = lipgloss.NewStyle().Foreground(colorRoleOther)
	}
	if selected {
		chip = chip.Underline(true)
	}
	var sb strings.Builder
	tr := []rune(text)
	sb.WriteString(fillStyle.Render(string(fill[:start])))
	sb.WriteString(chip.Render(string(tr)))
	sb.WriteString(fillStyle.Render(string(fill[min(w, start+len(tr)):])))
	return sb.String()
}

// diskBlock returns the lines of one disk's box, every line exactly iw wide.
func (app App) diskBlock(d diskmodel.Disk, dv diskView, selDisk bool, iw int) []string {
	border := paneInactiveBorder
	if selDisk {
		border = lipgloss.NewStyle().Foreground(categoryAccent("block"))
	}
	bw := max(1, iw-2)   // between the box borders
	barW := max(1, bw-2) // one space of padding each side

	// title
	var tp []string
	tp = append(tp, lipgloss.NewStyle().Bold(true).Render(d.ID))
	if d.System {
		tp = append(tp, lipgloss.NewStyle().Foreground(colorMarkAccent).Render("★ system"))
	}
	meta := []string{fmtSize(d.Size, dv.si)}
	for _, x := range []string{d.Model, d.Type, d.Transport} {
		if x != "" && (len(meta) == 0 || meta[len(meta)-1] != x) {
			meta = append(meta, x)
		}
	}
	if d.Serial != "" && iw >= 100 {
		meta = append(meta, "sn "+d.Serial)
	}
	title := " " + strings.Join(tp, " ") + dimStyle.Render(" · "+strings.Join(meta, " · ")) + " "
	title = clipANSI(title, bw)
	top := border.Render("╭") + title + border.Render(strings.Repeat("─", max(0, bw-lipgloss.Width(title)))+"╮")

	row := func(content string) string {
		return border.Render("│") + " " + padRight(clipANSI(content, barW), barW) + " " + border.Render("│")
	}

	sizes := make([]uint64, len(d.Segments))
	for i, s := range d.Segments {
		sizes[i] = s.Size
	}
	widths := layoutWidths(sizes, barW, segMinW)
	var bar, szs, mark strings.Builder
	selSeg := -1
	if selDisk {
		selSeg = dv.seg
	}
	for i, s := range d.Segments {
		w := widths[i]
		if w == 0 {
			continue
		}
		bar.WriteString(segCells(s, w, i == selSeg, d.Unused, dv.si))
		short := fmtShort(s.Size, dv.si)
		cell := ""
		if len(short) < w {
			cell = short
		}
		style := dimStyle
		if i == selSeg {
			style = lipgloss.NewStyle().Bold(true)
		}
		szs.WriteString(style.Render(fit(cell, w)))
		if i == selSeg {
			mark.WriteString(lipgloss.NewStyle().Foreground(categoryAccent("block")).Render(strings.Repeat("▔", w)))
		} else {
			mark.WriteString(strings.Repeat(" ", w))
		}
	}
	lines := []string{top, row(bar.String())}
	if iw >= diskNoSizeRow {
		lines = append(lines, row(szs.String()), row(mark.String()))
	} else if selDisk {
		lines = append(lines, row(mark.String()))
	}
	for _, mp := range d.Mappers {
		x := dimStyle.Render("↳ " + mp.Dev)
		parts := []string{}
		for _, v := range []string{mp.Kind, fmtSize(mp.Size, dv.si), mp.FS} {
			if v != "" && v != "0 B" {
				parts = append(parts, v)
			}
		}
		x += dimStyle.Render(" · " + strings.Join(parts, " · "))
		if mp.Backing != "" {
			x += dimStyle.Render(" · on " + mp.Backing)
		}
		lines = append(lines, row(x))
	}
	for _, pv := range d.Pending {
		msg := "⚠ " + pv.ID + " " + pv.Phase
		if pv.Error != "" {
			msg += ": " + pv.Error
		}
		lines = append(lines, row(lipgloss.NewStyle().Foreground(diskBad).Render(msg)))
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", bw)+"╯"))
	return lines
}

// --- the table ---

type diskCol struct {
	title string
	w     int
	get   func(d diskmodel.Disk, s diskmodel.Segment, si bool) string
}

func diskCols() []diskCol {
	return []diskCol{
		{"#", 3, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string {
			if s.Index > 0 {
				return fmt.Sprint(s.Index)
			}
			return "-"
		}},
		{"LABEL", 12, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string { return s.Label }},
		{"VOLUME", 12, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string {
			if s.Volume != nil {
				return s.Volume.ID
			}
			return "-"
		}},
		{"FS", 7, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string { return orDash(s.FS) }},
		{"SIZE", 11, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string { return fmtSize(s.Size, si) }},
		{"OFFSET", 11, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string { return fmtSize(s.Offset, si) }},
		{"MOUNT", 0, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string { return orDash(s.Mount) }},
		{"USED", 13, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string {
			if s.Usage == nil || s.Usage.Size == 0 {
				return "-"
			}
			return fmt.Sprintf("%s %d%%", fmtShort(s.Usage.Used, si), int(math.Round(100*float64(s.Usage.Used)/float64(s.Usage.Size))))
		}},
		{"PHASE", 9, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string {
			if s.Volume != nil {
				return s.Volume.Phase
			}
			return "-"
		}},
		{"ENC", 7, func(d diskmodel.Disk, s diskmodel.Segment, si bool) string {
			if s.Encrypted {
				return orDash(s.Provider)
			}
			return "-"
		}},
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// diskTable returns the table lines for the selected segment: title, header, row, note.
func (app App) diskTable(dv diskView, iw, rows int) []string {
	d, s, hasSeg, ok := dv.sel()
	if !ok || rows <= 0 {
		return nil
	}
	cols := diskCols()
	// drop columns until the table fits; MOUNT (w 0) takes what is left
	drop := []string{"OFFSET", "ENC", "VOLUME", "USED", "PHASE"}
	fits := func(cs []diskCol) (int, bool) {
		fixed := 2
		for _, c := range cs {
			fixed += c.w + 1
		}
		return iw - fixed, iw-fixed >= 8
	}
	for len(drop) > 0 {
		if _, okf := fits(cols); okf {
			break
		}
		var next []diskCol
		for _, c := range cols {
			if c.title != drop[0] {
				next = append(next, c)
			}
		}
		cols, drop = next, drop[1:]
	}
	left, _ := fits(cols)
	for i := range cols {
		if cols[i].w == 0 {
			cols[i].w = max(5, left)
		}
	}
	name := d.ID
	if hasSeg {
		name += " › " + s.Label
	}
	out := []string{lipgloss.NewStyle().Bold(true).Render(" " + cutWidth(name, iw-2))}
	var hdr, row strings.Builder
	hdr.WriteString("  ")
	row.WriteString("  ")
	seg := s
	if !hasSeg {
		seg = diskmodel.Segment{Label: "unused", Role: diskmodel.RoleFree, Size: d.Size}
	}
	for _, c := range cols {
		hdr.WriteString(fit(c.title, c.w) + " ")
		row.WriteString(fit(c.get(d, seg, dv.si), c.w) + " ")
	}
	out = append(out, colHeaderStyle.Render(fit(hdr.String(), iw)))
	rowStyle := lipgloss.NewStyle().Foreground(segColor(seg)).Bold(true)
	out = append(out, rowStyle.Render(fit(row.String(), iw)))
	if rows >= 4 {
		note := roleNotes[seg.Role]
		switch {
		case seg.Volume != nil && seg.Volume.Error != "":
			note = "⚠ " + seg.Volume.Error
		case d.Unused && !hasSeg || d.Unused && seg.Role == diskmodel.RoleFree:
			note = "Talos does not use this disk: no partition or volume is on it."
		}
		out = append(out, dimStyle.Render(fit(" "+note, iw)))
	}
	return out[:min(len(out), rows)]
}

// --- the pane ---

func (app App) disksLines(p pane, iw, inner int, active bool) []string {
	dv := p.disk
	if !dv.ready {
		return messageLines(iw, inner, dimStyle, "loading the disks of this node…")
	}
	ds := dv.shown()
	out := []string{padRight(clipANSI(" "+dimStyle.Render(diskSummary(dv)), iw), iw)}
	if len(ds) == 0 {
		why := "no disks found on this node"
		if len(dv.res.Denied)+len(dv.res.Failed) > 0 {
			why += " (some block resources could not be read)"
		}
		return append(out, messageLines(iw, inner-1, warnStyle, why)...)
	}
	tableRows := 4
	switch {
	case inner < 14:
		tableRows = 3
	case inner < 10:
		tableRows = 0
	}
	region := max(1, inner-1-tableRows)

	// all blocks stacked; keep the selected one in view
	var blocks [][]string
	start := make([]int, len(ds))
	total := 0
	for i, d := range ds {
		start[i] = total
		b := app.diskBlock(d, dv, i == dv.disk, iw)
		blocks = append(blocks, b)
		total += len(b)
	}
	scroll := dv.scroll
	selEnd := start[dv.disk] + len(blocks[dv.disk])
	if start[dv.disk] < scroll {
		scroll = start[dv.disk]
	}
	if selEnd > scroll+region {
		scroll = max(start[dv.disk], selEnd-region)
	}
	scroll = clamp(scroll, 0, max(0, total-region))
	var flat []string
	for _, b := range blocks {
		flat = append(flat, b...)
	}
	for i := scroll; i < len(flat) && i < scroll+region; i++ {
		out = append(out, flat[i])
	}
	for len(out) < 1+region {
		out = append(out, "")
	}
	return append(out, app.diskTable(dv, iw, tableRows)...)
}

func (app App) disksNextStep(p pane) []string {
	if !p.disk.ready {
		return []string{"loading…"}
	}
	return []string{"↑↓ disk", "←→ partition", "↵ YAML", "d what is this", "a all devices", "u units"}
}
