package main

import (
	"fmt"
	"os"
)

func cmdHelp() {
	initStyles()
	fmt.Println(helpTitleStyle.Render("cc-switch — switch the active Claude Code account"))
	fmt.Println()
	fmt.Println(labelStyle.Render("Usage:"))
	helpLines := []struct {
		cmd  string
		desc string
	}{
		{"cc-switch save [name]", "snapshot the currently logged-in account"},
		{"cc-switch use [name]", "switch to a saved account"},
		{"cc-switch list", "list saved accounts with details"},
		{"cc-switch whoami", "show the active account"},
		{"cc-switch usage [name]", "show rate-limit usage (all accounts, or named ones)"},
		{"cc-switch help", "show this help"},
	}
	for _, line := range helpLines {
		fmt.Printf("  %s  %s\n", helpCmdStyle.Render(line.cmd), mutedStyle.Render(line.desc))
	}
	fmt.Println()
	fmt.Println(labelStyle.Render("First-time setup for multiple accounts:"))
	setupLines := []string{
		"claude auth login          # login with account A",
		"cc-switch save personal",
		"claude auth logout",
		"claude auth login          # login with account B",
		"cc-switch save work",
		"cc-switch use personal     # switch without another browser login",
		"cc-switch use work",
	}
	for _, line := range setupLines {
		fmt.Println("  " + mutedStyle.Render(line))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: cc-switch {save [name]|use [name]|list|whoami|usage [name...]|help}")
	fmt.Fprintln(os.Stderr, "Run 'cc-switch help' for more information.")
	os.Exit(1)
}

func arg(i int) string {
	if len(os.Args) > i {
		return os.Args[i]
	}
	return ""
}

func main() {
	initStyles()
	ensureSetup()

	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "save":
		cmdSave(arg(2))
	case "use":
		cmdUse(arg(2))
	case "list":
		cmdList()
	case "whoami":
		cmdWhoami()
	case "usage":
		cmdUsage(os.Args[2:])
	case "help", "-h", "--help":
		cmdHelp()
	default:
		usage()
	}
}
