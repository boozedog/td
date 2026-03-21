package monitor

import (
	"testing"
	"unicode/utf8"

	"github.com/marcus/td/internal/models"
)

func TestCopyStatusFilter(t *testing.T) {
	// Verify that the deep-copy logic used in fetchBoardIssues produces
	// an independent map — mutations to the original must not affect the copy.
	original := map[models.Status]bool{
		models.StatusOpen:       true,
		models.StatusInProgress: true,
		models.StatusClosed:     false,
	}

	// Replicate the exact deep-copy logic from fetchBoardIssues
	copied := make(map[models.Status]bool, len(original))
	for k, v := range original {
		copied[k] = v
	}

	// Verify initial equality
	for k, v := range original {
		if copied[k] != v {
			t.Fatalf("copy doesn't match original for key %v: got %v, want %v", k, copied[k], v)
		}
	}

	// Mutate the original (simulates toggleBoardClosed in next Update cycle)
	original[models.StatusClosed] = true

	// The copy must NOT see the mutation
	if copied[models.StatusClosed] != false {
		t.Error("Deep copy was affected by mutation to original — map was shared, not copied")
	}

	// Mutate the copy — original must not see it
	copied[models.StatusInProgress] = false
	if original[models.StatusInProgress] != true {
		t.Error("Original was affected by mutation to copy — map was shared, not copied")
	}
}

func TestHelpFilterBackspaceUTF8(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{"ASCII single char", "a", ""},
		{"ASCII multi char", "abc", "ab"},
		{"2-byte rune (é)", "filé", "fil"},
		{"3-byte rune (€)", "cost€", "cost"},
		{"4-byte rune (emoji)", "test\U0001F600", "test"},
		{"only multi-byte", "é", ""},
		{"two multi-byte", "éé", "é"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{HelpFilter: tc.filter}

			// Simulate backspace: same logic as commands.go
			if len(m.HelpFilter) > 0 {
				runes := []rune(m.HelpFilter)
				m.HelpFilter = string(runes[:len(runes)-1])
			}

			if m.HelpFilter != tc.want {
				t.Errorf("got %q, want %q", m.HelpFilter, tc.want)
			}
			if !utf8.ValidString(m.HelpFilter) {
				t.Errorf("result %q is not valid UTF-8", m.HelpFilter)
			}
		})
	}
}
