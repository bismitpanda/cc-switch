# cc-switch

CLI to switch between Claude Code accounts without logging in again.

Snapshots the active OAuth credentials, then restores them when you switch. Saved under `~/.cc-accounts/`.

## Install

```bash
go install github.com/bismitpanda/cc-switch@latest
```

Or from a local clone:

```bash
go build -ldflags "-X main.version=$(git rev-parse --short=7 HEAD)" -o cc-switch .
```

Requires [Go](https://go.dev/) and [Claude Code](https://docs.anthropic.com/en/docs/claude-code) (`claude`).

## Commands

| Command                        | Description                                          |
| ------------------------------ | ---------------------------------------------------- |
| `cc-switch save [name]`        | Snapshot the currently logged-in account             |
| `cc-switch sync`               | Update the active account's snapshot from live creds |
| `cc-switch use [name]`         | Switch to a saved account                            |
| `cc-switch remove [name]`      | Delete a saved account                               |
| `cc-switch rename [old] [new]` | Rename a saved account                               |
| `cc-switch list`               | List saved accounts                                  |
| `cc-switch whoami`             | Show the active account                              |
| `cc-switch usage [name]`       | Show rate-limit usage (all accounts, or a named one) |
| `cc-switch help`               | Show help                                            |

Omit `[name]` in an interactive terminal and you'll get a prompt.

## First-time setup

```bash
claude auth login          # account A
cc-switch save personal

claude auth logout
claude auth login          # account B
cc-switch save work

cc-switch use personal     # switch without another browser login
cc-switch use work
```

## How it works

Each save stores `oauthAccount` (from `~/.claude.json`) and `claudeAiOauth` (from `~/.claude/.credentials.json`, or `$CLAUDE_CONFIG_DIR/.credentials.json` if set) as `~/.cc-accounts/<name>.json`.

`sync` writes the current live credentials back into the active account's snapshot (errors if the active account isn't saved yet).

`use` syncs the outgoing account's snapshot first (when it matches a saved account), then writes the target snapshot into the active Claude Code config files.

Account snapshots are stored with mode `0600`; the store directory is `0700`.
