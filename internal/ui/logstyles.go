package ui

import "github.com/charmbracelet/lipgloss"

// Styles for the log view, built from the palette in styles.go.
var (
	logErrStyle  = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	logWarnStyle = lipgloss.NewStyle().Foreground(colorYellow)
	logInfoStyle = lipgloss.NewStyle().Foreground(colorBlue)
	logDimStyle  = lipgloss.NewStyle().Foreground(colorGray)
	logKeyStyle  = lipgloss.NewStyle().Foreground(colorCyan)
	logFindStyle = lipgloss.NewStyle().Background(colorYellow).Foreground(colorBg).Bold(true)
)

// logSpanStyle returns the style for a span kind. selected styles sit on the
// cursor row's background so the row still reads as one bar.
func logSpanStyle(k logSpanKind, selected bool) lipgloss.Style {
	var st lipgloss.Style
	switch k {
	case spanErr:
		st = logErrStyle
	case spanWarn:
		st = logWarnStyle
	case spanInfo:
		st = logInfoStyle
	case spanDebug, spanTime:
		st = logDimStyle
	case spanKey:
		st = logKeyStyle
	case spanFind:
		return logFindStyle
	}
	if selected {
		st = st.Background(colorBgSel)
	}
	return st
}
