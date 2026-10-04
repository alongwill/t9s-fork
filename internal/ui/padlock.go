package ui

import (
	"os"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// padlock marks anything whose read was denied. It is 2 cells wide
// (lipgloss.Width); T9S_ASCII=1 swaps it for "[locked]" for terminals without
// emoji. Columns that show it size themselves with lipgloss.Width(padlock()).
func padlock() string {
	if os.Getenv("T9S_ASCII") == "1" {
		return "[locked]"
	}
	return "🔒"
}

// padlockRunes is the number of runes of the padlock, for the rune-indexed
// span painter (one rune is 2 cells for the emoji).
func padlockRunes() int { return utf8.RuneCountInString(padlock()) }

// lockReason says why a read was denied. On Omni the role is irrelevant:
// Omni's sensitive read guard refuses those types for every user. Elsewhere
// the Talos API needs os:admin; the message names the role the client
// certificate gives when it is known.
func (app App) lockReason() string {
	if app.id.viaOmni() {
		return "Omni does not forward reads of sensitive resources such as MachineConfig, whatever your role"
	}
	if app.id.hasCert {
		return "needs os:admin (you have " + app.id.roleSummary() + ")"
	}
	return "needs os:admin"
}

// lockShort is lockReason for narrow places (rows, hint line).
func (app App) lockShort() string {
	if app.id.viaOmni() {
		return "not forwarded by Omni"
	}
	return "needs os:admin"
}

// lockMessage is the status-line text for a denied row: padlock plus reason.
func (app App) lockMessage() string {
	return warnStyle.Render(padlock() + " " + app.lockReason())
}

// lockNote is the short reason for a padlock row, "needs os:admin" until the
// renderer has set it from the app's identity.
func (b browser) lockNote() string {
	if b.lockWhy != "" {
		return b.lockWhy
	}
	return "needs os:admin"
}

// lockPanel fills a view with the padlock and why the read was denied, in
// place of an error.
func (app App) lockPanel(height int) string {
	body := warnStyle.Bold(true).Render(padlock()+" Machine config is locked") + "\n\n" +
		dimStyle.Render(app.lockReason())
	return lipgloss.Place(app.width, max(1, height), lipgloss.Center, lipgloss.Center, body)
}
