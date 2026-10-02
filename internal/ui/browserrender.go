package ui

import "strings"

// renderBrowser is a stub: titles only. Replaced by the real renderer.
func (app App) renderBrowser(height int) string {
	var sb strings.Builder
	for _, p := range app.browser.stack {
		sb.WriteString(p.title + "\n")
	}
	return sb.String()
}
