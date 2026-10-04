package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// dangerousAction is a key that changes a cluster. In read-only mode (the
// default) it is hidden from hints and help, and pressing it only explains how
// to enable it (k9s ClearDanger). The client refuses the same calls too.
type dangerousAction struct {
	id   string // stable name used by the guard
	keys string // key as shown in help
	view string // where the key works
	verb string // "reboot a node": completes "start t9s with --write to …"
	desc string // help text when writes are enabled
}

// dangerousActions is the single list of every UI action that mutates a
// cluster. Add a new mutating key here and call app.refuseDangerous(id) in its
// handler; a test checks the list against the Client's mutating methods.
var dangerousActions = []dangerousAction{
	{"reboot", "R", "node list", "reboot a node", "Reboot node"},
	{"shutdown", "S", "node list", "shut down a node", "Shutdown node"},
	{"upgrade-talos", "U", "node list", "upgrade Talos", "Upgrade Talos"},
	{"upgrade-k8s", "K", "node list", "upgrade Kubernetes", "Upgrade Kubernetes"},
	{"edit-config", "e", "machine config", "edit and apply the machine config", "Edit & apply"},
}

func dangerousByID(id string) (dangerousAction, bool) {
	for _, a := range dangerousActions {
		if a.id == id {
			return a, true
		}
	}
	return dangerousAction{}, false
}

// WithWrite turns write mode on or off for the app and its client. The zero
// App is read-only.
func (app App) WithWrite(write bool) App {
	app.writeMode = write
	if app.client != nil {
		app.client.SetWritable(write)
	}
	return app
}

// ReadOnly reports whether mutating actions are disabled.
func (app App) ReadOnly() bool { return !app.writeMode }

// refuseDangerous reports whether the action must be blocked. When it is, the
// status line says how to enable it and the caller must do nothing else.
func (app App) refuseDangerous(id string) (App, bool) {
	if app.writeMode {
		return app, false
	}
	verb := id
	if a, ok := dangerousByID(id); ok {
		verb = a.verb
	}
	app.statusMsg = warnStyle.Render("read-only mode: start t9s with --write to " + verb)
	return app, true
}

// modeBadge is the top-bar badge: RO on green, RW (bold) on red.
func (app App) modeBadge() string {
	if app.writeMode {
		return lipgloss.NewStyle().Background(colorRed).Foreground(lipgloss.Color("#0d1117")).Bold(true).Render(" RW ")
	}
	return lipgloss.NewStyle().Background(colorGreen).Foreground(lipgloss.Color("#0d1117")).Render(" RO ")
}
