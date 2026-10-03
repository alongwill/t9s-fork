package talos

import "testing"

func TestPickMemberIP(t *testing.T) {
	const v6 = "fd51:f3ce:3650:a4dc:50bf:56ff:fe80:1e65"
	cases := []struct {
		name  string
		node  string
		addrs []string
		want  string
	}{
		{"self report", "172.30.0.2", []string{"172.30.0.2", v6}, "172.30.0.2"},
		{"worker via cp prefers ipv4", "172.30.0.2", []string{"172.30.0.3", "fd51:f3ce:3650:a4dc:a004:64ff:fea2:e340"}, "172.30.0.3"},
		{"vip first", "172.30.0.9", []string{"172.30.0.100", "172.30.0.2", v6}, "172.30.0.2"},
		{"ipv6 only", "172.30.0.9", []string{v6}, v6},
		{"link-local skipped", "x", []string{"fe80::1", v6}, v6},
		{"ipv4 link-local skipped", "x", []string{"169.254.1.1", v6}, v6},
		{"empty", "172.30.0.2", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickMemberIP(tc.node, tc.addrs); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
