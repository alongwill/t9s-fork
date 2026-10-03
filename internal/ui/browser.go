package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/florianspk/t9s/internal/catalog"
	"github.com/florianspk/t9s/internal/talos"
)

// Resource browser: node → categories → types → instances → YAML.
// All state lives in App.browser as plain values; the pane stack decides what
// is on screen. Mutators clone the slices/maps they touch because App is a
// value receiver.

type paneKind int

const (
	paneCategories paneKind = iota
	paneTypes
	paneInstances
	paneYAML
	paneDescribe
	paneAliases // ctrl+a palette
	paneCompare // c: same resource on every node
	paneDiff    // enter on a compare row: unified diff
)

// Count sentinels in browser.counts (missing key = not loaded yet).
const (
	countLocked = -1 // permission denied (needs os:admin)
	countError  = -2 // any other failure
)

type pane struct {
	kind   paneKind
	title  string
	cur    int
	scroll int
	filter string // active `/` filter for this pane

	category string               // paneTypes: category key
	def      talos.ResourceDef    // paneInstances, paneYAML
	meta     talos.ResourceMeta   // paneYAML
	items    []talos.ResourceMeta // paneInstances
	yaml     string               // paneYAML
	cfgKind  string               // paneInstances, paneYAML: set when showing config documents
	desc     []descLine           // paneDescribe
	cmp      compareView          // paneCompare
	diff     []diffLine           // paneDiff
	loading  bool
	err      string
}

type promptKind int

const (
	promptFilter promptKind = iota
	promptFind
)

type browser struct {
	node    talos.Node
	defs    []talos.ResourceDef
	counts  map[string]int // type → instances; -1 locked, -2 error; missing = not loaded
	singles map[string]talos.ResourceMeta
	loading map[string]bool
	stack   []pane // push on Enter, pop on Esc/q

	fullscreen bool // YAML pane `f`
	wrap       bool // YAML pane `w`

	flash map[string]time.Time // instance ID → highlight until (live watch)

	find     string // YAML `/` search
	findHits []int  // logical line indexes that match
	findIdx  int

	defsLoading bool
	defsErr     string
	pendingCmd  string // `:` command waiting for the definitions

	docs     []talos.ConfigDoc // the node's machine config documents
	cfgState cfgState
	cfgErr   string

	prompting  bool
	promptKind promptKind
	promptPrev string // filter to restore if the prompt is cancelled
	input      textinput.Model
}

func isBrowserState(s AppState) bool { return s == StateCategories || s == StateBrowser }

// --- stack helpers (all return copies) ---

func (b browser) top() (pane, bool) {
	if len(b.stack) == 0 {
		return pane{}, false
	}
	return b.stack[len(b.stack)-1], true
}

func (b browser) withTop(f func(p *pane)) browser {
	if len(b.stack) == 0 {
		return b
	}
	st := make([]pane, len(b.stack))
	copy(st, b.stack)
	f(&st[len(st)-1])
	b.stack = st
	return b
}

func (b browser) push(p pane) browser {
	st := make([]pane, len(b.stack), len(b.stack)+1)
	copy(st, b.stack)
	b.stack = append(st, p)
	return b
}

func (b browser) pop() browser {
	if len(b.stack) == 0 {
		return b
	}
	st := make([]pane, len(b.stack)-1)
	copy(st, b.stack)
	b.stack = st
	return b
}

// withPane updates the first pane (from the top) matching pred.
func (b browser) withPane(pred func(p pane) bool, f func(p *pane)) browser {
	for i := len(b.stack) - 1; i >= 0; i-- {
		if pred(b.stack[i]) {
			st := make([]pane, len(b.stack))
			copy(st, b.stack)
			f(&st[i])
			b.stack = st
			return b
		}
	}
	return b
}

func (b browser) setCount(typ string, n int) browser {
	m := make(map[string]int, len(b.counts)+1)
	for k, v := range b.counts {
		m[k] = v
	}
	m[typ] = n
	b.counts = m
	return b
}

func (b browser) setLoading(typ string, on bool) browser {
	m := make(map[string]bool, len(b.loading)+1)
	for k, v := range b.loading {
		m[k] = v
	}
	if on {
		m[typ] = true
	} else {
		delete(m, typ)
	}
	b.loading = m
	return b
}

func (b browser) setSingle(typ string, meta talos.ResourceMeta, ok bool) browser {
	m := make(map[string]talos.ResourceMeta, len(b.singles)+1)
	for k, v := range b.singles {
		m[k] = v
	}
	if ok {
		m[typ] = meta
	} else {
		delete(m, typ)
	}
	b.singles = m
	return b
}

// --- filter / search matching ---

// matcher implements the k9s filter grammar: plain text is a case-insensitive
// regex (falling back to a literal substring when invalid); a leading `!`
// inverts the match.
type matcher struct {
	re     *regexp.Regexp
	invert bool
	empty  bool
}

func newMatcher(term string, allowInvert bool) matcher {
	m := matcher{}
	if allowInvert && strings.HasPrefix(term, "!") {
		m.invert = true
		term = term[1:]
	}
	if term == "" {
		m.empty = true
		return m
	}
	re, err := regexp.Compile("(?i)" + term)
	if err != nil {
		re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(term))
	}
	m.re = re
	return m
}

func (m matcher) match(fields ...string) bool {
	if m.empty {
		return true
	}
	hit := false
	for _, f := range fields {
		if m.re.MatchString(f) {
			hit = true
			break
		}
	}
	return hit != m.invert
}

// --- row models ---

type catRow struct {
	key, label string
	present    int
	known      int
	counted    bool // every type in the category has been counted
}

func (b browser) typesIn(cat string) []talos.ResourceDef {
	var out []talos.ResourceDef
	for _, d := range b.defs {
		if catalog.CategoryFor(d) == cat {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].DisplayType) < strings.ToLower(out[j].DisplayType)
	})
	return out
}

func (b browser) categoryRows(filter string) []catRow {
	var rows []catRow
	for _, c := range catalog.Categories {
		defs := b.typesIn(c.Key)
		kinds := b.configKindsIn(c.Key)
		if len(defs)+len(kinds) == 0 {
			continue
		}
		r := catRow{key: c.Key, label: c.Label, known: len(defs) + len(kinds), counted: true}
		if len(kinds) > 0 {
			if b.cfgState != cfgLoaded {
				r.counted = false
			}
			for _, k := range kinds {
				if len(b.docsOfKind(k.Kind)) > 0 {
					r.present++
				}
			}
		}
		for _, d := range defs {
			n, ok := b.counts[d.Type]
			if !ok {
				r.counted = false
			} else if n > 0 {
				r.present++
			}
		}
		rows = append(rows, r)
	}
	return rankFilter(rows, filter, func(r catRow) []string { return []string{r.label, r.key} })
}

func (b browser) typeRows(cat, filter string) []talos.ResourceDef {
	return rankFilter(b.typesIn(cat), filter, func(d talos.ResourceDef) []string {
		return append([]string{d.DisplayType, d.Type}, d.Aliases...)
	})
}

func filterInstances(items []talos.ResourceMeta, filter string) []talos.ResourceMeta {
	return rankFilter(items, filter, func(it talos.ResourceMeta) []string {
		return []string{it.ID, it.Namespace, it.Phase, it.Version}
	})
}

// typeCell describes how a type row's count is shown.
func (b browser) typeCell(d talos.ResourceDef) (text string, dim bool) {
	n, ok := b.counts[d.Type]
	switch {
	case !ok && b.loading[d.Type]:
		return "…", false
	case !ok:
		return "?", false
	case n == countLocked:
		return "lock", true
	case n == countError:
		return "err", true
	case n == 0:
		return "-", true
	}
	return fmt.Sprint(n), false
}

// --- layout ---

const (
	widthCategories = 26
	widthTypes      = 40
	widthInstances  = 36
	minYAMLWidth    = 24
)

type paneBox struct{ idx, w int }

// isFlexPane reports panes that take the space the list panes leave over.
func isFlexPane(k paneKind) bool { return k == paneYAML || k == paneDescribe }

func fixedWidth(k paneKind) int {
	switch k {
	case paneCategories:
		return widthCategories
	case paneTypes:
		return widthTypes
	case paneInstances:
		return widthInstances
	}
	return minYAMLWidth
}

// browserLayout decides which stack panes are visible and how wide each is.
func (app App) browserLayout() []paneBox {
	st := app.browser.stack
	n := len(st)
	if n == 0 {
		return nil
	}
	if k := st[n-1].kind; k == paneAliases || k == paneCompare || k == paneDiff { // whole width
		return []paneBox{{n - 1, app.width}}
	}
	if app.browser.fullscreen && st[n-1].kind == paneYAML {
		return []paneBox{{n - 1, app.width}}
	}
	maxPanes := 2
	if app.width >= 120 {
		maxPanes = 3
	}
	first := max(0, n-maxPanes)
	for {
		used := 0
		for i := first; i < n-1; i++ {
			used += fixedWidth(st[i].kind)
		}
		rest := app.width - used
		if rest >= minYAMLWidth || first >= n-1 {
			var boxes []paneBox
			for i := first; i < n-1; i++ {
				boxes = append(boxes, paneBox{i, fixedWidth(st[i].kind)})
			}
			last := st[n-1]
			w := rest
			if !isFlexPane(last.kind) {
				w = min(rest, fixedWidth(last.kind))
			}
			if w < 4 {
				w = max(4, min(app.width, fixedWidth(last.kind)))
			}
			return append(boxes, paneBox{n - 1, w})
		}
		first++
	}
}

// paneInnerRows is the number of list rows (or YAML lines) visible in a pane.
func (app App) paneInnerRows(k paneKind) int {
	rows := app.mainHeight() - 2 - app.nextStepRows()
	if k == paneInstances || k == paneAliases || k == paneCompare {
		rows-- // column header
	}
	return max(1, rows)
}

func (app App) yamlInnerWidth() int {
	layout := app.browserLayout()
	if len(layout) == 0 {
		return max(1, app.width-2)
	}
	return max(1, layout[len(layout)-1].w-2)
}

// --- text helpers ---

func cutWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var sb strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > w {
			break
		}
		sb.WriteRune(r)
		cur += rw
	}
	return sb.String()
}

// fit cuts or pads plain text to exactly w display cells.
func fit(s string, w int) string { return padRight(cutWidth(s, w), w) }

func wrapChunks(s string, w int) []string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return []string{s}
	}
	var out []string
	var sb strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > w && cur > 0 {
			out = append(out, sb.String())
			sb.Reset()
			cur = 0
		}
		sb.WriteRune(r)
		cur += rw
	}
	return append(out, sb.String())
}

// vline is one visual (possibly wrapped) YAML line plus the logical line it came from.
type vline struct {
	text    string
	logical int
}

func yamlVisual(text string, w int, wrap bool) []vline {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	var out []vline
	for i, l := range strings.Split(text, "\n") {
		l = strings.ReplaceAll(strings.TrimRight(l, "\r"), "\t", "  ")
		if wrap {
			for _, c := range wrapChunks(l, w) {
				out = append(out, vline{c, i})
			}
		} else {
			out = append(out, vline{cutWidth(l, w), i})
		}
	}
	return out
}

func yamlFindHits(text, term string) []int {
	if term == "" {
		return nil
	}
	m := newMatcher(term, false)
	var hits []int
	for i, l := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if m.match(l) {
			hits = append(hits, i)
		}
	}
	return hits
}

// --- pane metrics ---

// paneLen is the number of selectable rows (lists) or visual lines (YAML).
func (app App) paneLen(p pane) int {
	b := app.browser
	switch p.kind {
	case paneCategories:
		return len(b.categoryRows(p.filter))
	case paneTypes:
		return len(b.typeEntries(p.category, p.filter))
	case paneInstances:
		return len(filterInstances(p.items, p.filter))
	case paneYAML:
		return len(yamlVisual(p.yaml, app.yamlInnerWidth(), b.wrap))
	case paneDescribe:
		return app.paneDescribeLen(p)
	case paneAliases:
		return len(b.paletteRows(p.filter))
	case paneCompare:
		return len(p.cmp.rows)
	case paneDiff:
		return len(p.diff)
	}
	return 0
}

// breadcrumb renders `node: host (role) > Category > Type > id`.
func (app App) breadcrumb() string {
	b := app.browser
	parts := []string{fmt.Sprintf("node: %s (%s)", b.node.Hostname, b.node.Role)}
	for _, p := range b.stack {
		switch p.kind {
		case paneTypes:
			parts = append(parts, catalog.Label(p.category))
		case paneInstances:
			parts = append(parts, p.title)
		case paneYAML:
			parts = append(parts, p.meta.ID)
		case paneDescribe:
			parts = append(parts, "describe")
		case paneAliases:
			parts = append(parts, "all types")
		case paneCompare:
			parts = append(parts, "compare nodes")
		case paneDiff:
			parts = append(parts, "diff")
		}
	}
	return strings.Join(parts, " > ")
}

// browserIndicators is the right-aligned part of the header line.
func (app App) browserIndicators() string {
	return joinIndicators(app.watchIndicator(), "src: "+app.sourceName())
}

// browserHeaderLine is the breadcrumb with the indicators pushed to the right.
func (app App) browserHeaderLine() string {
	avail := max(1, app.width-2)
	ind := app.browserIndicators()
	if lipgloss.Width(ind)+12 > avail { // too narrow: breadcrumb only
		return cutWidth(app.breadcrumb(), avail)
	}
	left := cutWidth(app.breadcrumb(), avail-lipgloss.Width(ind)-2)
	return padRight(left, avail-lipgloss.Width(ind)) + ind
}

// syncBrowserState keeps AppState in step with the stack depth.
func (app App) syncBrowserState() App {
	if len(app.browser.stack) <= 1 {
		app.state = StateCategories
	} else {
		app.state = StateBrowser
	}
	return app
}
