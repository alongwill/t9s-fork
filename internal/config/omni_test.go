package config

import "testing"

func parseFixture(t *testing.T, y string) *TalosConfig {
	t.Helper()
	cfg, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestDetectOmniSideroV1(t *testing.T) {
	cfg := parseFixture(t, `
context: omni-prod
contexts:
  omni-prod:
    endpoints:
      - https://acme.omni.siderolabs.io
    auth:
      siderov1:
        identity: andy@example.com
`)
	o := cfg.Named("omni-prod").DetectOmni()
	if !o.Detected || o.Via != ViaSideroV1 || o.Host != "acme.omni.siderolabs.io" {
		t.Errorf("got %+v", o)
	}
}

func TestDetectOmniSideroV1WithBareEndpoint(t *testing.T) {
	cfg := parseFixture(t, `
context: x
contexts:
  x:
    endpoints: ["10.1.2.3:8100"]
    auth:
      siderov1:
        identity: a@b.c
`)
	o := cfg.Named("").DetectOmni()
	if !o.Detected || o.Via != ViaSideroV1 || o.Host != "10.1.2.3:8100" {
		t.Errorf("got %+v", o)
	}
}

func TestDetectOmniEndpointOnly(t *testing.T) {
	cfg := parseFixture(t, `
context: x
contexts:
  x:
    endpoints:
      - https://omni.corp.example:8443
    ca: abc
`)
	o := cfg.Named("x").DetectOmni()
	if !o.Detected || o.Via != ViaEndpoint || o.Host != "omni.corp.example:8443" {
		t.Errorf("got %+v", o)
	}
}

func TestDetectOmniDefaultPortDropped(t *testing.T) {
	cfg := parseFixture(t, `
contexts:
  x:
    endpoints: ["https://Omni.Example.com:443"]
`)
	if o := cfg.Named("x").DetectOmni(); !o.Detected || o.Host != "Omni.Example.com" {
		t.Errorf("got %+v", o)
	}
}

func TestDetectOmniNotOmni(t *testing.T) {
	cases := map[string]string{
		"plain talos":           `{contexts: {x: {endpoints: ["10.0.0.1"], crt: abc}}}`,
		"http omni is not omni": `{contexts: {x: {endpoints: ["http://omni.example.com"]}}}`,
		"https without omni":    `{contexts: {x: {endpoints: ["https://api.example.com:6443"]}}}`,
		"omni in the path only": `{contexts: {x: {endpoints: ["https://example.com/omni"]}}}`,
		"basic auth":            `{contexts: {x: {endpoints: ["10.0.0.1"], auth: {basic: {username: u, password: p}}}}}`,
	}
	for name, y := range cases {
		cfg := parseFixture(t, y)
		if o := cfg.Named("x").DetectOmni(); o.Detected {
			t.Errorf("%s: detected as Omni: %+v", name, o)
		}
	}
	var nilCtx *Context
	if nilCtx.DetectOmni().Detected {
		t.Error("nil context detected as Omni")
	}
}

func TestNamedFallsBackToCurrent(t *testing.T) {
	cfg := parseFixture(t, `{context: a, contexts: {a: {endpoints: [x]}, b: {endpoints: [y]}}}`)
	if c := cfg.Named(""); c == nil || c.Endpoints[0] != "x" {
		t.Errorf("Named(\"\") = %+v", c)
	}
	if cfg.Named("zzz") != nil {
		t.Error("unknown context must be nil")
	}
}
