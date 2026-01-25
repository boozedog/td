package monitor

import (
	"path/filepath"

	"github.com/marcus/td/pkg/monitor/modal"
)

// createGettingStartedModal builds the Getting Started modal.
// Call this when opening the modal (not at init time) so it reflects current state.
func (m *Model) createGettingStartedModal() *modal.Modal {
	// Determine which file to suggest
	fileName := "AGENTS.md"
	if m.AgentFilePath != "" {
		fileName = filepath.Base(m.AgentFilePath)
	}

	md := modal.New("Welcome to td monitor!", modal.WithWidth(65))

	md.AddSection(modal.Text(
		"td is a task management system designed for AI agents.\n" +
			"It helps track work, coordinate handoffs, and maintain context."))

	md.AddSection(modal.Spacer())

	if m.AgentFileHasTD {
		md.AddSection(modal.Text("\u2713 Agent instructions already installed"))
	} else {
		md.AddSection(modal.Text(
			"SETUP YOUR AGENT:\n" +
				"  Press 'I' to add td instructions to " + fileName))
	}

	md.AddSection(modal.Spacer())

	md.AddSection(modal.Text(
		"PROMPT YOUR AGENT:\n" +
			"  \"Use td to plan {{my feature}}, create tasks, then implement it.\""))

	md.AddSection(modal.Spacer())

	md.AddSection(modal.Text(
		"KEYBOARD SHORTCUTS:\n" +
			"  Press '?' anytime for full help"))

	md.AddSection(modal.Spacer())

	md.AddSection(modal.Text(
		"DOCUMENTATION:\n" +
			"  https://marcus.github.io/td/docs/intro"))

	md.AddSection(modal.Spacer())

	// Only show Install button if not already installed
	if m.AgentFileHasTD {
		md.AddSection(modal.Buttons(
			modal.Btn(" Close ", "close"),
		))
	} else {
		md.AddSection(modal.Buttons(
			modal.Btn(" Install Instructions ", "install"),
			modal.Btn(" Close ", "close"),
		))
	}

	return md
}
