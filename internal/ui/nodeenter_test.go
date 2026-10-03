package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNodeListEnterOpensContainersAndSOpensServices(t *testing.T) {
	cases := []struct {
		key  tea.KeyMsg
		want AppState
	}{
		{tea.KeyMsg{Type: tea.KeyEnter}, StateContainers},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, StateContainers},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}, StateServices},
	}
	for _, tc := range cases {
		app := newTestApp(120, 40)
		app.state = StateNodeList
		app.nodes = makeNodes(2)
		got, _ := app.handleNodeListKey(tc.key)
		if got.state != tc.want {
			t.Errorf("key %q: state %v, want %v", tc.key.String(), got.state, tc.want)
		}
		if got.selNode == nil {
			t.Errorf("key %q: no node selected", tc.key.String())
		}
	}
}
