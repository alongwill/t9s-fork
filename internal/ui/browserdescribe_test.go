package ui

import (
	"strings"
	"testing"
)

// describeText is the pane's wrapped text, labels included, for assertions.
func describeText(app App, p pane, w int) string {
	var sb strings.Builder
	for _, l := range describeVisual(app.describeRows(p), w) {
		sb.WriteString(l.label + " " + l.text + "\n")
	}
	return sb.String()
}

func TestDescribeConfigKindText(t *testing.T) {
	app := cfgApp(120, 40, 3)
	app = press(t, app, "j", "d") // LinkConfig
	p, ok := app.browser.top()
	if !ok || p.kind != paneDescribe {
		t.Fatalf("top = %+v", p)
	}
	text := describeText(app, p, 80)
	ck, _ := findConfigKind("LinkConfig")
	for _, want := range []string{"LinkConfig", "SINCE", ck.Since, "Networking", "network", "2 document(s)", strings.Fields(ck.Desc)[0]} {
		if !strings.Contains(text, want) {
			t.Errorf("describe text missing %q:\n%s", want, text)
		}
	}
	if got := app.breadcrumb(); !strings.HasSuffix(got, "Networking > describe") {
		t.Errorf("breadcrumb = %q", got)
	}
}

func TestDescribeResourceShowsDefinitionFields(t *testing.T) {
	app := cfgApp(120, 60, 3)
	nCfg := len(app.browser.configEntries(testNet, ""))
	for i := 0; i < nCfg; i++ {
		app = press(t, app, "j")
	}
	app = press(t, app, "d") // Thing00
	p, _ := app.browser.top()
	text := describeText(app, p, 100)
	for _, want := range []string{"Thing00.net.talos.dev", "aliases thing00", "ns network", "CATEGORY", "ON THIS NODE", "loading relationships"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
}

func TestDescribeToggleAndClose(t *testing.T) {
	app := browserApp(120, 40, 2, 5)
	app = press(t, app, "d")
	if p, _ := app.browser.top(); p.kind != paneDescribe || len(app.browser.stack) != 3 {
		t.Fatalf("d did not open describe: stack=%d", len(app.browser.stack))
	}
	app = press(t, app, "d") // d again: back
	if p, _ := app.browser.top(); p.kind != paneTypes || len(app.browser.stack) != 2 {
		t.Fatalf("d did not toggle describe off: %+v", p)
	}
	for _, k := range []string{"esc", "q"} {
		a := press(t, app, "d", k)
		if p, _ := a.browser.top(); p.kind != paneTypes {
			t.Errorf("%s did not close describe", k)
		}
	}
	// the categories pane has no describe
	c := press(t, browserApp(120, 40, 1, 5), "d")
	if len(c.browser.stack) != 1 {
		t.Error("d must not open describe on the categories pane")
	}
}

func TestDescribeYSwitchesToYAML(t *testing.T) {
	// instances pane: y opens the YAML of the selected instance
	app := browserApp(120, 40, 3, 5)
	app = press(t, app, "d", "y")
	if p, _ := app.browser.top(); p.kind != paneYAML || len(app.browser.stack) != 4 {
		t.Errorf("y from describe over instances: top=%+v stack=%d", p, len(app.browser.stack))
	}
	// config kind with one document: y lands on that document's YAML
	c := cfgApp(120, 40, 3)
	c = press(t, c, "d", "y") // DHCPv4Config, single doc
	if p, _ := c.browser.top(); p.kind != paneYAML || p.yaml != "kind: DHCPv4Config\n" {
		t.Errorf("y from config describe: %+v", p)
	}
}

func TestDescribeInInstancesPaneUsesPaneSubject(t *testing.T) {
	app := cfgApp(120, 40, 3)
	app = press(t, app, "j", "enter", "d") // LinkConfig instances → describe
	p, _ := app.browser.top()
	if p.kind != paneDescribe || p.title != "Describe LinkConfig" {
		t.Errorf("top = %+v", p)
	}
}

func TestDescribeScrollsAndHintsListIt(t *testing.T) {
	app := browserApp(80, 24, 2, 5)
	app = press(t, app, "d")
	out := app.renderBrowser(app.mainHeight())
	if !strings.Contains(out, "Describe Thing00") {
		t.Errorf("title missing:\n%s", out)
	}
	var keys []string
	for _, h := range stateHints(app) {
		keys = append(keys, h.key)
	}
	joined := strings.Join(keys, " ")
	if !strings.Contains(joined, "y") || !strings.Contains(joined, "d") {
		t.Errorf("hints = %v", keys)
	}
	app = press(t, app, "j", "G", "g") // must not panic or go negative
	if p, _ := app.browser.top(); p.scroll < 0 {
		t.Errorf("scroll = %d", p.scroll)
	}
}
