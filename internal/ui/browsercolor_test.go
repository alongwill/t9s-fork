package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestKindSuffix(t *testing.T) {
	for in, want := range map[string][2]string{
		"LinkStatus": {"Link", "Status"}, "AddressSpec": {"Address", "Spec"},
		"LinkConfig": {"Link", "Config"}, "Node": {"Node", ""}, "Status": {"Status", ""},
	} {
		if st, sf := kindSuffix(in); st != want[0] || sf != want[1] {
			t.Errorf("kindSuffix(%q) = %q %q", in, st, sf)
		}
	}
}

func TestCategoryAccentsCoverEveryCategory(t *testing.T) {
	seen := map[string]bool{}
	for key := range categoryAccents {
		seen[key] = true
	}
	for _, key := range []string{"networking", "block", "storage", "kubernetes", "cluster", "security", "runtime"} {
		if !seen[key] {
			t.Errorf("no accent for %s", key)
		}
	}
	if categoryAccent("nope") != accentDefault {
		t.Error("unknown category must fall back to the default accent")
	}
}

// No pane of the browser may draw a line wider than the terminal once ANSI is
// stripped, at any size.
func TestNoRenderedLineExceedsWidthWithColour(t *testing.T) {
	for _, sz := range append(relSizes, struct{ w, h int }{60, 20}) {
		sz := sz
		t.Run(fmt.Sprintf("w%d_h%d", sz.w, sz.h), func(t *testing.T) {
			apps := map[string]App{
				"related":   relApp(sz.w, sz.h),
				"categs":    browserApp(sz.w, sz.h, 1, 12),
				"types":     browserApp(sz.w, sz.h, 2, 12),
				"instances": browserApp(sz.w, sz.h, 3, 12),
				"yaml":      browserApp(sz.w, sz.h, 4, 12),
			}
			for name, app := range apps {
				full := app.browserHeaderStyled() + "\n" + app.renderBrowser(app.mainHeight())
				for i, l := range strings.Split(full, "\n") {
					if w := ansi.StringWidth(l); w > sz.w {
						t.Errorf("%s: line %d is %d cells wide, terminal is %d: %q", name, i, w, sz.w, ansi.Strip(l))
					}
				}
			}
		})
	}
}

func TestHeaderShowsRoleAndSourceChips(t *testing.T) {
	app := browserApp(160, 30, 2, 6)
	got := ansi.Strip(app.browserHeaderStyled())
	for _, want := range []string{"node:", "cli", "Networking"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing from %q", want, got)
		}
	}
	if strings.Contains(app.breadcrumb(), "CP") {
		t.Error("the plain breadcrumb must stay chip-free")
	}
}
