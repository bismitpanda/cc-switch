package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func promptKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(
		key.WithKeys("esc", "ctrl+c"),
		key.WithHelp("esc/ctrl+c", "cancel"),
	)
	return km
}

type fieldWithCancelHelp struct {
	huh.Field
	cancel key.Binding
}

func withCancelHelp(field huh.Field, cancel key.Binding) huh.Field {
	return &fieldWithCancelHelp{Field: field, cancel: cancel}
}

func (f *fieldWithCancelHelp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := f.Field.Update(msg)
	f.Field = m.(huh.Field)
	return f, cmd
}

func (f *fieldWithCancelHelp) KeyBinds() []key.Binding {
	return append(f.Field.KeyBinds(), f.cancel)
}

func (f *fieldWithCancelHelp) WithKeyMap(k *huh.KeyMap) huh.Field {
	f.Field = f.Field.WithKeyMap(k)
	f.cancel = k.Quit
	return f
}

func (f *fieldWithCancelHelp) WithTheme(t *huh.Theme) huh.Field {
	f.Field = f.Field.WithTheme(t)
	return f
}

func (f *fieldWithCancelHelp) WithWidth(width int) huh.Field {
	f.Field = f.Field.WithWidth(width)
	return f
}

func (f *fieldWithCancelHelp) WithHeight(height int) huh.Field {
	f.Field = f.Field.WithHeight(height)
	return f
}

func (f *fieldWithCancelHelp) WithPosition(p huh.FieldPosition) huh.Field {
	f.Field = f.Field.WithPosition(p)
	return f
}

func runPrompt(form *huh.Form) error {
	form = form.WithKeyMap(promptKeyMap())

	form.SubmitCmd = tea.Quit
	form.CancelCmd = tea.Quit

	m, err := tea.NewProgram(form,
		tea.WithOutput(os.Stderr),
		tea.WithReportFocus(),
	).Run()
	if m != nil {
		if f, ok := m.(*huh.Form); ok && f.State == huh.StateAborted {
			return huh.ErrUserAborted
		}
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return huh.ErrUserAborted
	}
	if err != nil {
		return err
	}
	return nil
}

func exitOnPromptCancel(err error) {
	if errors.Is(err, huh.ErrUserAborted) {
		os.Exit(0)
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func validateAccountName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("name cannot contain path separators")
	}
	return nil
}

func promptSaveName() string {
	var name string
	km := promptKeyMap()
	form := huh.NewForm(
		huh.NewGroup(
			withCancelHelp(
				huh.NewInput().
					Placeholder("personal").
					Value(&name).
					Validate(validateAccountName),
				km.Quit,
			),
		),
	)
	exitOnPromptCancel(runPrompt(form))
	return strings.TrimSpace(name)
}

func promptUseAccount() string {
	names := listAccountNames()
	if len(names) == 0 {
		fatalf("No saved accounts yet. Run: cc-switch save <name>")
	}

	var selected string
	options := make([]huh.Option[string], len(names))
	for i, name := range names {
		options[i] = huh.NewOption(name, name)
	}

	km := promptKeyMap()
	form := huh.NewForm(
		huh.NewGroup(
			withCancelHelp(
				huh.NewSelect[string]().
					Options(options...).
					Value(&selected),
				km.Quit,
			),
		),
	)
	exitOnPromptCancel(runPrompt(form))
	return selected
}
