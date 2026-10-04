package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"reflect"
	"testing"
	"time"
)

// makeCert builds a throwaway self-signed certificate; no key is checked in.
func makeCert(t *testing.T, orgs []string, notAfter time.Time) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test", Organization: orgs},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestParseCertRoles(t *testing.T) {
	exp := time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	p := makeCert(t, []string{"os:admin", "os:etcd:backup"}, exp)

	for name, in := range map[string]string{
		"base64 (talosconfig form)": base64.StdEncoding.EncodeToString(p),
		"raw PEM":                   string(p),
	} {
		ci, err := ParseCert(in)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(ci.Roles, []string{"os:admin", "os:etcd:backup"}) {
			t.Errorf("%s: roles = %v", name, ci.Roles)
		}
		if !ci.NotAfter.Equal(exp) {
			t.Errorf("%s: NotAfter = %v, want %v", name, ci.NotAfter, exp)
		}
	}
}

func TestParseCertSingleRoleAndNone(t *testing.T) {
	ci, err := ParseCert(base64.StdEncoding.EncodeToString(makeCert(t, []string{"os:reader"}, time.Now().Add(time.Hour))))
	if err != nil || !reflect.DeepEqual(ci.Roles, []string{"os:reader"}) {
		t.Errorf("got %v, %v", ci.Roles, err)
	}
	ci, err = ParseCert(base64.StdEncoding.EncodeToString(makeCert(t, nil, time.Now().Add(time.Hour))))
	if err != nil || len(ci.Roles) != 0 {
		t.Errorf("no org: got %v, %v", ci.Roles, err)
	}
}

func TestParseCertErrors(t *testing.T) {
	if _, err := ParseCert(""); !errors.Is(err, ErrNoCert) {
		t.Errorf("empty: %v", err)
	}
	if _, err := ParseCert("!!!not base64!!!"); err == nil {
		t.Error("garbage must fail")
	}
	if _, err := ParseCert(base64.StdEncoding.EncodeToString([]byte("hello"))); err == nil {
		t.Error("base64 of non-PEM must fail")
	}
	var nilCtx *Context
	if _, err := nilCtx.CertInfo(); !errors.Is(err, ErrNoCert) {
		t.Errorf("nil context: %v", err)
	}
}

func TestExpiryWarning(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	const week = 7 * 24 * time.Hour
	cases := []struct {
		name        string
		notAfter    time.Time
		days        int
		expired, ok bool
	}{
		{"far", now.Add(30 * 24 * time.Hour), 0, false, false},
		{"exactly a week", now.Add(week), 7, false, true},
		{"3 days", now.Add(3*24*time.Hour + time.Hour), 3, false, true},
		{"hours left", now.Add(5 * time.Hour), 0, false, true},
		{"expired", now.Add(-time.Minute), 0, true, true},
		{"zero value", time.Time{}, 0, false, false},
	}
	for _, c := range cases {
		d, ex, ok := CertInfo{NotAfter: c.notAfter}.ExpiryWarning(now, week)
		if d != c.days || ex != c.expired || ok != c.ok {
			t.Errorf("%s: got (%d,%v,%v), want (%d,%v,%v)", c.name, d, ex, ok, c.days, c.expired, c.ok)
		}
	}
}
