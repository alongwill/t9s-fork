package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/talos"
)

func waitForUpgradeLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return upgradeDoneMsg{}
		}
		return upgradeLineMsg(line)
	}
}

func (app App) handleUpgradeKey(msg tea.KeyMsg) (App, tea.Cmd) {
	if app.upgradeRunning {
		switch msg.String() {
		case "ctrl+c":
			app.cleanup()
			return app, tea.Quit
		case "esc":
			// Navigate back — upgrade keeps running in background.
			app = app.goBack()
		case "up", "k":
			app.upgradeVP.LineUp(1)
		case "down", "j":
			app.upgradeVP.LineDown(1)
		case "pgup":
			app.upgradeVP.HalfViewUp()
		case "pgdown":
			app.upgradeVP.HalfViewDown()
		case "g":
			app.upgradeVP.GotoTop()
		case "G":
			app.upgradeVP.GotoBottom()
		}
		return app, nil
	}

	if app.upgradeConfirm {
		switch msg.String() {
		case "ctrl+c":
			app.cleanup()
			return app, tea.Quit
		case "y":
			return app.startUpgrade()
		case "n", "esc":
			app.upgradeConfirm = false
		}
		return app, nil
	}

	// Input phase
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit

	case "esc":
		app = app.goBack()
		return app, nil

	case "tab":
		// tab, not a letter: letters must reach the input (image refs, digests).
		if app.client.ModernCLI() {
			app.upgradeDrain = !app.upgradeDrain
		} else {
			app.upgradePreserve = !app.upgradePreserve
		}
		return app, nil

	case "enter":
		if strings.TrimSpace(app.upgradeInput.Value()) == "" {
			app.statusMsg = warnStyle.Render("Please enter a value")
			return app, nil
		}
		app.upgradeConfirm = true
		return app, nil
	}

	var cmd tea.Cmd
	app.upgradeInput, cmd = app.upgradeInput.Update(msg)
	return app, cmd
}

func (app App) startUpgrade() (App, tea.Cmd) {
	id := "upgrade-talos"
	if app.upgradeForK8s {
		id = "upgrade-k8s"
	}
	if a, blocked := app.refuseDangerous(id); blocked {
		a.upgradeConfirm = false
		return a, nil
	}
	app.upgradeConfirm = false
	app.upgradeRunning = true
	app.upgradeLines = nil
	app.upgradeVP.SetContent("")

	app.upgradeCh = make(chan string, 500)
	app.upgradeCtx, app.upgradeCancel = context.WithCancel(context.Background())

	val := strings.TrimSpace(app.upgradeInput.Value())
	client := app.client
	node := ""
	if app.selNode != nil {
		node = app.selNode.IP
	}
	forK8s := app.upgradeForK8s
	if forK8s {
		node = app.controlPlaneNode(app.selNode)
	}
	opts := talos.UpgradeOptions{Image: val, Drain: app.upgradeDrain, Preserve: app.upgradePreserve}

	upgradeCh := app.upgradeCh
	upgradeCtx := app.upgradeCtx
	go func() {
		defer close(upgradeCh)
		var err error
		if forK8s {
			err = client.UpgradeK8s(upgradeCtx, node, val, upgradeCh)
		} else {
			err = client.UpgradeTalos(upgradeCtx, node, opts, upgradeCh)
		}
		if err != nil && upgradeCtx.Err() == nil {
			upgradeCh <- fmt.Sprintf("ERROR: %v", err)
		}
	}()

	return app, waitForUpgradeLine(app.upgradeCh)
}

// upgradeFlagNote renders the upgrade toggle as it will be passed to talosctl.
func (app App) upgradeFlagNote() string {
	flag, on := "--preserve", app.upgradePreserve
	if app.client.ModernCLI() {
		flag, on = "--drain", app.upgradeDrain
	}
	if on {
		return okStyle.Render(fmt.Sprintf("%s=true", flag))
	}
	return dimStyle.Render(fmt.Sprintf("%s=false", flag))
}

func (app App) renderUpgrade(height int) string {
	isK8s := app.upgradeForK8s
	kind := "Talos"
	if isK8s {
		kind = "Kubernetes"
	}

	node := ""
	if app.selNode != nil && !isK8s {
		node = fmt.Sprintf(" on %s", titleStyle.Render(app.selNode.Hostname))
	}
	title := fmt.Sprintf("  Upgrade %s%s\n", titleStyle.Render(kind), node)

	if app.upgradeRunning {
		app.upgradeVP.Height = height - 2
		app.upgradeVP.Width = app.width
		return title + app.upgradeVP.View()
	}

	if app.upgradeConfirm {
		val := app.upgradeInput.Value()
		var msg string
		if isK8s {
			msg = fmt.Sprintf("Upgrade Kubernetes to %s?", okStyle.Render(val))
		} else {
			preserveNote := app.upgradeFlagNote()
			msg = fmt.Sprintf("Upgrade Talos on %s to image %s  %s",
				titleStyle.Render(app.selNode.Hostname),
				okStyle.Render(val),
				preserveNote)
		}
		confirm := fmt.Sprintf("\n  %s\n\n  %s  %s",
			msg,
			keyStyle.Render("[y]")+" confirm",
			keyStyle.Render("[n/Esc]")+" cancel",
		)
		return title + lipgloss.Place(app.width, height-2, lipgloss.Center, lipgloss.Center, confirm)
	}

	// Show final output if upgrade completed
	if len(app.upgradeLines) > 0 {
		app.upgradeVP.Height = height - 2
		app.upgradeVP.Width = app.width
		return title + app.upgradeVP.View()
	}

	// Input phase
	label := "Image (e.g. ghcr.io/siderolabs/installer:v1.14.2 or factory.talos.dev/installer/<schematic>:v1.14.2):"
	if isK8s {
		label = "Target Kubernetes version (e.g. 1.37.0):"
	}
	preserveLine := ""
	if !isK8s {
		flag, on, note := "--preserve", app.upgradePreserve, "(required for single-node etcd clusters)"
		if app.client.ModernCLI() {
			flag, on, note = "--drain", app.upgradeDrain, "(cordon + drain the node first; needs Kubernetes)"
		}
		val := dimStyle.Render("off")
		if on {
			val = okStyle.Render("on")
		}
		preserveLine = fmt.Sprintf("\n  %s %s %s  %s\n",
			keyStyle.Render("[tab]"), flag, val, dimStyle.Render(note))
	}
	inputView := fmt.Sprintf("\n  %s\n\n  %s\n%s",
		dimStyle.Render(label),
		app.upgradeInput.View(),
		preserveLine,
	)
	return title + lipgloss.Place(app.width, height-2, lipgloss.Left, lipgloss.Center, inputView)
}
