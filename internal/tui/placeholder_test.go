package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
)

func TestPlaceholderResize(t *testing.T) {
	id := identity.SessionIdentity{Fingerprint: "SHA256:test", Slot: "farm"}
	m := NewPlaceholder(id, 80, 24)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	view := next.(placeholder).View().Content
	if !strings.Contains(view, "SHA256:test") {
		t.Fatalf("expected fingerprint in view, got %q", view)
	}
	if next.(placeholder).width != 120 || next.(placeholder).height != 40 {
		t.Fatalf("expected resize to 120x40, got %dx%d", next.(placeholder).width, next.(placeholder).height)
	}
}

