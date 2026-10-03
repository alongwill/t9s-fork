package ui

import (
	"testing"

	"github.com/florianspk/t9s/internal/diskmodel"
	"github.com/florianspk/t9s/internal/talos"
)

func diskDefs() []talos.ResourceDef {
	mk := func(typ, display string) talos.ResourceDef {
		return talos.ResourceDef{Type: typ, DisplayType: display, DefaultNamespace: "runtime"}
	}
	return []talos.ResourceDef{
		mk(diskmodel.TypeDisk, "Disk"), mk(diskmodel.TypeDiscovered, "DiscoveredVolume"),
		mk(diskmodel.TypeVolume, "VolumeStatus"), mk(diskmodel.TypeSystemDisk, "SystemDisk"),
	}
}

// diskApp is a browser with the disk view on top, loaded from a fixture.
func diskApp(t *testing.T, fixture string, w, h int) App {
	t.Helper()
	in := diskmodel.Fixture(fixture)
	app := newTestApp(w, h)
	app.nodes = makeNodes(3)
	app.nodeCur = 1
	n := app.nodes[1]
	app.selNode = &n
	app.browser = browser{node: n, defs: diskDefs(), stack: []pane{{kind: paneCategories}}}
	app = app.syncBrowserState()
	app, _ = app.openDisks(true)
	return app.handleDiskFetch(diskFetchMsg{node: n.IP, seq: 1, res: diskmodel.FetchResult{In: in}})
}
