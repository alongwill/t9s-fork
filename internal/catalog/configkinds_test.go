package catalog

import "testing"

func TestConfigKindsLoaded(t *testing.T) {
	if n := len(ConfigKinds()); n != 96 {
		t.Fatalf("want 96 kinds, got %d", n)
	}
}

func TestConfigCategoryForAllGroups(t *testing.T) {
	known := map[string]bool{}
	for _, c := range Categories {
		known[c.Key] = true
	}
	for _, k := range ConfigKinds() {
		cat := ConfigCategoryFor(k.Group)
		if cat == "other" || !known[cat] {
			t.Errorf("kind %s group %q -> %q", k.Kind, k.Group, cat)
		}
	}
	if ConfigCategoryFor("network") != "networking" || ConfigCategoryFor("container") != "containers" {
		t.Error("network/container mapping wrong")
	}
	if ConfigCategoryFor("nope") != "other" {
		t.Error("unknown group should be other")
	}
}

func has(ks []ConfigKind, name string) bool {
	for _, k := range ks {
		if k.Kind == name {
			return true
		}
	}
	return false
}

func TestKindsAvailable(t *testing.T) {
	all := len(ConfigKinds())
	if got := len(KindsAvailable("")); got != all {
		t.Errorf("empty keeps all: %d != %d", got, all)
	}
	if got := len(KindsAvailable("garbage")); got != all {
		t.Errorf("unparsable keeps all: %d != %d", got, all)
	}
	v113 := KindsAvailable("v1.13.2")
	if has(v113, "BGPInstanceConfig") {
		t.Error("v1.13 should hide v1.14 kind BGPInstanceConfig")
	}
	if !has(v113, "BlackholeRouteConfig") {
		t.Error("v1.13 should keep v1.13 kind BlackholeRouteConfig")
	}
	v114 := KindsAvailable("v1.14")
	if !has(v114, "BGPInstanceConfig") || has(v114, "CPUScalingConfig") {
		t.Error("v1.14 should keep v1.14 and hide v1.15 kinds")
	}
	if !has(KindsAvailable("v1.15.0-alpha.0"), "CPUScalingConfig") {
		t.Error("v1.15.0-alpha.0 should parse as v1.15")
	}
}
