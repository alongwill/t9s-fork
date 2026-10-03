package ui

import "github.com/charmbracelet/lipgloss"

// k9s-inspired palette
var (
	colorBg      = lipgloss.Color("#0d1117")
	colorCyan    = lipgloss.Color("#00b4d8")
	colorGreen   = lipgloss.Color("#3ddc84")
	colorRed     = lipgloss.Color("#ff6b6b")
	colorOrange  = lipgloss.Color("#ffb347")
	colorBlue    = lipgloss.Color("#74b9ff")
	colorYellow  = lipgloss.Color("#fdcb6e")
	colorMagenta = lipgloss.Color("#a29bfe")
	colorGray    = lipgloss.Color("#636e72")
	colorDimGray = lipgloss.Color("#444c56")
	colorWhite   = lipgloss.Color("#dfe6e9")
	colorBgSel   = lipgloss.Color("#1e3a5f")
	colorBgHead  = lipgloss.Color("#161b22")
)

// Header bar
var headerStyle = lipgloss.NewStyle().
	Background(colorBgHead).
	Foreground(colorCyan).
	Bold(true).
	Padding(0, 1)

var headerDimStyle = lipgloss.NewStyle().
	Background(colorBgHead).
	Foreground(colorGray)

var headerSepStyle = lipgloss.NewStyle().
	Background(colorBgHead).
	Foreground(colorDimGray)

// Resource title bar (row below header)
var titleBarStyle = lipgloss.NewStyle().
	Foreground(colorYellow).
	Bold(true)

// Table
var colHeaderStyle = lipgloss.NewStyle().
	Foreground(colorCyan).
	Bold(true)

var selectedStyle = lipgloss.NewStyle().
	Background(colorBgSel).
	Foreground(colorWhite).
	Bold(true)

// Text styles
var (
	titleStyle = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(colorGray)
	okStyle    = lipgloss.NewStyle().Foreground(colorGreen)
	errStyle   = lipgloss.NewStyle().Foreground(colorRed)
	warnStyle  = lipgloss.NewStyle().Foreground(colorOrange)
	infoStyle  = lipgloss.NewStyle().Foreground(colorBlue)
	keyStyle   = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
)

// Help overlay
var helpBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorCyan).
	Padding(1, 3).
	Background(colorBgHead)

// Semantic coloring helpers
func colorNodeStatus(s string) string {
	switch s {
	case "ready":
		return okStyle.Render(s)
	case "rebooting", "booting", "upgrading", "installing", "…":
		return warnStyle.Render(s)
	case "maintenance", "resetting", "unreachable":
		return errStyle.Render(s)
	case "shutting-down":
		return errStyle.Render(s)
	case "not-ready", "not ready":
		return errStyle.Render(s)
	}
	return dimStyle.Render(s)
}

func colorRole(role string) string {
	switch role {
	case "controlplane":
		return lipgloss.NewStyle().Foreground(colorMagenta).Bold(true).Render(role)
	case "worker":
		return lipgloss.NewStyle().Foreground(colorBlue).Render(role)
	}
	return role
}

func colorHealth(h string) string {
	switch h {
	case "OK", "healthy":
		return okStyle.Render(h)
	case "unhealthy":
		return errStyle.Render(h)
	case "?":
		return dimStyle.Render(h)
	}
	return dimStyle.Render(h)
}

func colorState(s string) string {
	switch s {
	case "Running":
		return okStyle.Render(s)
	case "Stopped", "Finished":
		return dimStyle.Render(s)
	case "Failed":
		return errStyle.Render(s)
	}
	return s
}

func colorLogLine(line string) string {
	switch {
	case containsAny(line, "ERROR", "FATAL", "CRIT", "error", "fatal", "crit"):
		return errStyle.Render(line)
	case containsAny(line, "WARN", "WARNING", "warn"):
		return warnStyle.Render(line)
	case containsAny(line, "DEBUG", "debug", "TRACE", "trace"):
		return dimStyle.Render(line)
	}
	return line
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > len(s) {
			continue
		}
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

// Role colours: the three layers of a pipeline (Config → Spec → Status) look the
// same everywhere they appear (related view, types pane). AdaptiveColor keeps
// them readable on light terminals.
var (
	colorRoleConfig  = lipgloss.AdaptiveColor{Light: "#8250df", Dark: "#bc8cff"} // magenta / purple
	colorRoleSpec    = lipgloss.AdaptiveColor{Light: "#0969da", Dark: "#58a6ff"} // blue
	colorRoleStatus  = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#3fb950"} // green
	colorRoleOther   = lipgloss.AdaptiveColor{Light: "#6e7781", Dark: "#8b949e"} // grey
	colorOnAccent    = lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#0d1117"} // text on a coloured chip
	colorMarkAccent  = lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#e3b341"} // marked cell, highlighted box
	colorLockAccent  = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#ff7b72"}
	colorBorderQuiet = lipgloss.AdaptiveColor{Light: "#afb8c1", Dark: "#444c56"}
)

func roleColor(r relRole) lipgloss.AdaptiveColor {
	switch r {
	case roleConfig:
		return colorRoleConfig
	case roleSpec:
		return colorRoleSpec
	case roleStatus:
		return colorRoleStatus
	}
	return colorRoleOther
}

// roleStyle is the foreground style of a role.
func roleStyle(r relRole) lipgloss.Style { return lipgloss.NewStyle().Foreground(roleColor(r)) }

// chip is a small coloured badge: ` text ` on a background.
func chip(text string, bg lipgloss.TerminalColor) string {
	return lipgloss.NewStyle().Background(bg).Foreground(colorOnAccent).Bold(true).Padding(0, 1).Render(text)
}

// Category accents: one colour per browser category. They colour the category
// name, the breadcrumb segment and the active pane's border.
var categoryAccents = map[string]lipgloss.AdaptiveColor{
	"networking": {Light: "#0a7f8a", Dark: "#39c5cf"}, // cyan
	"siderolink": {Light: "#bf3989", Dark: "#f778ba"}, // pink
	"kubernetes": {Light: "#0550ae", Dark: "#58a6ff"}, // blue
	"cluster":    {Light: "#8250df", Dark: "#bc8cff"}, // magenta
	"block":      {Light: "#bc4c00", Dark: "#ffa657"}, // orange
	"storage":    {Light: "#9a6700", Dark: "#e3b341"}, // yellow
	"cri":        {Light: "#7d4e00", Dark: "#d29922"}, // amber
	"containers": {Light: "#6639ba", Dark: "#d2a8ff"}, // lilac
	"hypervisor": {Light: "#a40e26", Dark: "#ff9bce"},
	"hardware":   {Light: "#57606a", Dark: "#adbac7"},
	"security":   {Light: "#cf222e", Dark: "#ff7b72"}, // red
	"extensions": {Light: "#116329", Dark: "#7ee787"},
	"runtime":    {Light: "#1a7f37", Dark: "#3fb950"}, // green
}

var accentDefault = lipgloss.AdaptiveColor{Light: "#0a7f8a", Dark: "#00b4d8"}

func categoryAccent(key string) lipgloss.AdaptiveColor {
	if c, ok := categoryAccents[key]; ok {
		return c
	}
	return accentDefault
}

func accentStyle(key string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(categoryAccent(key)).Bold(true)
}
