_cc_switch_accounts() {
    local file
    for file in "$HOME"/.cc-accounts/*.json; do
        [[ -e "$file" ]] || continue
        file=${file##*/}
        printf '%s\n' "${file%.json}"
    done
}

_cc_switch_completion() {
    local cur command
    cur=${COMP_WORDS[COMP_CWORD]}
    command=${COMP_WORDS[1]}

    if (( COMP_CWORD == 1 )); then
        COMPREPLY=($(compgen -W 'save sync use remove rm rename mv list whoami status usage limit completion help' -- "$cur"))
        return
    fi

    case "$command" in
        use|remove|rm|usage|limit)
            COMPREPLY=($(compgen -W "$(_cc_switch_accounts)" -- "$cur"))
            ;;
        rename|mv)
            if (( COMP_CWORD == 2 )); then
                COMPREPLY=($(compgen -W "$(_cc_switch_accounts)" -- "$cur"))
            fi
            ;;
        completion)
            COMPREPLY=($(compgen -W 'bash fish zsh' -- "$cur"))
            ;;
    esac
}

complete -F _cc_switch_completion cc-switch
