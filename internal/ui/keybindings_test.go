package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestUpdate_DocumentedKeybindings(t *testing.T) {
	tests := []struct {
		name        string
		msg         tea.KeyMsg
		wantQuit    bool
		wantRefresh bool
	}{
		{
			name:     "q quits",
			msg:      tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")},
			wantQuit: true,
		},
		{
			name:     "ctrl+c quits",
			msg:      tea.KeyMsg{Type: tea.KeyCtrlC},
			wantQuit: true,
		},
		{
			name:        "r refreshes all clusters",
			msg:         tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")},
			wantRefresh: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel("a", "b")
			updated, cmd := m.Update(tt.msg)
			got := updated.(Model)

			if cmd == nil {
				t.Fatal("documented keybinding returned a nil command")
			}
			if got.Quitting != tt.wantQuit {
				t.Errorf("Quitting = %t, want %t", got.Quitting, tt.wantQuit)
			}

			if tt.wantQuit {
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Errorf("quit command returned %T, want tea.QuitMsg", cmd())
				}
				return
			}

			for i, cluster := range got.Clusters {
				if cluster.Fetching != tt.wantRefresh {
					t.Errorf("cluster %d Fetching = %t, want %t", i, cluster.Fetching, tt.wantRefresh)
				}
			}
		})
	}
}

func TestUpdate_DocumentedViewportKeysScroll(t *testing.T) {
	tests := []struct {
		name  string
		msg   tea.KeyMsg
		start int
		want  int
	}{
		{name: "up", msg: tea.KeyMsg{Type: tea.KeyUp}, start: 5, want: 4},
		{name: "down", msg: tea.KeyMsg{Type: tea.KeyDown}, start: 0, want: 1},
		{name: "page up", msg: tea.KeyMsg{Type: tea.KeyPgUp}, start: 5, want: 2},
		{name: "page down", msg: tea.KeyMsg{Type: tea.KeyPgDown}, start: 0, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel("a")
			m.Viewport.Width = 20
			m.Viewport.Height = 3
			m.Viewport.SetContent(strings.Repeat("line\n", 20))
			m.Viewport.SetYOffset(tt.start)

			updated, cmd := m.Update(tt.msg)
			got := updated.(Model)
			if cmd != nil {
				t.Errorf("viewport scroll returned an unexpected command: %v", cmd)
			}
			if got.Viewport.YOffset != tt.want {
				t.Errorf("viewport YOffset = %d, want %d", got.Viewport.YOffset, tt.want)
			}
		})
	}
}

func TestView_FooterMatchesShortHelpBindings(t *testing.T) {
	m := newTestModel("a")
	lines := strings.Split(strings.TrimSuffix(m.View(), "\n"), "\n")
	got := ansi.Strip(lines[len(lines)-1])
	want := "[q] quit  [r] refresh"
	if got != want {
		t.Fatalf("footer = %q, want %q", got, want)
	}

	for _, binding := range keys.ShortHelp() {
		help := binding.Help()
		wantBinding := fmt.Sprintf("[%s] %s", help.Key, help.Desc)
		if !strings.Contains(got, wantBinding) {
			t.Errorf("footer %q is missing short-help binding %q", got, wantBinding)
		}
	}
}
