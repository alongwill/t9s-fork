package talos

import (
	"context"
	"errors"
	"testing"
)

func TestCLISourceWatchUnsupported(t *testing.T) {
	s := NewCLISource(New("", ""))
	if s.Name() != "cli" {
		t.Fatalf("name = %q", s.Name())
	}
	err := s.Watch(context.Background(), "n", "ns", "t", make(chan WatchEvent))
	if !errors.Is(err, ErrWatchUnsupported) {
		t.Fatalf("err = %v", err)
	}
}
