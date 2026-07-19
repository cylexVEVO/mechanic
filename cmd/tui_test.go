package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTruncateTUI(t *testing.T) {
	tests := []struct {
		value string
		width int
		want  string
	}{
		{"short", 10, "short"},
		{"long command", 5, "long…"},
		{"héllo", 4, "hél…"},
	}
	for _, test := range tests {
		if got := truncateTUI(test.value, test.width); got != test.want {
			t.Errorf("truncateTUI(%q, %d) = %q, want %q", test.value, test.width, got, test.want)
		}
	}
}

func TestCreateFormEditingAndSubmit(t *testing.T) {
	form := newTUICreateForm()
	updateTUICreateForm(&form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("worker")})
	if form.name != "worker" {
		t.Fatalf("name = %q, want worker", form.name)
	}
	updateTUICreateForm(&form, tea.KeyMsg{Type: tea.KeyTab})
	updateTUICreateForm(&form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sleep")})
	updateTUICreateForm(&form, tea.KeyMsg{Type: tea.KeySpace})
	updateTUICreateForm(&form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("10")})
	if form.command != "sleep 10" {
		t.Fatalf("command = %q, want sleep 10", form.command)
	}
	submit, cancel := updateTUICreateForm(&form, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !submit || cancel {
		t.Fatalf("ctrl+s returned submit=%v cancel=%v", submit, cancel)
	}
}
