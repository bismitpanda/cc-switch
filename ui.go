package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	stylesReady bool

	titleStyle       lipgloss.Style
	successStyle     lipgloss.Style
	mutedStyle       lipgloss.Style
	errorStyle       lipgloss.Style
	accountStyle     lipgloss.Style
	labelStyle       lipgloss.Style
	barFillStyle     lipgloss.Style
	barEmptyStyle    lipgloss.Style
	criticalBarStyle lipgloss.Style
	activeBarStyle   lipgloss.Style
	whoamiKeyStyle   lipgloss.Style
	whoamiValStyle   lipgloss.Style
	helpTitleStyle   lipgloss.Style
	helpCmdStyle     lipgloss.Style
	helpArgStyle     lipgloss.Style
)

func initStyles() {
	if stylesReady {
		return
	}
	stylesReady = true

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	accountStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	barFillStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	barEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	criticalBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	activeBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	whoamiKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	whoamiValStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	helpCmdStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	helpArgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
}

func printMuted(msg string) {
	initStyles()
	fmt.Println(mutedStyle.Render(msg))
}

func printSuccess(msg string) {
	initStyles()
	fmt.Println(successStyle.Render(msg))
}

func printError(name, msg string) {
	initStyles()
	if name != "" {
		fmt.Fprintf(os.Stderr, "%s: %s\n", accountStyle.Render(name), errorStyle.Render(msg))
		return
	}
	fmt.Fprintln(os.Stderr, errorStyle.Render(msg))
}

var whoamiFields = []struct {
	key   string
	label string
}{
	{"displayName", "Name"},
	{"emailAddress", "Email"},
	{"organizationName", "Organization"},
	{"organizationType", "Type"},
}

func whoamiFieldValue(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		return s, s != ""
	default:
		return "", false
	}
}

func formatWhoamiField(key, value string) string {
	if key == "organizationType" || key == "billingType" {
		return cases.Title(language.English).String(strings.ReplaceAll(value, "_", " "))
	}
	return value
}

func printWhoami(oauth any) {
	initStyles()
	if oauth == nil {
		printMuted("(no active account)")
		return
	}
	m, ok := oauth.(map[string]any)
	if !ok || len(m) == 0 {
		printMuted("(no active account)")
		return
	}

	var lines []struct {
		label string
		value string
	}
	for _, field := range whoamiFields {
		value, ok := whoamiFieldValue(m[field.key])
		if !ok {
			continue
		}
		lines = append(lines, struct {
			label string
			value string
		}{field.label, formatWhoamiField(field.key, value)})
	}
	if len(lines) == 0 {
		printMuted("(no active account)")
		return
	}

	labelWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line.label + ":"); w > labelWidth {
			labelWidth = w
		}
	}

	fmt.Println(titleStyle.Render("Active account"))
	for _, line := range lines {
		fmt.Printf("  %s %s\n",
			whoamiKeyStyle.Width(labelWidth).Align(lipgloss.Right).Render(line.label+":"),
			whoamiValStyle.Render(line.value),
		)
	}
}

func cmdWhoami() {
	oauth, err := activeOAuthAccount()
	if err != nil {
		fatalf("%v", err)
	}
	printWhoami(oauth)
}
