package tui

import (
	"strings"
	"testing"
)

func TestWrapIndent(t *testing.T) {
	got := wrapIndent(20, "  ", "one two three four five six")
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(line)) > 20 {
			t.Fatalf("line exceeds width 20: %q", line)
		}
	}
	if !strings.HasPrefix(got, "  ") {
		t.Fatalf("expected indent prefix, got %q", got)
	}
}
