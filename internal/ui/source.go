package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/talos"
)

// Source modes for --source.
const (
	SourceAuto = "auto"
	SourceGRPC = "grpc"
	SourceCLI  = "cli"
)

const grpcDialTimeout = 5 * time.Second

// ValidSourceMode reports whether s is a --source value.
func ValidSourceMode(s string) bool {
	return s == SourceAuto || s == SourceGRPC || s == SourceCLI
}

// WithSourceMode picks how the resource browser reads COSI resources.
func (app App) WithSourceMode(mode string) App {
	if !ValidSourceMode(mode) {
		mode = SourceAuto
	}
	app.sourceMode = mode
	return app
}

type sourceReadyMsg struct {
	ctx string // talos context the connection was made for
	src talos.ResourceSource
	err error
}

// connectSource dials the gRPC source in the background. Until it answers the
// browser uses the CLI source, so startup is never blocked.
func (app App) connectSource() tea.Cmd {
	if app.sourceMode == SourceCLI {
		return nil
	}
	dial := app.dialSource
	if dial == nil {
		dial = talos.NewGRPCSource
	}
	cfgPath, tctx := app.cfgPath, app.talosCtx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), grpcDialTimeout)
		defer cancel()
		src, err := dial(ctx, cfgPath, tctx)
		return sourceReadyMsg{ctx: tctx, src: src, err: err}
	}
}

func (app App) handleSourceReady(msg sourceReadyMsg) App {
	if msg.ctx != app.talosCtx { // the user switched context while dialing
		if msg.src != nil {
			_ = msg.src.Close()
		}
		return app
	}
	if msg.err != nil {
		app.statusMsg = warnStyle.Render(fmt.Sprintf("gRPC unavailable (%s), using CLI", shortReason(msg.err)))
		return app
	}
	if app.source != nil {
		_ = app.source.Close()
	}
	app.source = msg.src
	return app
}

// resetSource drops the connection (context switch) and redials.
func (app App) resetSource() (App, tea.Cmd) {
	if app.source != nil {
		_ = app.source.Close()
		app.source = nil
	}
	return app, app.connectSource()
}

// shortReason trims an error to one short line for the status bar.
func shortReason(err error) string {
	s := err.Error()
	if i := strings.Index(s, "desc = "); i >= 0 {
		s = s[i+len("desc = "):]
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return truncate(strings.TrimSpace(s), 60)
}

// sourceName is the active browser source: "grpc" or "cli".
func (app App) sourceName() string { return app.src().Name() }

// hasWatch reports whether the active source can stream changes.
func (app App) hasWatch() bool { return app.sourceName() == "grpc" }
