package main

import (
	"context"
	"os"
	"strings"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"
)

var version = "dev"

func completeAccountNames(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return listAccountNames(), cobra.ShellCompDirectiveNoFileComp
}

func completeRenameArgs(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) >= 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) == 1 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return listAccountNames(), cobra.ShellCompDirectiveNoFileComp
}

func optionalName(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "cc-switch",
		Short: "Switch the active Claude Code account",
		Long:  "cc-switch snapshots Claude Code OAuth credentials and restores them when you switch accounts.",
		Example: `
  # First-time setup for multiple accounts:
  claude auth login          # login with account A
  cc-switch save personal
  claude auth logout
  claude auth login          # login with account B
  cc-switch save work
  cc-switch use personal     # switch without another browser login
  cc-switch use work
`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			name := cmd.Name()
			if name == "completion" || name == "help" || name == "man" ||
				strings.HasPrefix(name, "__") {
				return
			}
			initStyles()
			ensureSetup()
		},
	}

	root.AddCommand(
		&cobra.Command{
			Use:   "save [name]",
			Short: "Snapshot the currently logged-in account",
			Args:  cobra.MaximumNArgs(1),
			Run: func(_ *cobra.Command, args []string) {
				cmdSave(optionalName(args))
			},
		},
		&cobra.Command{
			Use:   "sync",
			Short: "Update the active account's snapshot from live credentials",
			Args:  cobra.NoArgs,
			Run: func(_ *cobra.Command, _ []string) {
				cmdSync()
			},
		},
		&cobra.Command{
			Use:               "use [name]",
			Short:             "Switch to a saved account",
			Args:              cobra.MaximumNArgs(1),
			ValidArgsFunction: completeAccountNames,
			Run: func(_ *cobra.Command, args []string) {
				cmdUse(optionalName(args))
			},
		},
		&cobra.Command{
			Use:               "remove [name]",
			Aliases:           []string{"rm"},
			Short:             "Delete a saved account",
			Args:              cobra.MaximumNArgs(1),
			ValidArgsFunction: completeAccountNames,
			Run: func(_ *cobra.Command, args []string) {
				cmdRemove(optionalName(args))
			},
		},
		&cobra.Command{
			Use:               "rename [old] [new]",
			Aliases:           []string{"mv"},
			Short:             "Rename a saved account",
			Args:              cobra.MaximumNArgs(2),
			ValidArgsFunction: completeRenameArgs,
			Run: func(_ *cobra.Command, args []string) {
				oldName, newName := "", ""
				if len(args) > 0 {
					oldName = args[0]
				}
				if len(args) > 1 {
					newName = args[1]
				}
				cmdRename(oldName, newName)
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List saved accounts with details",
			Args:  cobra.NoArgs,
			Run: func(_ *cobra.Command, _ []string) {
				cmdList()
			},
		},
		&cobra.Command{
			Use:   "history",
			Short: "Show account switch history",
			Args:  cobra.NoArgs,
			Run: func(_ *cobra.Command, _ []string) {
				cmdHistory()
			},
		},
		&cobra.Command{
			Use:   "whoami",
			Short: "Show the active account",
			Args:  cobra.NoArgs,
			Run: func(_ *cobra.Command, _ []string) {
				cmdWhoami()
			},
		},
		newStatusCmd(),
		newUsageCmd(),
	)

	return root
}

func newStatusCmd() *cobra.Command {
	var activeOnly, expiredOnly bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show credential validity and expiry",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			cmdStatus(statusOptions{
				activeOnly:  activeOnly,
				expiredOnly: expiredOnly,
			})
		},
	}
	cmd.Flags().BoolVarP(&activeOnly, "active", "A", false, "Show only the active account")
	cmd.Flags().BoolVarP(&expiredOnly, "expired", "e", false, "Show only accounts with an expired token")
	return cmd
}

func newUsageCmd() *cobra.Command {
	var activeOnly, availableOnly, unavailableOnly, snapshotOnly bool
	cmd := &cobra.Command{
		Use:               "usage [name]",
		Aliases:           []string{"limit"},
		Short:             "Show rate-limit usage (all accounts, or a named one)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountNames,
		Run: func(_ *cobra.Command, args []string) {
			cmdUsage(optionalName(args), usageOptions{
				activeOnly:      activeOnly,
				availableOnly:   availableOnly,
				unavailableOnly: unavailableOnly,
				snapshotOnly:    snapshotOnly,
			})
		},
	}
	cmd.Flags().BoolVarP(&activeOnly, "active", "A", false, "Show only the active account")
	cmd.Flags().BoolVarP(&availableOnly, "available", "a", false, "Show only available accounts")
	cmd.Flags().BoolVarP(&unavailableOnly, "unavailable", "u", false, "Show only unavailable accounts")
	cmd.Flags().BoolVarP(&snapshotOnly, "snapshot-only", "s", false, "Use saved snapshots only (no live creds, writes, or token refresh)")
	cmd.MarkFlagsMutuallyExclusive("available", "unavailable")
	return cmd
}

func main() {
	if err := fang.Execute(
		context.Background(),
		newRootCmd(),
		fang.WithVersion(version),
		fang.WithNotifySignal(os.Interrupt),
	); err != nil {
		os.Exit(1)
	}
}
