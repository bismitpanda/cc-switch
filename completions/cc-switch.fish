function __cc_switch_accounts
    for file in $HOME/.cc-accounts/*.json
        test -e "$file"; and basename "$file" .json
    end
end

function __cc_switch_needs_command
    test (count (commandline -opc)) -eq 1
end

function __cc_switch_using_command
    set -l words (commandline -opc)
    test (count $words) -gt 1; and contains -- $words[2] $argv
end

complete -c cc-switch -f
complete -c cc-switch -n __cc_switch_needs_command -a save -d 'Snapshot the currently logged-in account'
complete -c cc-switch -n __cc_switch_needs_command -a sync -d 'Update the active account snapshot'
complete -c cc-switch -n __cc_switch_needs_command -a use -d 'Switch to a saved account'
complete -c cc-switch -n __cc_switch_needs_command -a remove -d 'Delete a saved account'
complete -c cc-switch -n __cc_switch_needs_command -a rm -d 'Delete a saved account'
complete -c cc-switch -n __cc_switch_needs_command -a rename -d 'Rename a saved account'
complete -c cc-switch -n __cc_switch_needs_command -a mv -d 'Rename a saved account'
complete -c cc-switch -n __cc_switch_needs_command -a list -d 'List saved accounts'
complete -c cc-switch -n __cc_switch_needs_command -a whoami -d 'Show the active account'
complete -c cc-switch -n __cc_switch_needs_command -a usage -d 'Show rate-limit usage'
complete -c cc-switch -n __cc_switch_needs_command -a limit -d 'Show rate-limit usage'
complete -c cc-switch -n __cc_switch_needs_command -a completion -d 'Generate shell completion'
complete -c cc-switch -n __cc_switch_needs_command -a help -d 'Show help'

complete -c cc-switch -n '__cc_switch_using_command use remove rm usage limit' -a '(__cc_switch_accounts)'
complete -c cc-switch -n '__cc_switch_using_command rename mv' -a '(__cc_switch_accounts)'
complete -c cc-switch -n '__cc_switch_using_command completion' -a 'bash fish zsh'
