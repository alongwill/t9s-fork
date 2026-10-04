package talos

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testID = strings.Repeat("ab", 32)

func TestSchematicURLAndValidation(t *testing.T) {
	got, err := SchematicURL("https://factory.example.com/", testID)
	if err != nil || got != "https://factory.example.com/schematics/"+testID {
		t.Errorf("got %q, %v", got, err)
	}
	if got, _ := SchematicURL("", testID); got != DefaultFactoryURL+"/schematics/"+testID {
		t.Errorf("default factory: %q", got)
	}
	for _, bad := range []string{"", "abc", strings.Repeat("AB", 32), testID + "0", "../" + testID[3:], strings.Repeat("g", 64)} {
		if _, err := SchematicURL("https://f.example", bad); err == nil {
			t.Errorf("id %q accepted", bad)
		}
	}
	if _, err := SchematicURL("ftp://x", testID); err == nil {
		t.Error("ftp URL accepted")
	}
}

func TestFetchSchematicYAML(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Accept") != "application/yaml" || r.URL.Path != "/schematics/"+testID {
			t.Errorf("request %s accept=%q", r.URL.Path, r.Header.Get("Accept"))
		}
		w.Write([]byte("customization:\n  systemExtensions:\n    officialExtensions:\n      - siderolabs/iscsi-tools\n"))
	}))
	defer srv.Close()
	for i := 0; i < 2; i++ {
		y, err := FetchSchematicYAML(context.Background(), srv.URL, testID)
		if err != nil || !strings.Contains(y, "iscsi-tools") {
			t.Fatalf("got %q, %v", y, err)
		}
	}
	if hits != 1 {
		t.Errorf("server hit %d times, want 1 (cached)", hits)
	}
}

func TestFetchSchematicAuthAndErrors(t *testing.T) {
	for _, code := range []int{401, 403} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		_, err := FetchSchematicYAML(context.Background(), srv.URL, testID)
		srv.Close()
		if !errors.Is(err, ErrFactoryAuth) {
			t.Errorf("%d: err = %v", code, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	if _, err := FetchSchematicYAML(context.Background(), srv.URL, testID); err == nil || errors.Is(err, ErrFactoryAuth) {
		t.Errorf("500: err = %v", err)
	}
}

func TestFetchSchematicTimeout(t *testing.T) {
	old := SchematicTimeout
	SchematicTimeout = 50 * time.Millisecond
	defer func() { SchematicTimeout = old }()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	if _, err := FetchSchematicYAML(context.Background(), srv.URL, testID); err == nil {
		t.Error("want a timeout error")
	}
}

func TestSchematicFromExtensionsFallback(t *testing.T) {
	info, ok := SchematicFromExtensions([]Extension{{Name: "iscsi-tools", Version: "v0.1"}, {Name: "schematic", Version: testID}})
	if !ok || info.ID != testID || info.APIURL != DefaultFactoryURL || info.FromRes {
		t.Errorf("got %+v %v", info, ok)
	}
	if _, ok := SchematicFromExtensions([]Extension{{Name: "iscsi-tools"}}); ok {
		t.Error("no schematic entry must not match")
	}
}

func TestParseSchematicResource(t *testing.T) {
	y := "node: 10.0.0.1\nspec:\n    schematicId: " + testID + "\n    flavor: metal\n    apiUrl: https://f.example\n"
	info, ok := parseSchematicResource(y)
	if !ok || info.ID != testID || info.Flavor != "metal" || info.APIURL != "https://f.example" || !info.FromRes {
		t.Errorf("got %+v", info)
	}
	if info, _ := parseSchematicResource("spec:\n  schematicId: " + testID + "\n"); info.APIURL != DefaultFactoryURL {
		t.Errorf("missing apiUrl: %q", info.APIURL)
	}
	if _, ok := parseSchematicResource("spec: {}"); ok {
		t.Error("empty spec accepted")
	}
}
