package ui

import (
	"context"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	"github.com/florianspk/t9s/internal/config"
)

// identity is what t9s knows about who it talks to: whether the context goes
// through Omni (from the talosconfig, or later from the cluster).
type identity struct {
	omni config.Omni
}

func newIdentity(cfg *config.TalosConfig, ctxName string) identity {
	return identity{omni: cfg.Named(ctxName).DetectOmni()}
}

// ctxName is the active talosconfig context.
func (app App) ctxName() string {
	if app.talosCtx != "" {
		return app.talosCtx
	}
	if app.cfg != nil {
		return app.cfg.Context
	}
	return ""
}

// siderolinkStatusType is read on a node to tell whether it is SideroLink
// connected (an Omni-managed machine). It is not a sensitive type, unlike
// SiderolinkConfigs, which carries the join token.
const siderolinkStatusType = "SiderolinkStatuses.siderolink.talos.dev"

// omniProbeMsg is the result of the lazy SideroLink check.
type omniProbeMsg struct {
	ctx   string // talosconfig context the probe ran for
	found bool
	host  string
	err   error
}

// probeOmni is path (c) of the Omni detection: after the node list loads, look
// for a SiderolinkStatus on the first node. It runs once per context, and only
// when the talosconfig did not already say Omni.
func (app App) probeOmni() (App, tea.Cmd) {
	if app.id.omni.Detected || app.omniProbed || app.omniProbing || len(app.nodes) == 0 {
		return app, nil
	}
	app.omniProbing = true
	src := app.src()
	node := app.nodes[0].IP
	name := app.ctxName()
	return app, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		items, err := src.List(ctx, node, "config", siderolinkStatusType)
		if err != nil {
			return omniProbeMsg{ctx: name, err: err}
		}
		if len(items) == 0 {
			return omniProbeMsg{ctx: name}
		}
		host := ""
		if y, err := src.GetYAML(ctx, node, "config", siderolinkStatusType, items[0].ID); err == nil {
			host = siderolinkHost(y)
		}
		return omniProbeMsg{ctx: name, found: true, host: host}
	}
}

func (app App) handleOmniProbe(msg omniProbeMsg) App {
	if msg.ctx != app.ctxName() {
		return app // a reply for the context we switched away from
	}
	app.omniProbing = false
	if msg.err != nil {
		return app // try again after the next node refresh
	}
	app.omniProbed = true
	if msg.found && !app.id.omni.Detected {
		app.id.omni = config.Omni{Detected: true, Host: msg.host, Via: config.ViaSideroLink}
	}
	return app
}

// siderolinkHost extracts the host of a SiderolinkStatus YAML. Only the host
// is kept: the value can carry a scheme, a port or a join token in a query.
func siderolinkHost(y string) string {
	var doc struct {
		Spec struct {
			Host string `yaml:"host"`
		} `yaml:"spec"`
	}
	if yaml.Unmarshal([]byte(y), &doc) != nil {
		return ""
	}
	h := strings.TrimSpace(doc.Spec.Host)
	if h == "" {
		return ""
	}
	if !strings.Contains(h, "://") {
		h = "https://" + h
	}
	u, err := url.Parse(h)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// platformBadge is the top-bar badge for the control plane: purple "Omni
// <host>" when Omni is detected, plain "Talos" otherwise.
func (app App) platformBadge(withHost bool) string {
	if app.id.omni.Detected {
		label := " Omni"
		if withHost && app.id.omni.Host != "" {
			label += " " + app.id.omni.Host
		}
		return lipgloss.NewStyle().Background(colorMagenta).Foreground(lipgloss.Color("#0d1117")).Bold(true).Render(label + " ")
	}
	return headerStyle.Render("Talos")
}

// maxHeaderLevel is the most compact top-bar layout headerLeft knows.
const maxHeaderLevel = 4

// headerLeft builds the left part of the top bar. Level 0 shows everything;
// each higher level drops something: 1 the selected node, 2 the Omni host
// text, 3 the context, 4 the platform badge.
func (app App) headerLeft(level int) string {
	logo := lipgloss.NewStyle().
		Background(colorBgHead).
		Foreground(colorCyan).
		Bold(true).
		Render(" t9s ")
	sep := headerSepStyle.Render("│")

	left := logo + sep + app.modeBadge()
	if level < 4 {
		badge := app.platformBadge(level < 2)
		left += sep + badge
	}
	if level < 3 {
		left += sep + headerStyle.Render(" ctx: "+app.ctxName()+" ")
	}
	if level < 1 && app.selNode != nil {
		left += sep + headerStyle.Render(" "+app.selNode.Hostname+" ("+app.selNode.IP+") ")
	}
	return left
}
