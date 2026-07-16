package main

import (
	"fmt"
	"os"
)

var version = "dev"

func formatHelpCmd(cmd, args string) string {
	out := helpCmdStyle.Render(cmd)
	if args != "" {
		out += " " + helpArgStyle.Render(args)
	}
	return out
}

func cmdHelp() {
	initStyles()
	fmt.Println(helpTitleStyle.Render(fmt.Sprintf("cc-switch (v. %s) — switch the active Claude Code account", version)))
	fmt.Println()
	fmt.Println(labelStyle.Render("Usage:"))
	helpLines := []struct {
		cmd  string
		args string
		desc string
	}{
		{"cc-switch save", "[name]", "snapshot the currently logged-in account"},
		{"cc-switch sync", "", "update the active account's snapshot from live credentials"},
		{"cc-switch use", "[name]", "switch to a saved account"},
		{"cc-switch remove", "[name]", "delete a saved account"},
		{"cc-switch rename", "[old] [new]", "rename a saved account"},
		{"cc-switch list", "", "list saved accounts with details"},
		{"cc-switch whoami", "", "show the active account"},
		{"cc-switch status", "", "show credential validity and expiry"},
		{"cc-switch usage", "[name]", "show rate-limit usage (all accounts, or a named one)"},
		{"cc-switch completion", "{bash|fish|zsh}", "print a shell completion script"},
		{"cc-switch help", "", "show this help"},
	}
	for _, line := range helpLines {
		fmt.Printf("  %s  %s\n", formatHelpCmd(line.cmd, line.args), mutedStyle.Render(line.desc))
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
	fmt.Fprintln(os.Stderr, "Usage: cc-switch {save [name]|sync|use [name]|remove [name]|rename [old] [new]|list|whoami|status|usage [name]|completion {bash|fish|zsh}|help}")
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
	case "sync":
		cmdSync()
	case "use":
		cmdUse(arg(2))
	case "remove", "rm":
		cmdRemove(arg(2))
	case "rename", "mv":
		cmdRename(arg(2), arg(3))
	case "list":
		cmdList()
	case "whoami":
		cmdWhoami()
	case "status":
		cmdStatus()
	case "usage", "limit":
		cmdUsage(arg(2))
	case "completion":
		cmdCompletion(arg(2))
	case "help", "-h", "--help":
		cmdHelp()
	default:
		usage()
	}
}
