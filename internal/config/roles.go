package config

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"time"
)

// ErrNoCert means the context has no client certificate (Omni contexts sign
// requests instead).
var ErrNoCert = errors.New("context has no client certificate")

// CertInfo is what t9s reads from the client certificate of a context.
type CertInfo struct {
	Roles    []string  // subject Organization: Talos roles such as os:admin
	NotAfter time.Time // expiry
}

// ParseCert reads the `crt` field of a talosconfig context: base64 of a PEM
// certificate (the PEM text itself is accepted too).
func ParseCert(crt string) (CertInfo, error) {
	crt = strings.TrimSpace(crt)
	if crt == "" {
		return CertInfo{}, ErrNoCert
	}
	raw := []byte(crt)
	if !strings.Contains(crt, "-----BEGIN") {
		dec, err := base64.StdEncoding.DecodeString(crt)
		if err != nil {
			return CertInfo{}, errors.New("crt is neither PEM nor base64: " + err.Error())
		}
		raw = dec
	}
	for {
		var block *pem.Block
		block, raw = pem.Decode(raw)
		if block == nil {
			return CertInfo{}, errors.New("crt holds no certificate")
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return CertInfo{}, err
		}
		return CertInfo{Roles: append([]string(nil), cert.Subject.Organization...), NotAfter: cert.NotAfter}, nil
	}
}

// CertInfo reads the context's client certificate. It returns ErrNoCert when
// the context has none.
func (c *Context) CertInfo() (CertInfo, error) {
	if c == nil {
		return CertInfo{}, ErrNoCert
	}
	return ParseCert(c.Crt)
}

// ExpiryWarning reports a certificate that has expired or expires within
// `within`. days is the whole days left (0 when expired); ok is false when no
// warning is due.
func (ci CertInfo) ExpiryWarning(now time.Time, within time.Duration) (days int, expired, ok bool) {
	if ci.NotAfter.IsZero() {
		return 0, false, false
	}
	left := ci.NotAfter.Sub(now)
	switch {
	case left <= 0:
		return 0, true, true
	case left <= within:
		d := int(left / (24 * time.Hour))
		return d, false, true
	}
	return 0, false, false
}
