package ui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/catalog"
)

// `:` command mode (k9s §6). The prompt lives in the status line and is
// available from the node list and every browser pane.
//
// Grammar:
//
//	:nodes | :no                        node list
//	:aliases | :alias | :a              all-types palette
//	:q | :q! | :quit                    quit
//	:? | :h | :help                     help
//	:<category key or label prefix>     that category on the current node
//	:<alias | display type | full type | config kind>   jump (as the palette)

type cmdPrompt struct {
	active  bool
	input   textinput.Model
	sugIdx  int  // which suggestion is shown
	histIdx int  // position while cycling history; len(history) = not cycling
	hist    bool // currently showing a history entry
}

func isCmdState(s AppState) bool { return s == StateNodeList || isBrowserState(s) }

func (app App) openCommandPrompt() (App, tea.Cmd) {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 100
	app.cmd = cmdPrompt{active: true, input: ti, histIdx: len(app.cmdHistory)}
	app.statusMsg = ""
	return app, app.cmd.input.Focus()
}

func (app App) closeCommandPrompt() App {
	app.cmd.active = false
	app.cmd.input.Blur()
	return app
}

// commandBrowser is the browser whose entries the command can reach: the
// open one, or a bare one for the selected node (cached definitions only).
func (app App) commandBrowser() (browser, bool) {
	if isBrowserState(app.state) {
		return app.browser, true
	}
	n := app.selectedNode()
	if n == nil {
		return browser{}, false
	}
	b := browser{node: *n, defs: app.resourceDefs[n.IP]}
	if e, ok := app.configDocs[n.IP]; ok {
		b.docs, b.cfgState = e.docs, cfgLoaded
		if e.denied {
			b.cfgState = cfgDenied
		}
	}
	return b, true
}

var commandWords = []string{"nodes", "aliases", "quit", "help"}

// cmdCandidates is every string the prompt can complete to, lower-cased.
func (app App) cmdCandidates() []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.ToLower(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, w := range commandWords {
		add(w)
	}
	for _, c := range catalog.Categories {
		add(c.Key)
	}
	if b, ok := app.commandBrowser(); ok {
		for _, e := range b.paletteEntries() {
			add(e.name)
			add(e.typ)
			for _, a := range e.aliases {
				add(a)
			}
		}
	}
	sort.Strings(out)
	return out
}

// cmdSuggestions returns the completions of typed, best first: candidates the
// input is a prefix of, ranked by fuzzy score, then shorter, then alphabetical.
func (app App) cmdSuggestions(typed string) []string {
	if typed == "" {
		return nil
	}
	low := strings.ToLower(typed)
	type scored struct {
		s     string
		score int
	}
	var hits []scored
	for _, c := range app.cmdCandidates() {
		if len(c) > len(low) && strings.HasPrefix(c, low) {
			sc, _ := fuzzyScore(low, c)
			hits = append(hits, scored{c, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return len(hits[i].s) < len(hits[j].s)
	})
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.s
	}
	return out
}

// cmdSuffix is the dim completion shown after the cursor.
func (app App) cmdSuffix() string {
	typed := app.cmd.input.Value()
	sugs := app.cmdSuggestions(typed)
	if len(sugs) == 0 {
		return ""
	}
	return sugs[app.cmd.sugIdx%len(sugs)][len(typed):]
}

func (app App) handleCommandKey(msg tea.KeyMsg) (App, tea.Cmd) {
	typed := app.cmd.input.Value()
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc":
		return app.closeCommandPrompt(), nil
	case "enter", "ctrl+e":
		app = app.closeCommandPrompt()
		text := strings.TrimSpace(typed)
		if text == "" {
			return app, nil
		}
		if n := len(app.cmdHistory); n == 0 || app.cmdHistory[n-1] != text {
			app.cmdHistory = append(append([]string(nil), app.cmdHistory...), text)
		}
		return app.runCommand(text)
	case "ctrl+u", "ctrl+w":
		app.cmd.input.SetValue("")
		app.cmd.sugIdx, app.cmd.hist = 0, false
		app.cmd.histIdx = len(app.cmdHistory)
		return app, nil
	case "tab", "right":
		if suffix := app.cmdSuffix(); suffix != "" && app.cmd.input.Position() == len(app.cmd.input.Value()) {
			app.cmd.input.SetValue(typed + suffix)
			app.cmd.input.CursorEnd()
			app.cmd.sugIdx = 0
			return app, nil
		}
	case "up", "down":
		dir := 1
		if msg.String() == "up" {
			dir = -1
		}
		if typed == "" || app.cmd.hist { // empty prompt (or already cycling): history
			return app.cycleHistory(dir), nil
		}
		if n := len(app.cmdSuggestions(typed)); n > 0 {
			app.cmd.sugIdx = ((app.cmd.sugIdx+dir)%n + n) % n
		}
		return app, nil
	}
	var cmd tea.Cmd
	app.cmd.input, cmd = app.cmd.input.Update(msg)
	if app.cmd.input.Value() != typed { // editing resets suggestion and history position
		app.cmd.sugIdx, app.cmd.hist = 0, false
		app.cmd.histIdx = len(app.cmdHistory)
	}
	return app, cmd
}

func (app App) cycleHistory(dir int) App {
	n := len(app.cmdHistory)
	if n == 0 {
		return app
	}
	i := app.cmd.histIdx + dir
	if i < 0 {
		i = 0
	}
	if i >= n { // past the newest entry: back to an empty prompt
		app.cmd.histIdx, app.cmd.hist = n, false
		app.cmd.input.SetValue("")
		return app
	}
	app.cmd.histIdx, app.cmd.hist = i, true
	app.cmd.input.SetValue(app.cmdHistory[i])
	app.cmd.input.CursorEnd()
	return app
}

// --- running ---

func (app App) unknownCommand(text string) App {
	app.statusMsg = errStyle.Render("unknown command: " + text)
	return app
}

// matchCategory resolves a category key or label prefix.
func matchCategory(term string) (string, bool) {
	t := strings.ToLower(term)
	for _, c := range catalog.Categories {
		if strings.HasPrefix(c.Key, t) || strings.HasPrefix(strings.ToLower(c.Label), t) {
			return c.Key, true
		}
	}
	return "", false
}

func (app App) runCommand(text string) (App, tea.Cmd) {
	word := strings.ToLower(strings.TrimSpace(text))
	switch word {
	case "nodes", "no":
		if isBrowserState(app.state) {
			app = app.exitBrowser()
		}
		return app, nil
	case "q", "q!", "quit":
		app.cleanup()
		return app, tea.Quit
	case "?", "h", "help":
		app.helpVP.SetContent(buildHelpContent())
		app.helpVP.GotoTop()
		return app.goTo(StateHelp), nil
	}

	// everything else needs a node: the open browser's, or the selected one
	var openCmd tea.Cmd
	if !isBrowserState(app.state) {
		n := app.selectedNode()
		if n == nil {
			app.statusMsg = warnStyle.Render("no node selected")
			return app, nil
		}
		app, openCmd = app.openBrowser(*n)
	}
	var cmd tea.Cmd
	app, cmd = app.runNodeCommand(word)
	return app, tea.Batch(openCmd, cmd)
}

// runNodeCommand runs a command that targets the browser's node. Type lookups
// wait for the resource definitions when they are still loading.
func (app App) runNodeCommand(word string) (App, tea.Cmd) {
	switch word {
	case "a", "alias", "aliases":
		return app.openPalette()
	}
	if key, ok := matchCategory(word); ok {
		return app.jumpCategory(key)
	}
	if e, ok := app.browser.lookupExact(word); ok {
		return app.jumpTo(e)
	}
	if app.browser.defsLoading { // the type may be among the definitions still on their way
		app.browser.pendingCmd = word
		app.statusMsg = dimStyle.Render("loading resource definitions…")
		return app, nil
	}
	return app.unknownCommand(word), nil
}

// runPendingCommand finishes a command that was waiting for the definitions.
func (app App) runPendingCommand() (App, tea.Cmd) {
	word := app.browser.pendingCmd
	if word == "" {
		return app, nil
	}
	app.browser.pendingCmd = ""
	return app.runNodeCommand(word)
}

// --- rendering ---

func (app App) commandFooter(sepLine string) string {
	return sepLine + "\n  " + keyStyle.Render(":") + app.cmd.input.View() + dimStyle.Render(app.cmdSuffix())
}
