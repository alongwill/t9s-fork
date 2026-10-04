package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/config"
	"github.com/florianspk/t9s/internal/talos"
)

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func roApp(t *testing.T) App {
	t.Helper()
	cfg := &config.TalosConfig{Context: "c", Contexts: map[string]config.Context{"c": {}}}
	app := New(cfg, "", "c")
	app.width, app.height = 120, 40
	app.nodes = makeNodes(2)
	return app
}

func TestNewAppIsReadOnly(t *testing.T) {
	app := roApp(t)
	if !app.ReadOnly() || !app.client.ReadOnly() {
		t.Fatal("a new app and its client must be read-only")
	}
	w := app.WithWrite(true)
	if w.ReadOnly() || w.client.ReadOnly() {
		t.Fatal("WithWrite(true) must enable writes on the app and the client")
	}
}

func TestDangerousKeysRefusedInReadOnly(t *testing.T) {
	for _, a := range dangerousActions {
		if a.view != "node list" {
			continue
		}
		app := roApp(t)
		got, cmd := app.handleNodeListKey(runeKey(a.keys))
		if cmd != nil {
			t.Errorf("%s: read-only key press returned a command", a.keys)
		}
		if got.state != StateNodeList {
			t.Errorf("%s: left the node list (state %v)", a.keys, got.state)
		}
		if got.pendingAction != "" {
			t.Errorf("%s: opened a confirmation in read-only mode", a.keys)
		}
		want := "read-only mode: start t9s with --write to " + a.verb
		if !strings.Contains(got.statusMsg, want) {
			t.Errorf("%s: status %q does not contain %q", a.keys, got.statusMsg, want)
		}
	}
}

func TestDangerousKeysWorkInWriteMode(t *testing.T) {
	app := roApp(t).WithWrite(true)
	got, _ := app.handleNodeListKey(runeKey("R"))
	if got.pendingAction != "reboot" {
		t.Errorf("R in write mode: pendingAction = %q, want reboot", got.pendingAction)
	}
	got, _ = app.handleNodeListKey(runeKey("S"))
	if got.pendingAction != "shutdown" {
		t.Errorf("S in write mode: pendingAction = %q, want shutdown", got.pendingAction)
	}
	got, _ = app.handleNodeListKey(runeKey("U"))
	if got.state != StateUpgradeTalos {
		t.Errorf("U in write mode: state = %v, want StateUpgradeTalos", got.state)
	}
}

func TestUpgradeAndConfirmRefusedEvenIfReached(t *testing.T) {
	app := roApp(t)
	app.selNode = &app.nodes[0]
	app.state = StateUpgradeTalos
	app.upgradeConfirm = true
	got, cmd := app.startUpgrade()
	if cmd != nil || got.upgradeRunning {
		t.Error("startUpgrade must not run in read-only mode")
	}
	app.pendingAction = "reboot"
	got2, cmd2 := app.handleKey(runeKey("y"))
	if cmd2 != nil || got2.pendingAction != "" {
		t.Error("confirming a pending reboot must not run in read-only mode")
	}
	if !strings.Contains(got2.statusMsg, "--write") {
		t.Errorf("status %q should explain --write", got2.statusMsg)
	}
}

func TestMachineConfigEditHiddenAndRefusedInReadOnly(t *testing.T) {
	app := roApp(t)
	app.state = StateMachineConfig
	app.selNode = &app.nodes[0]
	for _, h := range stateHints(app) {
		if h.key == "e" {
			t.Error("e hint shown in read-only mode")
		}
	}
	got, cmd := app.handleMachineConfigKey(runeKey("e"))
	if cmd != nil || got.machEditFile != "" {
		t.Error("e must not start an edit in read-only mode")
	}
	if !strings.Contains(got.statusMsg, "read-only mode: start t9s with --write to edit and apply the machine config") {
		t.Errorf("status = %q", got.statusMsg)
	}
	w := app.WithWrite(true)
	foundE := false
	for _, h := range stateHints(w) {
		if h.key == "e" {
			foundE = true
		}
	}
	if !foundE {
		t.Error("e hint missing in write mode")
	}
}

func TestHelpHidesDangerousKeysInReadOnly(t *testing.T) {
	ro := buildHelpContentFor(roApp(t))
	for _, bad := range []string{"Reboot node", "Shutdown node", "Upgrade Talos", "Upgrade Kubernetes", "Edit and apply"} {
		if strings.Contains(ro, bad) {
			t.Errorf("read-only help lists %q", bad)
		}
	}
	if !strings.Contains(ro, "read-only") {
		t.Error("help status section must say the mode")
	}
	rw := buildHelpContentFor(roApp(t).WithWrite(true))
	for _, want := range []string{"Reboot node", "Shutdown node", "Upgrade Talos", "Upgrade Kubernetes", "Edit and apply"} {
		if !strings.Contains(rw, want) {
			t.Errorf("write-mode help lacks %q", want)
		}
	}
}

func TestModeBadge(t *testing.T) {
	app := roApp(t)
	if b := app.modeBadge(); !strings.Contains(b, "RO") || strings.Contains(b, "RW") {
		t.Errorf("read-only badge = %q", b)
	}
	if b := app.WithWrite(true).modeBadge(); !strings.Contains(b, "RW") {
		t.Errorf("write badge = %q", b)
	}
	if h := app.renderHeader(); !strings.Contains(h, "RO") {
		t.Error("header lacks the RO badge")
	}
	if h := app.WithWrite(true).renderHeader(); !strings.Contains(h, "RW") {
		t.Error("header lacks the RW badge")
	}
}

// Every UI action marked dangerous maps to something a client method guards:
// the two lists are the audit trail for "what can change a cluster".
func TestDangerousActionsCoverClientMutations(t *testing.T) {
	covered := map[string]string{
		"reboot": "Reboot", "shutdown": "Shutdown",
		"upgrade-talos": "UpgradeTalos", "upgrade-k8s": "UpgradeK8s",
		"edit-config": "ApplyConfig",
	}
	for _, a := range dangerousActions {
		if covered[a.id] == "" {
			t.Errorf("dangerous action %s has no client method mapping", a.id)
		}
		delete(covered, a.id)
	}
	for id := range covered {
		t.Errorf("mapping %s has no dangerous action", id)
	}
	// PatchMachineConfig has no UI key; the client guard still covers it.
	noKey := map[string]bool{"PatchMachineConfig": true}
	have := map[string]bool{"Reboot": true, "Shutdown": true, "UpgradeTalos": true, "UpgradeK8s": true, "ApplyConfig": true}
	for _, m := range talos.MutatingMethods {
		if !have[m] && !noKey[m] {
			t.Errorf("client mutating method %s has no dangerous UI action", m)
		}
	}
}
