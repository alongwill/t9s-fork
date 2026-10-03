package catalog

import (
	_ "embed"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed resource-notes.yaml
var resourceNotesYAML []byte

// Note is the knowledge text for one resource type.
type Note struct {
	What     string   `yaml:"what"`
	Ubuntu   string   `yaml:"ubuntu"`
	LookWhen []string `yaml:"lookWhen"`
}

var (
	notesOnce sync.Once
	notes     map[string]Note
)

// NoteFor returns the note for a display type (e.g. "LinkStatus").
func NoteFor(displayType string) (Note, bool) {
	notesOnce.Do(func() {
		notes = map[string]Note{}
		_ = yaml.Unmarshal(resourceNotesYAML, &notes)
	})
	n, ok := notes[displayType]
	return n, ok
}

// NoteTypes lists every display type that has a note.
func NoteTypes() []string {
	NoteFor("")
	out := make([]string, 0, len(notes))
	for k := range notes {
		out = append(out, k)
	}
	return out
}
