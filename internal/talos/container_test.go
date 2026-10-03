package talos

import "testing"

func TestParseCRIID(t *testing.T) {
	cases := []struct {
		id   string
		want CRIID
	}{
		{"kube-system/coredns", CRIID{Namespace: "kube-system", Pod: "coredns", CRI: true}},
		{"kube-system/coredns:coredns:abc123def456", CRIID{Namespace: "kube-system", Pod: "coredns", Name: "coredns", ShortID: "abc123def456", CRI: true}},
		{"default/web-7d9f8-x2k:nginx", CRIID{Namespace: "default", Pod: "web-7d9f8-x2k", Name: "nginx", CRI: true}},
		{"apid", CRIID{Name: "apid"}},
		{"", CRIID{}},
	}
	for _, tc := range cases {
		if got := ParseCRIID(tc.id); got != tc.want {
			t.Errorf("ParseCRIID(%q) = %+v, want %+v", tc.id, got, tc.want)
		}
	}
}

func TestParseImageRef(t *testing.T) {
	cases := []struct {
		ref  string
		want ImageRef
	}{
		{"registry.k8s.io/coredns/coredns:v1.11.3", ImageRef{"registry.k8s.io", "coredns/coredns", "v1.11.3", ""}},
		{"registry.k8s.io/pause:3.8", ImageRef{"registry.k8s.io", "pause", "3.8", ""}},
		{"ghcr.io/siderolabs/kubelet:v1.34.1@sha256:abcd", ImageRef{"ghcr.io", "siderolabs/kubelet", "v1.34.1", "sha256:abcd"}},
		{"ghcr.io/siderolabs/apid@sha256:abcd", ImageRef{"ghcr.io", "siderolabs/apid", "", "sha256:abcd"}},
		{"localhost:5000/foo/bar:dev", ImageRef{"localhost:5000", "foo/bar", "dev", ""}},
		{"nginx:1.27", ImageRef{"", "nginx", "1.27", ""}},
		{"library/nginx", ImageRef{"", "library/nginx", "", ""}},
		{"", ImageRef{}},
	}
	for _, tc := range cases {
		if got := ParseImageRef(tc.ref); got != tc.want {
			t.Errorf("ParseImageRef(%q) = %+v, want %+v", tc.ref, got, tc.want)
		}
	}
}

func TestParseStatsLinesTreeMarker(t *testing.T) {
	data := []byte("NODE       NAMESPACE   ID                                     MEMORY(MB)   CPU\n" +
		"10.5.0.2   k8s.io      kube-system/coredns                    0.00         0\n" +
		"10.5.0.2   k8s.io      └─ kube-system/coredns:coredns:abc123   18.52        4210000000\n" +
		"10.5.0.2   system      apid                                   25.10        900000\n")
	got := parseStatsLines(data)
	if len(got) != 3 {
		t.Fatalf("got %d rows: %+v", len(got), got)
	}
	if got[1].ID != "kube-system/coredns:coredns:abc123" || got[1].MemoryMB != 18.52 || got[1].CPUNanos != 4210000000 {
		t.Errorf("tree row = %+v", got[1])
	}
	if got[2].ID != "apid" || got[2].MemoryMB != 25.10 {
		t.Errorf("plain row = %+v", got[2])
	}
}

func TestParseMemoryTotalMB(t *testing.T) {
	data := []byte("NODE         TOTAL   USED   FREE   SHARED   BUFFERS   CACHE   AVAILABLE\n172.30.0.2   3902    812    2100   12       3         977     2900\n")
	if v, ok := parseMemoryTotalMB(data); !ok || v != 3902 {
		t.Errorf("got %v %v", v, ok)
	}
	if _, ok := parseMemoryTotalMB([]byte("NODE TOTAL\n")); ok {
		t.Error("header-only output parsed")
	}
}
