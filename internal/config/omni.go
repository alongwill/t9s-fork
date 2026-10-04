package config

import (
	"net/url"
	"strings"
)

// How Omni was recognised. The help overlay shows these texts.
const (
	ViaSideroV1   = "talosconfig siderov1"
	ViaEndpoint   = "talosconfig endpoint"
	ViaSideroLink = "SideroLink status on the node"
)

// Omni says whether the selected context talks to an Omni instance.
type Omni struct {
	Detected bool
	Host     string // Omni host, empty when not known
	Via      string // one of the Via constants
}

// DetectOmni looks at the talosconfig only (no network call):
//
//  1. the context authenticates with an `auth.siderov1` block, or
//  2. an endpoint is an HTTPS URL whose host contains "omni".
//
// The third path (SideroLink status read from the cluster) runs in the UI after
// the node list loads.
func (c *Context) DetectOmni() Omni {
	if c == nil {
		return Omni{}
	}
	host, httpsOmni := omniEndpoint(c.Endpoints)
	if c.Auth.SideroV1 != nil {
		if host == "" {
			host = endpointHost(c.Endpoints)
		}
		return Omni{Detected: true, Host: host, Via: ViaSideroV1}
	}
	if httpsOmni {
		return Omni{Detected: true, Host: host, Via: ViaEndpoint}
	}
	return Omni{}
}

// omniEndpoint returns the host of the first HTTPS endpoint containing "omni".
func omniEndpoint(endpoints []string) (string, bool) {
	for _, e := range endpoints {
		u, err := url.Parse(strings.TrimSpace(e))
		if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
			continue
		}
		if strings.Contains(strings.ToLower(u.Hostname()), "omni") {
			return hostOf(u), true
		}
	}
	return "", false
}

// endpointHost is the host of the first endpoint, URL or bare host[:port].
func endpointHost(endpoints []string) string {
	for _, e := range endpoints {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if u, err := url.Parse(e); err == nil && u.Host != "" {
			return hostOf(u)
		}
		return e
	}
	return ""
}

// hostOf drops the default HTTPS port.
func hostOf(u *url.URL) string {
	if u.Port() == "443" {
		return u.Hostname()
	}
	return u.Host
}
