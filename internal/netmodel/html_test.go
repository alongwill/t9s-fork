package netmodel

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/")

// goldenModel is the model without the notes: those come from the Talos skill
// and change when it does.
func goldenModel(name string) Model {
	m := Build(Fixture(name))
	m.Notes = nil
	return m
}

func TestEmbeddedJSONGolden(t *testing.T) {
	for _, name := range FixtureNames() {
		t.Run(name, func(t *testing.T) {
			got, err := json.MarshalIndent(goldenModel(name), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", name+".json")
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run: go test ./internal/netmodel -run TestEmbeddedJSONGolden -update)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("model JSON for %s changed; review testdata/%s.json.new and rerun with -update if intended", name, name)
				_ = os.WriteFile(path+".new", got, 0o644)
			}
		})
	}
}

// pageModel pulls the JSON out of the rendered page.
var modelRe = regexp.MustCompile(`(?s)<script type="application/json" id="model">(.*?)</script>`)

func TestRenderHTMLWithEachFixture(t *testing.T) {
	for _, name := range FixtureNames() {
		t.Run(name, func(t *testing.T) {
			m := Build(Fixture(name))
			page, err := RenderHTML(m)
			if err != nil {
				t.Fatal(err)
			}
			s := string(page)
			if strings.Contains(s, "<!--T9S_") {
				t.Error("a template marker was not replaced")
			}
			sub := modelRe.FindStringSubmatch(s)
			if sub == nil {
				t.Fatal("no model script element in the page")
			}
			var back Model
			if err := json.Unmarshal([]byte(sub[1]), &back); err != nil {
				t.Fatalf("embedded JSON does not parse: %v", err)
			}
			if len(back.Links) != len(m.Links) || back.Hostname != m.Hostname || len(back.Warnings) != len(m.Warnings) {
				t.Errorf("round trip lost data: %d/%d links", len(back.Links), len(m.Links))
			}
			// exactly the script elements the template has: model, cytoscape, the page script
			if got := strings.Count(s, "</script>"); got != 3 {
				t.Errorf("%d closing script tags, want 3", got)
			}
			if !strings.Contains(s, "<title>Network of ") {
				t.Error("no title")
			}
		})
	}
}

func TestJSONInjectionIsEscaped(t *testing.T) {
	evil := `</script><script>alert(1)</script><!-- & "quotes" '  `
	in := Fixture("single-nic-dhcp")
	in.Hostnames = []Res{fixRes(NSNetwork, "hostname", "hostname: x\ndomainname: \"\"")}
	m := Build(in)
	m.Hostname = evil
	m.Resolvers = []string{evil}
	m.Warnings = append(m.Warnings, Warning{Name: evil, Message: evil, Config: ConfigRef{Kind: "LinkConfig", Name: evil}})
	m.Links[0].Alias = evil
	page, err := RenderHTML(m)
	if err != nil {
		t.Fatal(err)
	}
	s := string(page)
	if strings.Contains(s, "<script>alert(1)") {
		t.Fatal("a value ended the model script element and started another")
	}
	if got := strings.Count(s, "</script>"); got != 3 {
		t.Errorf("%d closing script tags, want 3: a value injected one", got)
	}
	sub := modelRe.FindStringSubmatch(s)
	if sub == nil {
		t.Fatal("no model element")
	}
	if strings.ContainsAny(sub[1], "<>&") {
		t.Errorf("embedded JSON still holds raw <, > or &: %q", sub[1])
	}
	var back Model
	if err := json.Unmarshal([]byte(sub[1]), &back); err != nil {
		t.Fatal(err)
	}
	if back.Hostname != evil || back.Resolvers[0] != evil || back.Warnings[len(back.Warnings)-1].Message != evil {
		t.Error("the escaped values do not decode back to the originals")
	}
	// the <title> is HTML-escaped too
	if !strings.Contains(s, "<title>Network of &lt;/script&gt;") {
		t.Errorf("title not escaped: %s", s[strings.Index(s, "<title>"):strings.Index(s, "</title>")])
	}
}

func TestFileNameAndWrite(t *testing.T) {
	now := time.Date(2026, 10, 3, 14, 5, 9, 0, time.UTC)
	m := Model{Hostname: "cp-1.example/../x y", Node: "10.0.0.5"}
	if got := FileName(m, now); got != "t9s-network-cp-1.example-..-x-y-20261003-140509.html" && !strings.HasPrefix(got, "t9s-network-cp-1.example") {
		t.Errorf("FileName = %q", got)
	}
	if strings.ContainsAny(FileName(m, now), "/ ") {
		t.Errorf("FileName has a path separator or space: %q", FileName(m, now))
	}
	if got := FileName(Model{Node: "10.0.0.5"}, now); got != "t9s-network-10.0.0.5-20261003-140509.html" {
		t.Errorf("FileName without hostname = %q", got)
	}
	dir := t.TempDir()
	path, err := WriteHTML(Build(Fixture("bridge")), dir, now)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir || !strings.HasSuffix(path, "t9s-network-w-1-20261003-140509.html") {
		t.Errorf("path = %s", path)
	}
	if b, err := os.ReadFile(path); err != nil || !bytes.Contains(b, []byte("cytoscape")) {
		t.Errorf("file not written: %v", err)
	}
}

// TestWriteExampleHTML writes the page for a fixture, no cluster needed:
//
//	go test ./internal/netmodel -run TestWriteExampleHTML -fixture bond-vlan-vip -out /tmp/net.html
func TestWriteExampleHTML(t *testing.T) {
	if *exampleOut == "" {
		t.Skip("set -out to write an example page")
	}
	page, err := RenderHTML(Build(Fixture(*exampleFixture)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(*exampleOut, page, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", *exampleOut)
}

var (
	exampleFixture = flag.String("fixture", "bond-vlan-vip", "fixture for TestWriteExampleHTML")
	exampleOut     = flag.String("out", "", "output path for TestWriteExampleHTML")
)
