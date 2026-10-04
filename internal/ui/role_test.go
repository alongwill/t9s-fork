package ui

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// certYAML builds a talosconfig whose context holds a throwaway client cert.
func certYAML(t *testing.T, orgs []string, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "t9s-test", Organization: orgs},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	crt := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return fmt.Sprintf("context: c\ncontexts:\n  c:\n    endpoints: [\"10.0.0.1\"]\n    crt: %s\n", crt)
}

func TestRoleBadgeFromCertificate(t *testing.T) {
	cases := []struct {
		orgs []string
		want []string
	}{
		{[]string{"os:admin"}, []string{"os:admin"}},
		{[]string{"os:operator"}, []string{"os:operator"}},
		{[]string{"os:reader"}, []string{"os:reader"}},
		{[]string{"os:admin", "os:etcd:backup"}, []string{"os:admin", "os:etcd:backup"}},
		{nil, []string{"no roles"}},
	}
	for _, c := range cases {
		app := appFromYAML(t, certYAML(t, c.orgs, time.Now().Add(365*24*time.Hour)), "c")
		h := app.renderHeader()
		for _, w := range c.want {
			if !strings.Contains(h, w) {
				t.Errorf("orgs %v: header lacks %q", c.orgs, w)
			}
		}
		if strings.Contains(h, "cert expires") || strings.Contains(h, "via Omni") {
			t.Errorf("orgs %v: unexpected text in header", c.orgs)
		}
	}
}

func TestRoleChipWidth(t *testing.T) {
	for _, r := range []string{"os:admin", "os:operator", "os:reader", "os:etcd:backup"} {
		if got := lipgloss.Width(talosRoleChip(r)); got != len(r)+2 {
			t.Errorf("chip %s is %d cells, want %d", r, got, len(r)+2)
		}
	}
}

func TestCertExpiryWarningInHeader(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		notAfter time.Time
		want     string
		not      string
	}{
		{"far", now.Add(60 * 24 * time.Hour), "", "cert exp"},
		{"3 days", now.Add(3*24*time.Hour + time.Hour), "cert expires in 3d", ""},
		{"hours", now.Add(4 * time.Hour), "cert expires in <1d", ""},
		{"expired", now.Add(-time.Hour), "cert expired", ""},
	}
	for _, c := range cases {
		app := appFromYAML(t, certYAML(t, []string{"os:reader"}, c.notAfter), "c")
		app.id.now = func() time.Time { return now }
		h := app.renderHeader()
		if c.want != "" && !strings.Contains(h, c.want) {
			t.Errorf("%s: header lacks %q:\n%s", c.name, c.want, h)
		}
		if c.not != "" && strings.Contains(h, c.not) {
			t.Errorf("%s: header has %q", c.name, c.not)
		}
	}
}

func TestOmniContextShowsViaOmniAndHelpExplains(t *testing.T) {
	app := appFromYAML(t, omniSideroV1YAML, "omni")
	if !strings.Contains(app.renderHeader(), "role: via Omni") {
		t.Errorf("header lacks 'role: via Omni':\n%s", app.renderHeader())
	}
	help := buildHelpContentFor(app)
	for _, want := range []string{
		"Omni maps your Omni user's role to a Talos role",
		"os:reader on every call",
		"sensitive",
		"talos_backend.go",
		"sensitive_read_guard.go",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q", want)
		}
	}
}

func TestHelpShowsCertRoleAndExpiry(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	app := appFromYAML(t, certYAML(t, []string{"os:reader"}, now.Add(48*time.Hour)), "c")
	app.id.now = func() time.Time { return now }
	help := buildHelpContentFor(app)
	if !strings.Contains(help, "role: os:reader (from the client certificate)") || !strings.Contains(help, "expires in 2 days") {
		t.Errorf("help status lacks the cert role/expiry:\n%s", help[:min(len(help), 600)])
	}
	if strings.Contains(help, "Omni maps") {
		t.Error("cert context must not show the Omni role text")
	}
}

func TestRoleSummary(t *testing.T) {
	cases := []struct{ yaml, ctx, want string }{
		{certYAML(t, []string{"os:reader"}, time.Now().Add(time.Hour)), "c", "os:reader"},
		{certYAML(t, []string{"os:admin", "os:etcd:backup"}, time.Now().Add(time.Hour)), "c", "os:admin, os:etcd:backup"},
		{certYAML(t, nil, time.Now().Add(time.Hour)), "c", "no role in the certificate"},
		{omniSideroV1YAML, "omni", "via Omni"},
		{omniSideroV1YAML, "plain", "unknown"},
	}
	for _, c := range cases {
		if got := appFromYAML(t, c.yaml, c.ctx).id.roleSummary(); got != c.want {
			t.Errorf("%s: roleSummary = %q, want %q", c.ctx, got, c.want)
		}
	}
}

func TestHeaderFitsWithRoleBadges(t *testing.T) {
	app := appFromYAML(t, certYAML(t, []string{"os:admin", "os:operator", "os:etcd:backup"}, time.Now().Add(2*24*time.Hour)), "c")
	app.nodes = makeNodes(1)
	app.selNode = &app.nodes[0]
	for _, w := range []int{50, 60, 80, 100, 140, 200} {
		app.width = w
		for i, line := range strings.Split(app.renderHeader(), "\n") {
			if got := lipgloss.Width(line); got > w {
				t.Errorf("width %d: header line %d is %d cells", w, i, got)
			}
		}
	}
}
