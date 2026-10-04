package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/florianspk/t9s/internal/config"
	"github.com/florianspk/t9s/internal/ui"
)

var version = "0.1.0"

func main() {
	var (
		cfgPath  string
		talosCtx string
		showVer  bool
		source   string
		readOnly bool
		write    bool
	)

	flag.StringVar(&cfgPath, "talosconfig", "", "Path to talosconfig (default: $TALOSCONFIG or ~/.talos/config)")
	flag.StringVar(&talosCtx, "context", "", "Talos context to use")
	flag.StringVar(&source, "source", ui.SourceAuto, "Resource browser data source: auto (gRPC, falling back to the CLI), grpc, or cli")
	flag.BoolVar(&showVer, "version", false, "Print version and exit")
	flag.BoolVar(&readOnly, "readonly", true, "Read-only mode (default): keys that change a cluster are hidden and refused; --readonly=false is the same as --write")
	flag.BoolVar(&write, "write", false, "Enable actions that change a cluster (reboot, shutdown, upgrades, machine config edit)")
	flag.Parse()

	if showVer {
		fmt.Printf("t9s v%s\n", version)
		os.Exit(0)
	}

	if !ui.ValidSourceMode(source) {
		fmt.Fprintf(os.Stderr, "invalid --source %q (want auto, grpc or cli)\n", source)
		os.Exit(2)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading talosconfig: %v\n\nMake sure talosctl is configured and ~/.talos/config exists.\n", err)
		os.Exit(1)
	}

	if talosCtx == "" {
		talosCtx = cfg.Context
	}

	flagSet := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { flagSet[f.Name] = true })
	allowWrite := resolveWrite(readOnly, write, flagSet, os.Getenv("T9S_READONLY"))

	app := ui.New(cfg, cfgPath, talosCtx).WithSourceMode(source).WithWrite(allowWrite)

	p := tea.NewProgram(app,
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running t9s: %v\n", err)
		os.Exit(1)
	}
}
