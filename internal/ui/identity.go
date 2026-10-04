package ui

import (
	"context"
	"fmt"
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
	omni     config.Omni
	siderov1 bool            // the context signs requests for Omni (no client cert)
	cert     config.CertInfo // client certificate: roles and expiry
	hasCert  bool
	now      func() time.Time // tests; nil = time.Now
}

func newIdentity(cfg *config.TalosConfig, ctxName string) identity {
	c := cfg.Named(ctxName)
	id := identity{omni: c.DetectOmni(), siderov1: c != nil && c.Auth.SideroV1 != nil}
	if ci, err := c.CertInfo(); err == nil {
		id.cert, id.hasCert = ci, true
	}
	return id
}

func (id identity) clock() time.Time {
	if id.now != nil {
		return id.now()
	}
	return time.Now()
}

// viaOmni is true when Omni, not a certificate, decides the Talos role.
func (id identity) viaOmni() bool {
	return id.siderov1 || (id.omni.Detected && !id.hasCert)
}

// roles are the Talos roles of the client certificate (empty for Omni).
func (id identity) roles() []string {
	if id.viaOmni() || !id.hasCert {
		return nil
	}
	return id.cert.Roles
}

// roleSummary is the current role as text, for messages: "os:reader",
// "os:admin, os:etcd:backup", "no role in the certificate", or "via Omni".
func (id identity) roleSummary() string {
	switch {
	case id.viaOmni():
		return "via Omni"
	case !id.hasCert:
		return "unknown"
	case len(id.cert.Roles) == 0:
		return "no role in the certificate"
	}
	return strings.Join(id.cert.Roles, ", ")
}

// certWarnWindow is how long before expiry the top bar starts to warn.
const certWarnWindow = 7 * 24 * time.Hour

// talosRoleChip colours one Talos role: admin red, operator yellow, reader green,
// anything else dim.
func talosRoleChip(role string) string {
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#0d1117")).Bold(true)
	switch role {
	case "os:admin":
		return base.Background(colorRed).Render(" " + role + " ")
	case "os:operator":
		return base.Background(colorYellow).Render(" " + role + " ")
	case "os:reader":
		return base.Background(colorGreen).Render(" " + role + " ")
	}
	return lipgloss.NewStyle().Background(colorBgHead).Foreground(colorGray).Render(" " + role + " ")
}

// roleBadge is the top-bar role part: one chip per certificate role plus the
// expiry warning, or "role: via Omni". Empty when the role is unknown.
func (app App) roleBadge(withExpiry bool) string {
	id := app.id
	if id.viaOmni() {
		return headerDimStyle.Render(" role: via Omni ")
	}
	if !id.hasCert {
		return ""
	}
	var parts []string
	for _, r := range id.cert.Roles {
		parts = append(parts, talosRoleChip(r))
	}
	if len(parts) == 0 {
		parts = append(parts, headerDimStyle.Render(" no roles "))
	}
	out := strings.Join(parts, "")
	if withExpiry {
		warn := lipgloss.NewStyle().Background(colorBgHead).Foreground(colorYellow)
		days, expired, ok := id.cert.ExpiryWarning(id.clock(), certWarnWindow)
		switch {
		case ok && expired:
			out += lipgloss.NewStyle().Background(colorBgHead).Foreground(colorRed).Bold(true).Render(" cert expired ")
		case ok && days == 0:
			out += warn.Render(" cert expires in <1d ")
		case ok:
			out += warn.Render(fmt.Sprintf(" cert expires in %dd ", days))
		}
	}
	return out
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
const maxHeaderLevel = 5

// headerLeft builds the left part of the top bar. Level 0 shows everything;
// each higher level drops something: 1 the selected node, 2 the Omni host text
// and the cert expiry, 3 the context, 4 the platform badge, 5 the role.
func (app App) headerLeft(level int) string {
	logo := lipgloss.NewStyle().
		Background(colorBgHead).
		Foreground(colorCyan).
		Bold(true).
		Render(" t9s ")
	sep := headerSepStyle.Render("│")

	left := logo + sep + app.modeBadge()
	if level < 4 {
		left += sep + app.platformBadge(level < 2)
	}
	if level < 5 {
		if r := app.roleBadge(level < 2); r != "" {
			left += sep + r
		}
	}
	if level < 3 {
		left += sep + headerStyle.Render(" ctx: "+app.ctxName()+" ")
	}
	if level < 1 && app.selNode != nil {
		left += sep + headerStyle.Render(" "+app.selNode.Hostname+" ("+app.selNode.IP+") ")
	}
	return left
}
