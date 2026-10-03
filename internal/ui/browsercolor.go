package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/catalog"
)

// Browser colour: category accents, header chips and the per-kind suffix
// colours. Plain-text helpers (breadcrumb, watchIndicator, resourceLine) stay
// as they are; these produce the styled versions the header and panes draw.

// paneAccent is the accent colour of a pane: its category's.
func (app App) paneAccent(p pane) lipgloss.TerminalColor {
	b := app.browser
	switch p.kind {
	case paneCategories:
		rows := b.categoryRows(p.filter)
		if p.cur < len(rows) {
			return categoryAccent(rows[p.cur].key)
		}
	case paneTypes:
		return categoryAccent(p.category)
	case paneInstances, paneYAML:
		return categoryAccent(app.defCategory(p))
	case paneDescribe:
		if p.sub.cfg {
			return categoryAccent(catalog.ConfigCategoryFor(p.sub.ck.Group))
		}
		return categoryAccent(catalog.CategoryFor(p.sub.def))
	case paneNetwork:
		return categoryAccent("networking")
	case paneDisks:
		return categoryAccent("block")
	case paneRelated:
		if p.rel.subject.config {
			ck, _ := findConfigKind(p.rel.subject.kind)
			return categoryAccent(catalog.ConfigCategoryFor(ck.Group))
		}
		return categoryAccent(catalog.CategoryFor(p.rel.subject.def))
	}
	return accentDefault
}

func (app App) defCategory(p pane) string {
	if p.cfgKind != "" {
		ck, _ := findConfigKind(p.cfgKind)
		return catalog.ConfigCategoryFor(ck.Group)
	}
	return catalog.CategoryFor(p.def)
}

// roleChip is the node role badge: CP blue, W grey.
func roleChip(role string) string {
	switch role {
	case "controlplane":
		return chip("CP", colorRoleSpec)
	case "worker":
		return chip("W", colorRoleOther)
	}
	return dimStyle.Render(role)
}

// breadcrumbStyled is the breadcrumb with the node chip and a coloured
// category segment. Every segment carries its own style.
func (app App) breadcrumbStyled() string {
	b := app.browser
	sep := dimStyle.Render(" > ")
	out := dimStyle.Render("node: ") + lipgloss.NewStyle().Bold(true).Render(b.node.Hostname) + " " + roleChip(b.node.Role)
	for _, c := range app.crumbs() {
		seg := lipgloss.NewStyle().Render(c.text)
		if c.cat != "" {
			seg = accentStyle(c.cat).Render(c.text)
		}
		out += sep + seg
	}
	return out
}

// indicatorsStyled is the right part of the header: watch state and source as chips.
func (app App) indicatorsStyled() string {
	var parts []string
	switch app.watchIndicator() {
	case "watch":
		parts = append(parts, chip("watch", colorRoleStatus))
	case "watch off":
		parts = append(parts, dimStyle.Render("watch off"))
	case "watch lost":
		parts = append(parts, chip("watch lost", colorOrange))
	}
	if app.sourceName() == "grpc" {
		parts = append(parts, chip("grpc", colorRoleStatus))
	} else {
		parts = append(parts, chip("cli", colorMarkAccent))
	}
	return strings.Join(parts, " ")
}

// browserHeaderStyled mirrors browserHeaderLine with styled segments.
func (app App) browserHeaderStyled() string {
	avail := max(1, app.width-2)
	ind := app.indicatorsStyled()
	bc := app.breadcrumbStyled()
	if lipgloss.Width(ind)+12 > avail {
		return clipANSI(bc, avail)
	}
	left := clipANSI(bc, avail-lipgloss.Width(ind)-1)
	return padRight(left, avail-lipgloss.Width(ind)) + ind
}

// kindSuffix splits a display name into stem and a Config/Spec/Status suffix.
func kindSuffix(name string) (stem, suffix string) {
	for _, s := range []string{"Config", "Spec", "Status"} {
		if strings.HasSuffix(name, s) && len(name) > len(s) {
			return strings.TrimSuffix(name, s), s
		}
	}
	return name, ""
}

// span colours the runes [from, to) of a plain row.
type span struct {
	from, to int
	style    lipgloss.Style
}

// paintSpans renders a plain row with base, painting the spans (sorted, not
// overlapping) over it. Rows are ASCII, so runes stand for cells.
func paintSpans(text string, base lipgloss.Style, spans []span) string {
	rs := []rune(text)
	var sb strings.Builder
	pos := 0
	for _, sp := range spans {
		from, to := clamp(sp.from, pos, len(rs)), clamp(sp.to, 0, len(rs))
		if to <= from {
			continue
		}
		if from > pos {
			sb.WriteString(base.Render(string(rs[pos:from])))
		}
		sb.WriteString(sp.style.Render(string(rs[from:to])))
		pos = to
	}
	if pos < len(rs) {
		sb.WriteString(base.Render(string(rs[pos:])))
	}
	return sb.String()
}

// withFg keeps a row style's background and weight but swaps the colour.
func withFg(base lipgloss.Style, c lipgloss.TerminalColor) lipgloss.Style { return base.Foreground(c) }
