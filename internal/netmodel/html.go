package netmodel

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

//go:embed web/diagram.html
var diagramTemplate string

const (
	titleMarker = "<!--T9S_TITLE-->"
	modelMarker = "<!--T9S_MODEL-->"
)

// ModelJSON is the model as embedded in the page. encoding/json escapes <, >, &
// and U+2028/9 as \uXXXX, so no value can end the <script> element it sits in.
func ModelJSON(m Model) ([]byte, error) { return json.Marshal(m) }

// RenderHTML returns the self-contained stack diagram page for the model.
func RenderHTML(m Model) ([]byte, error) {
	data, err := ModelJSON(m)
	if err != nil {
		return nil, err
	}
	name := m.Hostname
	if name == "" {
		name = m.Node
	}
	title := "Network of " + name
	r := strings.NewReplacer(titleMarker, html.EscapeString(title), modelMarker, string(data))
	return []byte(r.Replace(diagramTemplate)), nil
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// FileName is t9s-network-<hostname>-<timestamp>.html with the hostname made
// safe for a file name.
func FileName(m Model, now time.Time) string {
	name := m.Hostname
	if name == "" {
		name = m.Node
	}
	name = strings.Trim(unsafeName.ReplaceAllString(name, "-"), "-.")
	if name == "" {
		name = "node"
	}
	return fmt.Sprintf("t9s-network-%s-%s.html", name, now.Format("20060102-150405"))
}

// WriteHTML writes the page into dir (os.TempDir when dir is empty) and
// returns its path.
func WriteHTML(m Model, dir string, now time.Time) (string, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	page, err := RenderHTML(m)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, FileName(m, now))
	if err := os.WriteFile(path, page, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
