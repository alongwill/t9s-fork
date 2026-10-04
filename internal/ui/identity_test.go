package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/config"
	"github.com/florianspk/t9s/internal/talos"
)

func appFromYAML(t *testing.T, y, ctxName string) App {
	t.Helper()
	cfg, err := config.Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	app := New(cfg, "", ctxName)
	app.width, app.height = 140, 40
	return app
}

const omniSideroV1YAML = `
context: omni
contexts:
  omni:
    endpoints: ["https://acme.omni.siderolabs.io"]
    auth:
      siderov1:
        identity: a@example.com
  plain:
    endpoints: ["10.0.0.1"]
  endpointonly:
    endpoints: ["https://omni.corp.example"]
`

func TestOmniDetectedAtStartupFromSideroV1(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "omni")
	if !app.id.omni.Detected || app.id.omni.Via != config.ViaSideroV1 {
		t.Fatalf("id = %+v", app.id)
	}
	h := app.renderHeader()
	if !strings.Contains(h, "Omni acme.omni.siderolabs.io") {
		t.Errorf("header lacks the Omni badge:\n%s", h)
	}
	help := buildHelpContentFor(app)
	if !strings.Contains(help, "detected via talosconfig siderov1") {
		t.Error("help status must say how Omni was detected")
	}
}

func TestOmniDetectedAtStartupFromEndpoint(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "endpointonly")
	if !app.id.omni.Detected || app.id.omni.Via != config.ViaEndpoint {
		t.Fatalf("id = %+v", app.id)
	}
	if !strings.Contains(buildHelpContentFor(app), "detected via talosconfig endpoint") {
		t.Error("help must name the endpoint path")
	}
}

func TestPlainTalosBadge(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "plain")
	if app.id.omni.Detected {
		t.Fatal("plain context detected as Omni")
	}
	h := app.renderHeader()
	if !strings.Contains(h, "Talos") || strings.Contains(h, "Omni") {
		t.Errorf("header = %q", h)
	}
	if !strings.Contains(buildHelpContentFor(app), "no Omni detected") {
		t.Error("help status must say no Omni was detected")
	}
}

// fakeProbeSource answers List/GetYAML for the SideroLink status type.
type fakeProbeSource struct {
	talos.ResourceSource
	items []talos.ResourceMeta
	yaml  string
	err   error
	calls int
}

func (f *fakeProbeSource) List(_ context.Context, _, ns, typ string) ([]talos.ResourceMeta, error) {
	f.calls++
	if ns != "config" || typ != siderolinkStatusType {
		return nil, errors.New("unexpected list")
	}
	return f.items, f.err
}

func (f *fakeProbeSource) GetYAML(context.Context, string, string, string, string) (string, error) {
	return f.yaml, nil
}

func runProbe(t *testing.T, app App, src *fakeProbeSource) App {
	t.Helper()
	app.source = src
	app.nodes = makeNodes(2)
	app, cmd := app.probeOmni()
	if cmd == nil {
		t.Fatal("probe did not start")
	}
	msg, ok := cmd().(omniProbeMsg)
	if !ok {
		t.Fatal("probe returned the wrong message")
	}
	return app.handleOmniProbe(msg)
}

func TestOmniDetectedLazilyFromSideroLinkStatus(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "plain")
	src := &fakeProbeSource{
		items: []talos.ResourceMeta{{ID: "siderolink-status"}},
		// the join token in the query must never reach the screen
		yaml: "node: 10.0.0.1\nspec:\n    host: https://acme.omni.example:8090?jointoken=SECRET\n    connected: true\n",
	}
	app = runProbe(t, app, src)
	if !app.id.omni.Detected || app.id.omni.Via != config.ViaSideroLink || app.id.omni.Host != "acme.omni.example" {
		t.Fatalf("id = %+v", app.id)
	}
	if h := app.renderHeader(); !strings.Contains(h, "Omni acme.omni.example") || strings.Contains(h, "SECRET") {
		t.Errorf("header = %q", h)
	}
	if !strings.Contains(buildHelpContentFor(app), "detected via SideroLink status") {
		t.Error("help must name the SideroLink path")
	}
	if _, cmd := app.probeOmni(); cmd != nil {
		t.Error("probe must not run again once detected")
	}
}

func TestOmniProbeNotFoundStaysTalosAndRunsOnce(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "plain")
	src := &fakeProbeSource{}
	app = runProbe(t, app, src)
	if app.id.omni.Detected {
		t.Fatal("no status instance must not detect Omni")
	}
	if _, cmd := app.probeOmni(); cmd != nil {
		t.Error("probe must run once per context")
	}
}

func TestOmniProbeErrorRetries(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "plain")
	app = runProbe(t, app, &fakeProbeSource{err: errors.New("unavailable")})
	if app.id.omni.Detected || app.omniProbed {
		t.Fatal("a failed probe must not conclude")
	}
	app.nodes = makeNodes(1)
	if _, cmd := app.probeOmni(); cmd == nil {
		t.Error("a failed probe must retry after the next node refresh")
	}
}

func TestOmniProbeStaleContextDropped(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "plain")
	app.omniProbing = true
	got := app.handleOmniProbe(omniProbeMsg{ctx: "other", found: true, host: "x"})
	if got.id.omni.Detected {
		t.Error("a reply for another context must be ignored")
	}
}

func TestContextSwitchResetsIdentity(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "omni")
	app.state = StateContextSwitcher
	app.contexts = app.cfg.ContextNames()
	for i, c := range app.contexts {
		if c == "plain" {
			app.ctxCur = i
		}
	}
	app.omniProbed = true
	got, _ := app.handleContextsKey(runeEnter())
	if got.id.omni.Detected || got.omniProbed {
		t.Errorf("switching to a plain context kept Omni: %+v probed=%v", got.id, got.omniProbed)
	}
}

func TestHeaderFitsNarrowTerminals(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "omni")
	app.nodes = makeNodes(1)
	app.selNode = &app.nodes[0]
	for _, w := range []int{60, 80, 100, 140, 200} {
		app.width = w
		for i, line := range strings.Split(app.renderHeader(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("width %d: header line %d is %d cells", w, i, got)
			}
		}
	}
}

func TestSiderolinkHost(t *testing.T) {
	cases := map[string]string{
		"spec:\n  host: omni.example.com:8090\n":                  "omni.example.com",
		"spec:\n  host: https://omni.example.com?jointoken=abc\n": "omni.example.com",
		"spec:\n  host: \"\"\n":                                   "",
		"not yaml: [":                                             "",
		"spec:\n  other: 1\n":                                     "",
	}
	for in, want := range cases {
		if got := siderolinkHost(in); got != want {
			t.Errorf("siderolinkHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func runeEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
