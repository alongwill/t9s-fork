package ui

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/florianspk/t9s/internal/talos"
)

// schematicMsg is the result of looking up a node's schematic and fetching its
// YAML from the Image Factory.
type schematicMsg struct {
	seq     uint64
	info    talos.SchematicInfo
	hasInfo bool
	yaml    string
	err     error // lookup or fetch error; info may still be set
}

// schematicInfoMsg fills the Extensions view's header line.
type schematicInfoMsg struct {
	node string
	info talos.SchematicInfo
	ok   bool
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func factoryHost(apiURL string) string {
	if u, err := url.Parse(apiURL); err == nil && u.Host != "" {
		return u.Host
	}
	return apiURL
}

// loadSchematicInfo reads the schematic ID for the Extensions header line.
func (app App) loadSchematicInfo() tea.Cmd {
	client, node := app.client, app.selNode.IP
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		info, err := client.GetSchematicInfo(ctx, node)
		return schematicInfoMsg{node: node, info: info, ok: err == nil}
	}
}

// openSchematic shows the schematic pane for node, remembering where to
// return. The lookup and the factory request run in one command.
func (app App) openSchematic(node string, known *talos.SchematicInfo) (App, tea.Cmd) {
	app.schemOrigin = app.state
	app.schemSeq++
	app.schemInfo, app.schemYAML, app.schemErr = talos.SchematicInfo{}, "", ""
	app.schemLoading = true
	if known != nil {
		app.schemInfo = *known
	}
	app.schemVP = viewport.New(max(1, app.width), max(1, app.mainHeight()))
	app = app.goTo(StateSchematic)
	app.statusMsg = "Fetching schematic..."
	client, seq := app.client, app.schemSeq
	have := known
	return app, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		msg := schematicMsg{seq: seq}
		if have != nil {
			msg.info, msg.hasInfo = *have, true
		} else {
			info, err := client.GetSchematicInfo(ctx, node)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.info, msg.hasInfo = info, true
		}
		msg.yaml, msg.err = talos.FetchSchematicYAML(ctx, msg.info.APIURL, msg.info.ID)
		return msg
	}
}

func (app App) handleSchematic(msg schematicMsg) App {
	if msg.seq != app.schemSeq {
		return app
	}
	app.schemLoading = false
	if msg.hasInfo {
		app.schemInfo = msg.info
	}
	app.statusMsg = ""
	switch {
	case msg.err == nil:
		app.schemYAML = msg.yaml
	case !msg.hasInfo:
		app.schemErr = "could not find the schematic: " + msg.err.Error()
	case errorsIsAuth(msg.err):
		app.schemErr = padlock() + " the factory needs authentication; schematic ID " + msg.info.ID
	default:
		app.schemErr = "could not fetch the schematic: " + msg.err.Error() +
			"\nschematic ID " + msg.info.ID + "\nURL " + schematicURLText(msg.info)
	}
	app.schemVP.SetContent(ansi.Hardwrap(app.schematicBody(), max(10, app.width), true))
	app.schemVP.GotoTop()
	return app
}

func errorsIsAuth(err error) bool { return strings.Contains(err.Error(), talos.ErrFactoryAuth.Error()) }

func schematicURLText(i talos.SchematicInfo) string {
	if u, err := talos.SchematicURL(i.APIURL, i.ID); err == nil {
		return u
	}
	return i.APIURL
}

// schematicBody is the viewport content: facts, then the YAML or the error.
func (app App) schematicBody() string {
	i := app.schemInfo
	var sb strings.Builder
	if i.ID != "" {
		sb.WriteString(dimStyle.Render("schematic ID ") + i.ID + "\n")
		if i.Flavor != "" {
			sb.WriteString(dimStyle.Render("flavor       ") + i.Flavor + "\n")
		}
		sb.WriteString(dimStyle.Render("factory      ") + i.APIURL + "\n\n")
	}
	if app.schemErr != "" {
		sb.WriteString(warnStyle.Render(app.schemErr))
		return sb.String()
	}
	sb.WriteString(colorYAMLText(strings.TrimRight(app.schemYAML, "\n"), ""))
	return sb.String()
}

func (app App) handleSchematicKey(msg tea.KeyMsg) (App, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		app.cleanup()
		return app, tea.Quit
	case "esc", "q":
		app.schemSeq++ // drop a reply still in flight
		app.state = app.schemOrigin
		app.statusMsg = ""
		return app, nil
	case "g":
		app.schemVP.GotoTop()
	case "G":
		app.schemVP.GotoBottom()
	default:
		var cmd tea.Cmd
		app.schemVP, cmd = app.schemVP.Update(msg)
		return app, cmd
	}
	return app, nil
}

func (app App) renderSchematic(height int) string {
	title := "  Schematic " + titleStyle.Render(shortID(app.schemInfo.ID)) + "\n"
	if app.schemLoading {
		return title + lipgloss.Place(app.width, max(1, height-1), lipgloss.Center, lipgloss.Center,
			infoStyle.Render("Fetching schematic from the Image Factory..."))
	}
	app.schemVP.Width, app.schemVP.Height = app.width, max(1, height-1)
	lines := strings.Split(app.schemVP.View(), "\n")
	for i, l := range lines {
		lines[i] = clipANSI(l, app.width)
	}
	return title + strings.Join(lines, "\n") + "\n"
}

// extensionsHeaderLine is `schematic <short id> · <factory host>`, shown once
// the schematic is known.
func (app App) extensionsHeaderLine() string {
	if !app.extSchemOK {
		return ""
	}
	return "  " + dimStyle.Render("schematic ") + shortID(app.extSchem.ID) + dimStyle.Render(" · "+factoryHost(app.extSchem.APIURL)+"   ↵ shows the YAML")
}
