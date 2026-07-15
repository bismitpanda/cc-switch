package main

import (
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

type loaderDoneMsg struct{}

type loaderModel struct {
	spinner spinner.Model
	label   string
	done    <-chan struct{}
}

func newLoaderModel(label string, done <-chan struct{}) loaderModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	return loaderModel{spinner: s, label: label, done: done}
}

func waitForLoader(done <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-done
		return loaderDoneMsg{}
	}
}

func (m loaderModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForLoader(m.done))
}

func (m loaderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case loaderDoneMsg:
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

func (m loaderModel) View() string {
	return m.spinner.View() + " " + mutedStyle.Render(m.label)
}

func runWithLoader(label string, work func()) {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		work()
		return
	}

	initStyles()
	done := make(chan struct{})
	go func() {
		work()
		close(done)
	}()

	prog := tea.NewProgram(
		newLoaderModel(label, done),
		tea.WithOutput(os.Stderr),
	)
	if _, err := prog.Run(); err != nil {
		fatalf("%v", err)
	}
}
