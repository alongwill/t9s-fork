package main

import "testing"

func TestResolveWrite(t *testing.T) {
	set := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	tests := []struct {
		name     string
		readOnly bool
		write    bool
		set      map[string]bool
		env      string
		want     bool
	}{
		{"default", true, false, set(), "", false},
		{"--write", true, true, set("write"), "", true},
		{"--readonly=false", false, false, set("readonly"), "", true},
		{"--readonly", true, false, set("readonly"), "", false},
		{"env false", true, false, set(), "false", true},
		{"env 0", true, false, set(), "0", true},
		{"env true", true, false, set(), "true", false},
		{"explicit --readonly beats env", true, false, set("readonly"), "false", false},
		{"--write and --readonly=true stays read-only", true, true, set("write", "readonly"), "", false},
		{"--write=false beats env", true, false, set("write"), "false", false},
	}
	for _, tt := range tests {
		if got := resolveWrite(tt.readOnly, tt.write, tt.set, tt.env); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
