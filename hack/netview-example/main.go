// Command netview-example writes the network stack diagram for a built-in
// fixture, no cluster needed:
//
//	go run ./hack/netview-example [-fixture bond-vlan-vip] [-out /tmp/t9s-network-example.html] [-open]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/florianspk/t9s/internal/netmodel"
)

func main() {
	fixture := flag.String("fixture", "bond-vlan-vip", "one of: "+strings.Join(netmodel.FixtureNames(), ", "))
	out := flag.String("out", "", "output file (default: $TMPDIR/t9s-network-example-<fixture>.html)")
	open := flag.Bool("open", false, "open the page in the browser")
	flag.Parse()

	known := false
	for _, n := range netmodel.FixtureNames() {
		known = known || n == *fixture
	}
	if !known {
		fmt.Fprintf(os.Stderr, "unknown fixture %q (have: %s)\n", *fixture, strings.Join(netmodel.FixtureNames(), ", "))
		os.Exit(2)
	}
	path := *out
	if path == "" {
		path = fmt.Sprintf("%s/t9s-network-example-%s.html", os.TempDir(), *fixture)
	}
	page, err := netmodel.RenderHTML(netmodel.Build(netmodel.Fixture(*fixture)))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(path, page, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(path)
	if *open {
		opener := "xdg-open"
		if runtime.GOOS == "darwin" {
			opener = "open"
		}
		if err := exec.Command(opener, path).Start(); err != nil {
			fmt.Fprintln(os.Stderr, "could not open:", err)
		}
	}
}
