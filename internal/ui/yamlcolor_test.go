package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func withColor(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// markOf returns the mark of the first occurrence of sub in text.
func markOf(t *testing.T, text, sub string) int8 {
	t.Helper()
	i := strings.Index(text, sub)
	if i < 0 {
		t.Fatalf("%q not in %q", sub, text)
	}
	marks := make([]int8, len(text))
	markYAML(text, marks)
	return marks[i]
}

func TestMarkYAMLClassifiesTokens(t *testing.T) {
	cases := []struct {
		line, sub string
		want      int8
	}{
		{"metadata:", "metadata", markTopKey},
		{"    namespace: network", "namespace", markKey},
		{"    namespace: network", "network", markString},
		{"    version: 12", "12", markNumber},
		{"    version: -1.5e3", "-1.5e3", markNumber},
		{"    running: true", "true", markKeyword},
		{"    owner: null", "null", markKeyword},
		{"    address: 10.0.0.1/24", "10.0.0.1/24", markString}, // not a number
		{"    url: http://x:80/y", "http://x:80/y", markString},
		{"    - eth0", "eth0", markString},
		{"    - eth0", "-", markPunct},
		{"    - name: eth0", "name", markKey},
		{"    - fe80::1", "fe80::1", markString}, // not a key
		{"    note: hi # why", "# why", markComment},
		{"    note: hi # why", "hi", markString},
		{"    # whole line", "# whole line", markComment},
		{"    script: |", "|", markPunct},
		{"---", "---", markPunct},
		{`    "quoted key": 7`, `"quoted key"`, markKey},
		{`    msg: "a # not comment"`, `"a # not comment"`, markString},
	}
	for _, c := range cases {
		if got := markOf(t, c.line, c.sub); got != c.want {
			t.Errorf("%q: mark of %q = %d, want %d", c.line, c.sub, got, c.want)
		}
	}
}

func TestColorYAMLLineKeepsTextAndWidth(t *testing.T) {
	withColor(t)
	line := fit("    address: 10.0.0.1/24 # lan", 40)
	out := colorYAMLLine(line, nil, hitStyle)
	if stripANSI(out) != line {
		t.Errorf("colouring changed the text: %q vs %q", stripANSI(out), line)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Error("no colour applied")
	}
	if lipgloss.Width(out) != 40 {
		t.Errorf("width = %d, want 40", lipgloss.Width(out))
	}
	// non-ASCII values stay intact
	u := colorYAMLLine("name: héllo wörld", nil, hitStyle)
	if stripANSI(u) != "name: héllo wörld" {
		t.Errorf("unicode mangled: %q", stripANSI(u))
	}
}

func TestColorYAMLSearchHitWinsOverSyntax(t *testing.T) {
	withColor(t)
	find := newMatcher("net", false).re
	out := colorYAMLLine("    namespace: network", find, hitStyle)
	if !strings.Contains(out, hitStyle.Render("net")) {
		t.Errorf("match not highlighted: %q", out)
	}
	txt := colorYAMLText("a: 1\nb: network\n", "NET")
	if !strings.Contains(txt, hitStyle.Render("net")) || stripANSI(txt) != "a: 1\nb: network\n" {
		t.Errorf("colorYAMLText: %q", txt)
	}
}

func TestYAMLPaneIsColoured(t *testing.T) {
	withColor(t)
	app := browserApp(120, 40, 4, 5)
	out := app.renderBrowser(app.mainHeight())
	if !strings.Contains(out, yamlStringStyle.Render("network")) {
		t.Errorf("string values not coloured:\n%s", out)
	}
	if !strings.Contains(out, yamlTopKeyStyle.Render("metadata")) {
		t.Errorf("top-level key not coloured")
	}
}

func TestColorTreeText(t *testing.T) {
	withColor(t)
	in := "├─◀ KubeSpanConfigs.kubespan.talos.dev"
	out := colorTreeText(in)
	if stripANSI(out) != in {
		t.Fatalf("text changed: %q", stripANSI(out))
	}
	for _, want := range []string{
		typeNameStyle.Render("KubeSpanConfigs"),
		typeSuffixStyle.Render(".kubespan.talos.dev"),
		arrowStyle.Render("◀"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	c := colorTreeText("network.LinkStatusController")
	if !strings.Contains(c, pkgStyle.Render("network.")) {
		t.Errorf("controller package not dimmed: %q", c)
	}
	// prose is untouched
	if p := colorTreeText("The live state of a link, e.g. up."); p != "The live state of a link, e.g. up." {
		t.Errorf("prose coloured: %q", p)
	}
}
