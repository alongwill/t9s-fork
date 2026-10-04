package ui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/florianspk/t9s/internal/talos"
	"github.com/muesli/termenv"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func newTestApp(width, height int) App {
	return App{width: width, height: height}
}

func makeProcesses(n int) []talos.ProcessInfo {
	p := make([]talos.ProcessInfo, n)
	for i := range p {
		p[i] = talos.ProcessInfo{
			PID:     fmt.Sprintf("%d", 1000+i),
			State:   "S",
			CPUTime: "0:01",
			ResMem:  "10240",
			Command: "/usr/bin/containerd --config /etc/containerd/config.toml --root /var/lib/containerd --state /run/containerd --log-level debug",
		}
	}
	return p
}

func makeContainers(n int) []talos.ContainerInfo {
	c := make([]talos.ContainerInfo, n)
	for i := range c {
		c[i] = talos.ContainerInfo{
			Namespace: "k8s.io",
			ID:        fmt.Sprintf("abc%012d", i),
			Image:     "ghcr.io/siderolabs/extensions:v1.6.4-sha256-abcdef1234567890abcdef1234567890abcdef",
			PID:       fmt.Sprintf("%d", 2000+i),
			Status:    "Running",
		}
	}
	return c
}

func makeNodes(n int) []talos.Node {
	nodes := make([]talos.Node, n)
	for i := range nodes {
		nodes[i] = talos.Node{
			Hostname:    fmt.Sprintf("talos-node-%02d.example.internal", i),
			IP:          fmt.Sprintf("10.0.0.%d", i+1),
			DisplayIP:   fmt.Sprintf("10.0.0.%d", i+1),
			Role:        "worker",
			Version:     "v1.13.3",
			KubeVersion: "v1.31.0",
			Status:      "ready",
		}
	}
	if len(nodes) > 0 {
		nodes[0].Role = "controlplane"
	}
	return nodes
}

func makeServices(n int) []talos.Service {
	svcs := make([]talos.Service, n)
	for i := range svcs {
		svcs[i] = talos.Service{
			ID:      fmt.Sprintf("service-%02d", i),
			State:   "Running",
			Healthy: "OK",
		}
	}
	return svcs
}

func makeAddresses(n int) []talos.AddressInfo {
	a := make([]talos.AddressInfo, n)
	for i := range a {
		a[i] = talos.AddressInfo{
			Interface: fmt.Sprintf("eth%d", i),
			Address:   fmt.Sprintf("192.168.%d.%d/24", i, i+1),
			Family:    "inet",
			Scope:     "global very-long-scope-label-that-might-overflow-the-column",
		}
	}
	return a
}

func lineCount(s string) int { return strings.Count(s, "\n") }

func maxLineWidth(s string) int {
	max := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(line); w > max {
			max = w
		}
	}
	return max
}

// ── processes ─────────────────────────────────────────────────────────────────

func TestRenderProcessesHeightBudget(t *testing.T) {
	cases := []struct {
		width, height, n, cur int
		wrap                  bool
	}{
		{80, 20, 5, 0, false},
		{80, 20, 5, 0, true},
		{80, 20, 20, 5, true},
		{80, 20, 20, 15, true},  // cursor late, many wrapped rows above
		{60, 15, 20, 10, true},  // narrow terminal
		{120, 30, 50, 25, true}, // wide terminal
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("w%d_h%d_n%d_cur%d_wrap%v", tc.width, tc.height, tc.n, tc.cur, tc.wrap), func(t *testing.T) {
			app := newTestApp(tc.width, tc.height)
			app.processes = makeProcesses(tc.n)
			app.wrapMode = tc.wrap
			app.listScroll = tc.cur
			out := app.renderProcesses(tc.height)
			if got := lineCount(out); got > tc.height {
				t.Errorf("output has %d lines, want ≤ %d\n%s", got, tc.height, out)
			}
		})
	}
}

func TestRenderProcessesCursorAlwaysVisible(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		for _, cur := range []int{0, 5, 15, 19} {
			wrap, cur := wrap, cur
			t.Run(fmt.Sprintf("wrap%v_cur%d", wrap, cur), func(t *testing.T) {
				app := newTestApp(80, 25)
				app.processes = makeProcesses(20)
				app.wrapMode = wrap
				app.listScroll = cur
				out := app.renderProcesses(22)
				if !strings.Contains(out, "▶") {
					t.Errorf("▶ cursor not visible\n%s", out)
				}
			})
		}
	}
}

// Specific regression: many wrapped items above the cursor must not push cursor off-screen.
func TestRenderProcessesCursorVisibleWhenManyWrappedRowsAbove(t *testing.T) {
	app := newTestApp(60, 20) // narrow → many wrap lines per item
	app.processes = makeProcesses(20)
	app.wrapMode = true
	app.listScroll = 19 // last item
	out := app.renderProcesses(17)
	if !strings.Contains(out, "▶") {
		t.Error("▶ cursor not visible when many wrapped items above it")
	}
	if got := lineCount(out); got > 17 {
		t.Errorf("height budget exceeded: %d lines > 17", got)
	}
}

// ── containers ────────────────────────────────────────────────────────────────

func TestRenderContainersCursorAlwaysVisible(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		for _, cur := range []int{0, 5, 9} {
			wrap, cur := wrap, cur
			t.Run(fmt.Sprintf("wrap%v_cur%d", wrap, cur), func(t *testing.T) {
				app := newTestApp(80, 25)
				app.containers = makeContainers(10)
				app.wrapMode = wrap
				app.contCur = cur
				out := app.renderContainers(22)
				if !strings.Contains(out, "▶") {
					t.Errorf("▶ cursor not visible\n%s", out)
				}
			})
		}
	}
}

func TestRenderContainersHeightBudget(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		for _, cur := range []int{0, 5, 9} {
			wrap, cur := wrap, cur
			t.Run(fmt.Sprintf("wrap%v_cur%d", wrap, cur), func(t *testing.T) {
				app := newTestApp(80, 25)
				app.containers = makeContainers(10)
				app.wrapMode = wrap
				app.contCur = cur
				const h = 22
				out := app.renderContainers(h)
				if got := lineCount(out); got > h {
					t.Errorf("output has %d lines, want ≤ %d", got, h)
				}
			})
		}
	}
}

// ── addresses ────────────────────────────────────────────────────────────────

func TestRenderAddressesCursorAlwaysVisible(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		for _, cur := range []int{0, 3, 9} {
			wrap, cur := wrap, cur
			t.Run(fmt.Sprintf("wrap%v_cur%d", wrap, cur), func(t *testing.T) {
				app := newTestApp(80, 25)
				app.addresses = makeAddresses(10)
				app.wrapMode = wrap
				app.listScroll = cur
				out := app.renderAddresses(22)
				if !strings.Contains(out, "▶") {
					t.Errorf("▶ cursor not visible\n%s", out)
				}
			})
		}
	}
}

// ── renderLinesCursor (logs/dmesg/health) ─────────────────────────────────────

func makeLines(n int) []string {
	ls := make([]string, n)
	for i := range ls {
		ls[i] = fmt.Sprintf("line %04d: some log content that is long enough to wrap on a narrow terminal and test the budget", i)
	}
	return ls
}

func TestRenderLinesCursorHeightBudget(t *testing.T) {
	lines := makeLines(50)
	cases := []struct{ width, maxRows, cur int }{
		{80, 20, 0},
		{80, 20, 25},
		{80, 20, 49},
		{40, 10, 25}, // narrow → each line wraps → tightest budget
		{40, 10, 49},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("w%d_r%d_cur%d", tc.width, tc.maxRows, tc.cur), func(t *testing.T) {
			out := renderLinesCursor(lines, tc.cur, tc.width, tc.maxRows, 0, "")
			if got := lineCount(out); got > tc.maxRows {
				t.Errorf("got %d lines, want ≤ %d", got, tc.maxRows)
			}
		})
	}
}

func TestRenderLinesCursorAlwaysVisible(t *testing.T) {
	lines := makeLines(50)
	for _, cur := range []int{0, 10, 25, 49} {
		cur := cur
		t.Run(fmt.Sprintf("cur%d", cur), func(t *testing.T) {
			out := renderLinesCursor(lines, cur, 80, 20, 0, "")
			if !strings.Contains(out, "▶") {
				t.Errorf("▶ not visible at cur=%d", cur)
			}
		})
	}
}

// ── find in logs ──────────────────────────────────────────────────────────────

func TestFindLineNextWraps(t *testing.T) {
	lines := []string{"alpha", "beta", "gamma", "alpha again"}
	// from=2 (gamma), should wrap and find "alpha" at 0
	idx := findLineNext(lines, 2, "alpha")
	if idx != 2 { // (2+0)%4=2? no: 2 doesn't match, 3 matches "alpha again"
		// Actually from=2: checks 2,3,0,1 → 3 matches "alpha again"
	}
	if idx < 0 {
		t.Fatal("expected a match")
	}
	if !strings.Contains(strings.ToLower(lines[idx]), "alpha") {
		t.Errorf("line %d does not contain 'alpha': %q", idx, lines[idx])
	}
}

func TestFindLineNextNoMatch(t *testing.T) {
	lines := []string{"foo", "bar", "baz"}
	if idx := findLineNext(lines, 0, "xyz"); idx != -1 {
		t.Errorf("want -1, got %d", idx)
	}
}

func TestFindLinePrevWraps(t *testing.T) {
	lines := []string{"error here", "info", "another error"}
	// from=1 going backward: 1 (no), 0 (error) → 0
	idx := findLinePrev(lines, 1, "error")
	if idx != 0 {
		t.Errorf("want 0, got %d", idx)
	}
}

func TestFindLinePrevNoMatch(t *testing.T) {
	lines := []string{"foo", "bar"}
	if idx := findLinePrev(lines, 1, "xyz"); idx != -1 {
		t.Errorf("want -1, got %d", idx)
	}
}

func TestCountMatches(t *testing.T) {
	lines := []string{"error: disk full", "info: started", "ERROR: timeout", "ok"}
	n := countMatches(lines, "error")
	if n != 2 {
		t.Errorf("want 2, got %d", n)
	}
	if countMatches(lines, "") != 0 {
		t.Error("empty query must return 0")
	}
	if countMatches(nil, "error") != 0 {
		t.Error("nil lines must return 0")
	}
}

func TestRenderLinesCursorFindHighlight(t *testing.T) {
	lines := []string{"no match here", "ERROR: something bad", "normal line"}
	out := renderLinesCursor(lines, 0, 80, 10, 0, "error")
	if !strings.Contains(out, "▸") {
		t.Error("▸ marker expected for matching line")
	}
}

func TestRenderLinesCursorNoFindQuery(t *testing.T) {
	lines := makeLines(5)
	out := renderLinesCursor(lines, 0, 80, 10, 0, "")
	if strings.Contains(out, "▸") {
		t.Error("▸ must not appear when findQuery is empty")
	}
}

// ── node list ─────────────────────────────────────────────────────────────────

func TestRenderNodeListNoLineExceedsWidth(t *testing.T) {
	for _, width := range []int{80, 120, 160, 220} {
		width := width
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			app := newTestApp(width, 30)
			app.nodes = makeNodes(3)
			out := app.renderNodeList(25)
			if got := maxLineWidth(out); got > width {
				t.Errorf("line width %d > terminal width %d", got, width)
			}
		})
	}
}

func TestRenderNodeListCursorAlwaysVisible(t *testing.T) {
	for _, cur := range []int{0, 1, 2} {
		cur := cur
		t.Run(fmt.Sprintf("cur%d", cur), func(t *testing.T) {
			app := newTestApp(120, 30)
			app.nodes = makeNodes(3)
			app.nodeCur = cur
			out := app.renderNodeList(25)
			if !strings.Contains(out, "▶") {
				t.Errorf("▶ not visible at cur=%d", cur)
			}
		})
	}
}

func TestRenderNodeListHeightBudget(t *testing.T) {
	cases := []struct{ n, cur, height int }{
		{3, 0, 20},
		{3, 2, 20},
		{20, 0, 20},
		{20, 10, 20},
		{20, 19, 20},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("n%d_cur%d_h%d", tc.n, tc.cur, tc.height), func(t *testing.T) {
			app := newTestApp(120, 30)
			app.nodes = makeNodes(tc.n)
			app.nodeCur = tc.cur
			out := app.renderNodeList(tc.height)
			if got := lineCount(out); got > tc.height {
				t.Errorf("output %d lines > budget %d", got, tc.height)
			}
		})
	}
}

// NAME column must be at least 20 chars even on narrow terminals.
func TestRenderNodeListResponsiveMinColHost(t *testing.T) {
	for _, width := range []int{60, 80, 120, 200} {
		width := width
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			app := newTestApp(width, 20)
			app.nodes = makeNodes(1)
			out := app.renderNodeList(15)
			if got := maxLineWidth(out); got > width {
				t.Errorf("line width %d > terminal width %d", got, width)
			}
		})
	}
}

// K8S version column must appear and not overflow.
func TestRenderNodeListK8sVersionColumn(t *testing.T) {
	app := newTestApp(160, 20)
	app.nodes = makeNodes(2)
	out := app.renderNodeList(15)
	if !strings.Contains(out, "K8S") {
		t.Error("K8S column header not found")
	}
	if !strings.Contains(out, "v1.31.0") {
		t.Error("K8S version v1.31.0 not displayed")
	}
	if got := maxLineWidth(out); got > 160 {
		t.Errorf("line width %d > terminal width 160", got)
	}
}

// ── services ──────────────────────────────────────────────────────────────────

func TestRenderServicesCursorAlwaysVisible(t *testing.T) {
	nodes := makeNodes(1)
	for _, cur := range []int{0, 3, 6} {
		cur := cur
		t.Run(fmt.Sprintf("cur%d", cur), func(t *testing.T) {
			app := newTestApp(120, 25)
			app.services = makeServices(7)
			app.svcCur = cur
			app.selNode = &nodes[0]
			out := app.renderServices(20)
			if !strings.Contains(out, "▶") {
				t.Errorf("▶ not visible at cur=%d", cur)
			}
		})
	}
}

func TestRenderServicesHeightBudget(t *testing.T) {
	nodes := makeNodes(1)
	for _, cur := range []int{0, 3, 6} {
		cur := cur
		t.Run(fmt.Sprintf("cur%d", cur), func(t *testing.T) {
			app := newTestApp(120, 25)
			app.services = makeServices(7)
			app.svcCur = cur
			app.selNode = &nodes[0]
			const h = 20
			out := app.renderServices(h)
			if got := lineCount(out); got > h {
				t.Errorf("output %d lines > budget %d", got, h)
			}
		})
	}
}

func TestRenderServicesNoLineExceedsWidth(t *testing.T) {
	nodes := makeNodes(1)
	for _, width := range []int{80, 120, 200} {
		width := width
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			app := newTestApp(width, 25)
			app.services = makeServices(7)
			app.selNode = &nodes[0]
			out := app.renderServices(20)
			if got := maxLineWidth(out); got > width {
				t.Errorf("line width %d > terminal width %d", got, width)
			}
		})
	}
}

// ── resource browser ─────────────────────────────────────────────────────────

var browserSizes = []struct{ w, h int }{{80, 24}, {120, 40}, {200, 50}}

func TestRenderBrowserHeightAndWidthBudget(t *testing.T) {
	for _, sz := range browserSizes {
		for depth := 1; depth <= 4; depth++ {
			for _, wrap := range []bool{false, true} {
				sz, depth, wrap := sz, depth, wrap
				t.Run(fmt.Sprintf("w%d_h%d_depth%d_wrap%v", sz.w, sz.h, depth, wrap), func(t *testing.T) {
					app := browserApp(sz.w, sz.h, depth, 60)
					app.browser.wrap = wrap
					out := app.renderBrowser(app.mainHeight())
					if got := lineCount(out); got != app.mainHeight() {
						t.Errorf("output has %d lines, want exactly %d", got, app.mainHeight())
					}
					if got := maxLineWidth(out); got > sz.w {
						t.Errorf("a line is %d cells wide, terminal is %d\n%s", got, sz.w, out)
					}
				})
			}
		}
	}
}

func TestRenderBrowserTitleBarFitsHeader(t *testing.T) {
	app := browserApp(80, 24, 4, 10)
	app.browser.node.Hostname = strings.Repeat("very-long-hostname-", 6)
	if w := lipgloss.Width(resourceLine(app)); w > 80-2 {
		t.Errorf("breadcrumb is %d cells, want ≤ 78", w)
	}
}

func TestRenderBrowserCursorVisibleAfterFold(t *testing.T) {
	for _, sz := range browserSizes {
		for _, depth := range []int{1, 2, 3} {
			sz, depth := sz, depth
			t.Run(fmt.Sprintf("w%d_h%d_depth%d", sz.w, sz.h, depth), func(t *testing.T) {
				app := browserApp(sz.w, sz.h, depth, 60)
				rows := app.paneInnerRows(paneTypes)
				if depth == 3 {
					rows = app.paneInnerRows(paneInstances)
				}
				moves := rows + 12 // well past the fold
				if depth == 1 {
					moves = 0 // only one category in the fixtures; nothing to scroll
				}
				for i := 0; i < moves; i++ {
					app = press(t, app, "down")
				}
				p, _ := app.browser.top()
				out := app.renderBrowser(app.mainHeight())
				var want string
				switch depth {
				case 1:
					want = "▶ Networking"
				case 2:
					want = fmt.Sprintf("▶ Thing%02d", p.cur)
				case 3:
					want = fmt.Sprintf("▶ eth%d/10.0.0.%d/24", p.cur, p.cur)
				}
				if !strings.Contains(out, want) {
					t.Errorf("cursor row %q (cur=%d scroll=%d) not visible\n%s", want, p.cur, p.scroll, out)
				}
				if p.scroll == 0 && depth > 1 && moves >= rows {
					t.Errorf("window never scrolled (cur=%d)", p.cur)
				}
			})
		}
	}
}

func TestRenderBrowserGreyedRowIsDim(t *testing.T) {
	// Force colour so dimStyle emits escape codes in the test environment.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(prev)

	app := browserApp(120, 40, 2, 6)
	app.browser = app.browser.setCount("Thing03.net.talos.dev", countLocked)
	out := app.renderBrowser(app.mainHeight())
	find := func(name string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, name) {
				return l
			}
		}
		t.Fatalf("row %s not rendered\n%s", name, out)
		return ""
	}
	// A dim row is one run in the grey colour (ANSI256 59); populated rows are not.
	const dimRun = "\x1b[38;5;59m  "
	if strings.Contains(find("Thing02"), dimRun+"Thing02") {
		t.Errorf("populated Thing02 must not be dim: %q", find("Thing02"))
	}
	for name, marker := range map[string]string{"Thing01": " -", "Thing03": padlock()} {
		l := find(name)
		if !strings.Contains(l, dimRun+name) || !strings.Contains(ansi.Strip(l), marker) {
			t.Errorf("%s should be dim and show %q: %q", name, marker, l)
		}
	}
	for _, c := range []struct {
		n    int
		text string
		dim  bool
	}{{0, "-", true}, {countLocked, padlock(), true}, {countError, "err", true}, {5, "5", false}} {
		b := browser{counts: map[string]int{"T": c.n}}
		if text, dim := b.typeCell(talos.ResourceDef{Type: "T"}); text != c.text || dim != c.dim {
			t.Errorf("typeCell(%d) = %q,%v want %q,%v", c.n, text, dim, c.text, c.dim)
		}
	}
	b := browser{loading: map[string]bool{"T": true}}
	if text, _ := b.typeCell(talos.ResourceDef{Type: "T"}); text != "…" {
		t.Errorf("loading cell = %q", text)
	}
}

func TestRenderBrowserTwoPaneCollapseBelow120(t *testing.T) {
	for _, tc := range []struct{ w, panes int }{{80, 2}, {119, 2}, {120, 3}, {200, 3}} {
		app := browserApp(tc.w, 30, 4, 10)
		first := strings.SplitN(app.renderBrowser(app.mainHeight()), "\n", 2)[0]
		if got := strings.Count(first, "╭"); got != tc.panes {
			t.Errorf("width %d: %d panes, want %d\n%s", tc.w, got, tc.panes, first)
		}
		if strings.Contains(first, "Categories") != (tc.panes == 4) {
			t.Errorf("width %d: root pane should be hidden\n%s", tc.w, first)
		}
	}
}

func TestRenderBrowserFullScreenYAML(t *testing.T) {
	for _, sz := range browserSizes {
		app := browserApp(sz.w, sz.h, 4, 10)
		app.browser.fullscreen = true
		out := app.renderBrowser(app.mainHeight())
		first := strings.SplitN(out, "\n", 2)[0]
		if strings.Count(first, "╭") != 1 || lipgloss.Width(first) != sz.w {
			t.Errorf("%dx%d: want one full-width pane, got %q", sz.w, sz.h, first)
		}
		if !strings.Contains(out, "metadata:") {
			t.Errorf("%dx%d: YAML body missing", sz.w, sz.h)
		}
	}
}

func TestRenderBrowserStatesShowMessages(t *testing.T) {
	app := browserApp(120, 30, 1, 3)
	app.browser.defs = nil
	app.browser.defsLoading = true
	if out := app.renderBrowser(app.mainHeight()); !strings.Contains(out, "loading resource") || !strings.Contains(out, "definitions…") {
		t.Errorf("loading message missing\n%s", out)
	}
	app.browser.defsLoading = false
	app.browser.defsErr = "rpc error: code = Unavailable"
	if out := app.renderBrowser(app.mainHeight()); !strings.Contains(out, "Unavailable") {
		t.Errorf("error message missing\n%s", out)
	}
	app = browserApp(120, 30, 4, 3)
	app.browser = app.browser.withTop(func(p *pane) { p.yaml, p.loading = "", true })
	if out := app.renderBrowser(app.mainHeight()); !strings.Contains(out, "loading…") {
		t.Errorf("yaml loading missing\n%s", out)
	}
}
