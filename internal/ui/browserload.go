package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

func (app App) loadResourceDefs() tea.Cmd                                  { return nil }
func (app App) loadCounts(types []talos.ResourceDef) tea.Cmd               { return nil }
func (app App) loadInstances(d talos.ResourceDef) tea.Cmd                  { return nil }
func (app App) loadYAML(d talos.ResourceDef, m talos.ResourceMeta) tea.Cmd { return nil }
