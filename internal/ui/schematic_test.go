package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/talos"
)

var schemID = strings.Repeat("cd", 32)

func extApp(t *testing.T, apiURL string) App {
	t.Helper()
	app := newTestApp(80, 24)
	app.nodes = makeNodes(1)
	app.selNode = &app.nodes[0]
	app.state = StateExtensions
	app.extensions = []talos.Extension{{Name: "iscsi-tools", Version: "v0.1.6"}}
	app.extSchem = talos.SchematicInfo{ID: schemID, Flavor: "metal", APIURL: apiURL}
	app.extSchemOK = true
	return app
}

func runSchematic(t *testing.T, app App) App {
	t.Helper()
	app, cmd := app.handleExtensionsKey(runeEnter())
	if app.state != StateSchematic || cmd == nil {
		t.Fatalf("enter: state=%v cmd=%v", app.state, cmd)
	}
	msg, ok := cmd().(schematicMsg)
	if !ok {
		t.Fatal("wrong message")
	}
	return app.handleSchematic(msg)
}

func TestExtensionsHeaderLineAndRender(t *testing.T) {
	app := extApp(t, "https://factory.example.com")
	out := ansi.Strip(app.renderExtensions(app.mainHeight()))
	if !strings.Contains(out, "schematic "+schemID[:12]+" · factory.example.com") {
		t.Errorf("header line missing:\n%s", out)
	}
	app.extSchemOK = false
	if strings.Contains(ansi.Strip(app.renderExtensions(app.mainHeight())), "schematic ") {
		t.Error("header line shown without a schematic")
	}
}

func TestSchematicPaneShowsYAMLAt80x24(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("customization:\n  systemExtensions:\n    officialExtensions:\n      - siderolabs/iscsi-tools\n"))
	}))
	defer srv.Close()
	app := runSchematic(t, extApp(t, srv.URL))
	if app.state != StateSchematic {
		t.Fatal("not in the schematic pane")
	}
	view := ansi.Strip(app.View())
	for _, want := range []string{"Schematic " + schemID[:12], schemID, "metal", srv.URL, "officialExtensions", "siderolabs/iscsi-tools"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
	lines := strings.Split(app.View(), "\n")
	if len(lines) > 24 {
		t.Errorf("view is %d lines, want <= 24", len(lines))
	}
	for i, l := range lines {
		if lipgloss.Width(l) > 80 {
			t.Errorf("line %d is %d cells: %q", i, lipgloss.Width(l), ansi.Strip(l))
		}
	}
}

func TestSchematicAuthAndNetworkErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(403) }))
	app := runSchematic(t, extApp(t, srv.URL))
	srv.Close()
	view := ansi.Strip(app.View())
	if !strings.Contains(strings.ReplaceAll(view, "\n", ""), padlock()+" the factory needs authentication; schematic ID "+schemID) {
		t.Errorf("auth message missing:\n%s", view)
	}

	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL
	dead.Close() // connection refused
	other := strings.Repeat("ef", 32)
	a := extApp(t, url)
	a.extSchem.ID = other
	a = runSchematic(t, a)
	view = ansi.Strip(a.View())
	for _, want := range []string{"could not fetch the schematic", "schematic ID " + other, "URL " + url + "/schematics/" + other} {
		if !strings.Contains(strings.ReplaceAll(view, "\n", ""), want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
}

func TestSchematicEscReturnsToOriginAndDropsStaleReply(t *testing.T) {
	app := extApp(t, "https://f.example")
	app, _ = app.handleExtensionsKey(runeEnter())
	seq := app.schemSeq
	app, _ = app.handleSchematicKey(runeKey("q"))
	if app.state != StateExtensions {
		t.Errorf("state = %v, want extensions", app.state)
	}
	got := app.handleSchematic(schematicMsg{seq: seq, yaml: "late: reply\n", hasInfo: true})
	if got.schemYAML != "" {
		t.Error("a stale reply was applied")
	}
}

func TestDescribeSchematicLineAndYOpensPane(t *testing.T) {
	app := browserApp(120, 40, 2, 3)
	d := talos.ResourceDef{Type: talos.SchematicType, DisplayType: "ImageFactorySchematic", DefaultNamespace: "runtime"}
	app.browser = app.browser.push(pane{kind: paneDescribe, title: d.DisplayType, sub: descSubject{def: d}})
	p, _ := app.browser.top()
	if text := describeText(app, p, 100); !strings.Contains(text, "y on the instance shows the factory's YAML") {
		t.Errorf("describe lacks the y line:\n%s", text)
	}
	got, cmd := app.describeToYAML()
	if got.state != StateSchematic || cmd == nil || got.schemOrigin != app.state {
		t.Errorf("y: state=%v origin=%v cmd=%v", got.state, got.schemOrigin, cmd)
	}
	got, _ = got.handleSchematicKey(runeKey("esc"))
	_ = got
}

func TestSchematicHintsAndHelpNotBroken(t *testing.T) {
	app := extApp(t, "https://f.example")
	var found bool
	for _, h := range stateHints(app) {
		found = found || h.desc == "Schematic YAML"
	}
	if !found {
		t.Error("extensions hints lack the schematic key")
	}
	app.state = StateSchematic
	if len(stateHints(app)) == 0 || viewTitle(StateSchematic) == "" {
		t.Error("schematic state lacks hints or a title")
	}
}
