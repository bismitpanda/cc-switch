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
| `cc-switch completion <shell>` | Print a completion script for Bash, Fish, or Zsh     |
| `cc-switch help`               | Show help                                            |

Omit `[name]` in an interactive terminal and you'll get a prompt.

## Shell completion

Choose your shell:

### Zsh

Create a completion directory and add it to `fpath`:

```bash
completion_dir="${ZDOTDIR:-$HOME}/.zfunc"
mkdir -p "$completion_dir"
cc-switch completion zsh > "$completion_dir/_cc-switch"
```

Add this before `compinit` in `${ZDOTDIR:-$HOME}/.zshrc`:

```zsh
fpath=("${ZDOTDIR:-$HOME}/.zfunc" $fpath)
autoload -Uz compinit
compinit
```

With Oh My Zsh, use its custom completion directory instead; it is already
added to `fpath` and is kept separate from framework updates:

```zsh
completion_dir="${ZSH_CUSTOM:-${ZSH:-$HOME/.oh-my-zsh}/custom}/completions"
mkdir -p "$completion_dir"
cc-switch completion zsh > "$completion_dir/_cc-switch"
```

Restart Zsh with `exec zsh`.

### Bash

Install
[bash-completion](https://github.com/scop/bash-completion) first, then write
the script to its per-user completion directory:

```bash
completion_dir="${BASH_COMPLETION_USER_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/bash-completion}/completions"
mkdir -p "$completion_dir"
cc-switch completion bash > "$completion_dir/cc-switch"
exec bash
```

### Fish

```fish
set -q XDG_CONFIG_HOME; or set XDG_CONFIG_HOME "$HOME/.config"
set completion_dir "$XDG_CONFIG_HOME/fish/completions"
mkdir -p "$completion_dir"
cc-switch completion fish > "$completion_dir/cc-switch.fish"
exec fish
```

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
