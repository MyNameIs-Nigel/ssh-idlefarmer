package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
)

// Model is the Bubble Tea model type used by the Wish middleware.
type Model = tea.Model

// ProgramOption is a Bubble Tea program option.
type ProgramOption = tea.ProgramOption

type placeholder struct {
	identity identity.SessionIdentity
	width    int
	height   int
}

// NewPlaceholder builds the Task 1 proof-of-connection screen.
func NewPlaceholder(id identity.SessionIdentity, width, height int) tea.Model {
	if width < 1 {
		width = 80
	}
	if height < 1 {
		height = 24
	}
	return placeholder{
		identity: id,
		width:    width,
		height:   height,
	}
}

func (m placeholder) Init() tea.Cmd {
	return nil
}

func (m placeholder) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m placeholder) View() tea.View {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	valueStyle := lipgloss.NewStyle()
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Italic(true)

	lines := []string{
		titleStyle.Render("ssh-idlefarmer"),
		"",
		labelStyle.Render("Key fingerprint") + ":",
		valueStyle.Render(m.identity.Fingerprint),
		"",
		labelStyle.Render("Save slot") + ":",
		valueStyle.Render(m.identity.Slot),
		"",
		hintStyle.Render("Press q or Ctrl+C to disconnect."),
	}

	content := strings.Join(lines, "\n")
	// Center vertically when the terminal is tall enough.
	if m.height > 0 {
		pad := (m.height - strings.Count(content, "\n") - 1) / 2
		if pad > 0 {
			content = strings.Repeat("\n", pad) + content
		}
	}

	if m.width > 0 {
		content = lipgloss.NewStyle().Width(m.width).Render(content)
	}
	return tea.NewView(content)
}

// String for debugging only.
func (m placeholder) String() string {
	return fmt.Sprintf("placeholder{%s/%s}", m.identity.Fingerprint, m.identity.Slot)
}
