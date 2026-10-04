package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/talos"
)

func TestPadlockIsTwoCells(t *testing.T) {
	t.Setenv("T9S_ASCII", "")
	if got := padlock(); got != "🔒" || lipgloss.Width(got) != 2 {
		t.Errorf("padlock = %q, %d cells", got, lipgloss.Width(got))
	}
	if padlockRunes() != 1 {
		t.Errorf("padlockRunes = %d", padlockRunes())
	}
}

func TestPadlockASCIIFallback(t *testing.T) {
	t.Setenv("T9S_ASCII", "1")
	if got := padlock(); got != "[locked]" {
		t.Errorf("padlock = %q", got)
	}
	b := browser{counts: map[string]int{"T": countLocked}}
	if text, dim := b.typeCell(talos.ResourceDef{Type: "T"}); text != "[locked]" || !dim {
		t.Errorf("typeCell = %q, %v", text, dim)
	}
	if relCellText(relCell{state: cellLocked}, false) != "[locked]" {
		t.Error("related cell must use the ASCII padlock")
	}
	if (cmpRow{state: cmpLocked}).presentText() != "[locked]" {
		t.Error("compare row must use the ASCII padlock")
	}
}

func TestLockReasonOmniVsRole(t *testing.T) {
	omni := appFromYAML(t, omniSideroV1YAML, "omni")
	if got := omni.lockReason(); got != "Omni does not forward reads of sensitive resources such as MachineConfig, whatever your role" {
		t.Errorf("omni reason = %q", got)
	}
	if got := omni.lockShort(); !strings.Contains(got, "Omni") {
		t.Errorf("omni short = %q", got)
	}

	reader := appFromYAML(t, certYAML(t, []string{"os:reader"}, farFuture()), "c")
	if got := reader.lockReason(); got != "needs os:admin (you have os:reader)" {
		t.Errorf("reader reason = %q", got)
	}
	op := appFromYAML(t, certYAML(t, []string{"os:operator", "os:etcd:backup"}, farFuture()), "c")
	if got := op.lockReason(); got != "needs os:admin (you have os:operator, os:etcd:backup)" {
		t.Errorf("operator reason = %q", got)
	}
	plain := appFromYAML(t, omniSideroV1YAML, "plain")
	if got := plain.lockReason(); got != "needs os:admin" {
		t.Errorf("no-cert reason = %q", got)
	}
	if !strings.HasPrefix(ansi.Strip(reader.lockMessage()), padlock()+" needs os:admin") {
		t.Errorf("lockMessage = %q", reader.lockMessage())
	}
}

// A types pane with a locked row: the padlock must end in the same cell as
// the counts of the other rows (right-aligned in the count column) and no line
// may exceed the terminal width, whichever padlock is in use.
func TestLockedRowAlignedInTypesPane(t *testing.T) {
	for _, ascii := range []string{"", "1"} {
		t.Setenv("T9S_ASCII", ascii)
		app := browserApp(120, 40, 2, 6)
		app.browser = app.browser.setCount("Thing03.net.talos.dev", countLocked)
		out := app.renderBrowser(app.mainHeight())
		ends := map[int][]string{}
		sawLock := false
		for i, l := range strings.Split(out, "\n") {
			if w := lipgloss.Width(l); w > 120 {
				t.Errorf("ascii=%q: line %d is %d cells", ascii, i, w)
			}
			plain := ansi.Strip(l)
			at := strings.Index(plain, "Thing")
			if at < 0 {
				continue
			}
			border := strings.Index(plain[at:], "│")
			if border < 0 {
				continue
			}
			seg := strings.TrimRight(plain[:at+border], " ")
			ends[lipgloss.Width(seg)] = append(ends[lipgloss.Width(seg)], strings.TrimSpace(plain[at:at+border]))
			if strings.Contains(seg, padlock()) {
				sawLock = true
			}
		}
		if !sawLock {
			t.Fatalf("ascii=%q: no locked row rendered:\n%s", ascii, ansi.Strip(out))
		}
		if len(ends) != 1 {
			t.Errorf("ascii=%q: count column is not aligned, row ends: %v", ascii, ends)
		}
	}
}

func TestEnterOnLockedTypeExplainsWhy(t *testing.T) {
	for _, tc := range []struct{ yaml, ctx, want string }{
		{omniSideroV1YAML, "omni", "Omni does not forward reads of sensitive resources"},
		{certYAML(t, []string{"os:reader"}, farFuture()), "c", "needs os:admin (you have os:reader)"},
	} {
		app := appFromYAML(t, tc.yaml, tc.ctx)
		app.width, app.height = 120, 40
		d := talos.ResourceDef{Type: "Secrets.secrets.talos.dev", DisplayType: "Secret"}
		app.browser.counts = map[string]int{d.Type: countLocked}
		got, _ := app.openType(d)
		plain := ansi.Strip(got.statusMsg)
		if !strings.HasPrefix(plain, padlock()+" ") || !strings.Contains(plain, tc.want) {
			t.Errorf("%s: status = %q, want padlock + %q", tc.ctx, plain, tc.want)
		}
	}
}

func TestMachineConfigDeniedShowsPadlockPanel(t *testing.T) {
	for _, tc := range []struct{ yaml, ctx, want string }{
		{omniSideroV1YAML, "omni", "Omni does not forward reads of sensitive resources"},
		{certYAML(t, []string{"os:reader"}, farFuture()), "c", "needs os:admin (you have os:reader)"},
	} {
		app := appFromYAML(t, tc.yaml, tc.ctx)
		app.nodes = makeNodes(1)
		app.selNode = &app.nodes[0]
		app.state = StateMachineConfig
		app.machLoading = true
		m, _ := app.Update(machineConfigLoadedMsg{err: errPermissionDenied()})
		app = m.(App)
		out := ansi.Strip(app.renderMachineConfig(app.mainHeight()))
		if !strings.Contains(out, padlock()+" Machine config is locked") || !strings.Contains(out, tc.want) {
			t.Errorf("%s: panel lacks the padlock text:\n%s", tc.ctx, out)
		}
		if strings.Contains(out, "Error:") || strings.Contains(out, "No machine config") {
			t.Errorf("%s: panel shows an error:\n%s", tc.ctx, out)
		}
		if !strings.Contains(ansi.Strip(app.statusMsg), padlock()) {
			t.Errorf("%s: status = %q", tc.ctx, app.statusMsg)
		}
	}
}

func TestMachineConfigOtherErrorStaysAnError(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "plain")
	app.state = StateMachineConfig
	m, _ := app.Update(machineConfigLoadedMsg{err: errTimeout()})
	app = m.(App)
	if app.machDenied || !strings.Contains(app.statusMsg, "Error:") {
		t.Errorf("denied=%v status=%q", app.machDenied, app.statusMsg)
	}
}

func TestStatusLineIsClippedToWidth(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "omni")
	app.width = 60
	app.statusMsg = app.lockMessage()
	for i, l := range strings.Split(app.renderFooter(), "\n") {
		if w := lipgloss.Width(l); w > 60 {
			t.Errorf("footer line %d is %d cells", i, w)
		}
	}
}

func farFuture() time.Time { return time.Now().Add(400 * 24 * time.Hour) }

func errPermissionDenied() error {
	return errors.New("exit status 1: rpc error: code = PermissionDenied desc = not authorized")
}

func errTimeout() error { return errors.New("context deadline exceeded") }
