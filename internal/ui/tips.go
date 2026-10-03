package ui

import (
	"math/rand"

	"github.com/charmbracelet/lipgloss"
)

// browserTips are one-line hints shown in the status line when the browser or
// a category opens. They cover browser features and Talos concepts; keep each
// to one line at 80 columns.
var browserTips = []string{
	"Config is what you asked for; Specs are what Talos decided; Statuses are what the kernel reports",
	"ctrl+a lists every type on this node",
	":net jumps straight to Networking",
	"d explains the selected type: what it is, its Ubuntu equivalent, what feeds it",
	"Greyed rows have no instances on this node",
	"/ filters the current pane; a leading ! inverts the match",
	"c compares one resource across every node",
	"W toggles a live watch of the open instance list (gRPC source)",
	"Esc clears a filter first, then goes back one pane",
	"ctrl+r reloads the pane you are looking at",
	"In a YAML pane, / searches and n / N step through the matches",
	"f makes the YAML pane full screen; w wraps long lines",
	"A resource's owner is the controller that wrote it; d shows it for an instance",
	"A *Spec is usually the input of a controller; the matching *Status is its output",
	"Talos has no shell or SSH: resources are how you look inside a node",
	"A lock row needs os:admin: its type is marked sensitive",
	"The CONFIG section lists the machine config documents on this node",
	":nodes goes back to the node list; :aliases opens the all types palette",
	"q goes back like Esc; on the first pane it returns to the node list",
	"Types with one instance open straight to its YAML",
	"Press ? for every key; :tips off silences these tips",
}

// tipStart picks where tip rotation begins; tests replace it.
var tipStart = func(n int) int { return rand.Intn(n) }

// nextTip returns the next tip in order and the new rotation index. A negative
// idx means rotation has not started: begin at a random tip.
func nextTip(idx int) (string, int) {
	if idx < 0 {
		idx = tipStart(len(browserTips))
	}
	idx %= len(browserTips)
	return browserTips[idx], idx + 1
}

// showTip puts the next tip in the status line unless tips are off or another
// message is showing. A tip left from earlier counts as no message.
func (app App) showTip() App {
	if app.tipsOff {
		return app
	}
	if app.statusMsg != "" && app.statusMsg != app.tipMsg {
		return app
	}
	tip, next := nextTip(app.tipIdx)
	app.tipIdx = next
	text := "tip: " + tip
	if app.width > 8 {
		if lipgloss.Width(text) > app.width-4 {
			text = cutWidth(text, app.width-5) + "…"
		}
	}
	app.tipMsg = dimStyle.Render(text)
	app.statusMsg = app.tipMsg
	return app
}

// setTips handles `:tips on` and `:tips off` (this session only).
func (app App) setTips(on bool) App {
	app.tipsOff = !on
	if on {
		app.statusMsg = dimStyle.Render("tips on")
	} else {
		app.statusMsg = dimStyle.Render("tips off")
	}
	return app
}
