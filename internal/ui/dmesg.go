package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func waitForDmesgLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return dmesgDoneMsg{}
		}
		return dmesgLineMsg(line)
	}
}

func (app App) handleDmesgKey(msg tea.KeyMsg) (App, tea.Cmd) {
	return app.handleLogPaneKey(msg, logKindDmesg)
}

func (app App) renderDmesg(height int) string {
	node := ""
	if app.selNode != nil {
		node = app.selNode.Hostname
	}
	streaming := dimStyle.Render(" [stopped]")
	if app.dmesgStreaming {
		streaming = infoStyle.Render(" [streaming]")
	}
	title := fmt.Sprintf("  Dmesg: %s%s\n", titleStyle.Render(node), streaming)
	return app.renderLogPane(app.logPaneFor(logKindDmesg), title, "Waiting for dmesg…", height)
}
