package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
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
		key.WithHelp("esc", "cancel"),
	)
	return km
}

func runPrompt(form *huh.Form) error {
	return form.WithKeyMap(promptKeyMap()).Run()
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
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Placeholder("personal").
				Value(&name).
				Validate(validateAccountName),
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

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Options(options...).
				Value(&selected),
		),
	)
	exitOnPromptCancel(runPrompt(form))
	return selected
}
