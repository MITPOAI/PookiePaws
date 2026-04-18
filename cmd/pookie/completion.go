package main

import (
	"flag"
	"fmt"
	"os"
)

func cmdCompletion(args []string) {
	fs := flag.NewFlagSet("completion", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: pookie completion <bash|zsh|fish|powershell>")
	}
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	switch fs.Arg(0) {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	case "powershell", "pwsh":
		fmt.Print(powershellCompletion)
	default:
		fs.Usage()
		os.Exit(2)
	}
}

const bashCompletion = `# pookie bash completion
# Source this file or save to /etc/bash_completion.d/pookie
_pookie() {
    local cur prev cmd subcmd
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    cmd="${COMP_WORDS[1]}"

    if [ ${COMP_CWORD} -eq 1 ]; then
        local cmds="start status run install init chat list sessions approvals audit doctor smoke context memory version research completion help"
        COMPREPLY=( $(compgen -W "${cmds}" -- ${cur}) )
        return 0
    fi

    case "${cmd}" in
        research)
            if [ ${COMP_CWORD} -eq 2 ]; then
                COMPREPLY=( $(compgen -W "watchlists refresh schedule status recommendations dossier" -- ${cur}) )
            elif [ ${COMP_CWORD} -eq 3 ]; then
                local sub="${COMP_WORDS[2]}"
                case "${sub}" in
                    watchlists)     COMPREPLY=( $(compgen -W "list apply delete show" -- ${cur}) ) ;;
                    recommendations) COMPREPLY=( $(compgen -W "list show queue discard" -- ${cur}) ) ;;
                    dossier)        COMPREPLY=( $(compgen -W "list show diff evidence" -- ${cur}) ) ;;
                esac
            fi
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish powershell" -- ${cur}) )
            ;;
        schedule)
            if [ "${prev}" = "--mode" ]; then
                COMPREPLY=( $(compgen -W "manual hourly daily" -- ${cur}) )
            fi
            ;;
        *)
            COMPREPLY=( $(compgen -f -- ${cur}) )
            ;;
    esac
    return 0
}
complete -F _pookie pookie
`

const zshCompletion = `#compdef pookie
# pookie zsh completion
# Save as _pookie in a directory in your fpath

_pookie() {
    local -a cmds research_subs watchlists_subs recs_subs dossier_subs shells
    cmds=(
        'start:Run the daemon + web console'
        'status:Check whether the agent is running'
        'run:Execute a skill directly'
        'install:Install a skill from GitHub'
        'init:First-run setup wizard'
        'chat:Talk to Pookie in your terminal'
        'list:Show all installed skills'
        'sessions:Inspect persisted sessions'
        'approvals:Review or resolve approvals'
        'audit:Tail audit events'
        'doctor:Print runtime diagnostics'
        'smoke:Run operator smoke checks'
        'context:Inspect prompt and memory'
        'memory:Manage brain memory'
        'version:Print version'
        'research:Research watchlists, scheduler, dossiers'
        'completion:Output shell completion script'
        'help:Show help'
    )
    research_subs=(watchlists refresh schedule status recommendations dossier)
    watchlists_subs=(list apply delete show)
    recs_subs=(list show queue discard)
    dossier_subs=(list show diff evidence)
    shells=(bash zsh fish powershell)

    if (( CURRENT == 2 )); then
        _describe 'command' cmds
    elif (( CURRENT == 3 )); then
        case "${words[2]}" in
            research)   _values 'subcommand' $research_subs ;;
            completion) _values 'shell' $shells ;;
        esac
    elif (( CURRENT == 4 )); then
        if [[ "${words[2]}" == "research" ]]; then
            case "${words[3]}" in
                watchlists)      _values 'subcommand' $watchlists_subs ;;
                recommendations) _values 'subcommand' $recs_subs ;;
                dossier)         _values 'subcommand' $dossier_subs ;;
            esac
        fi
    else
        _files
    fi
}
compdef _pookie pookie
`

const fishCompletion = `# pookie fish completion
# Save to ~/.config/fish/completions/pookie.fish

complete -c pookie -f

# Top-level commands
complete -c pookie -n '__fish_use_subcommand' -a 'start' -d 'Run daemon + web console'
complete -c pookie -n '__fish_use_subcommand' -a 'status' -d 'Check daemon'
complete -c pookie -n '__fish_use_subcommand' -a 'run' -d 'Execute a skill'
complete -c pookie -n '__fish_use_subcommand' -a 'install' -d 'Install a skill from GitHub'
complete -c pookie -n '__fish_use_subcommand' -a 'init' -d 'Setup wizard'
complete -c pookie -n '__fish_use_subcommand' -a 'chat' -d 'Terminal chat'
complete -c pookie -n '__fish_use_subcommand' -a 'list' -d 'List installed skills'
complete -c pookie -n '__fish_use_subcommand' -a 'sessions' -d 'Inspect sessions'
complete -c pookie -n '__fish_use_subcommand' -a 'approvals' -d 'Review approvals'
complete -c pookie -n '__fish_use_subcommand' -a 'audit' -d 'Tail audit events'
complete -c pookie -n '__fish_use_subcommand' -a 'doctor' -d 'Diagnostics'
complete -c pookie -n '__fish_use_subcommand' -a 'smoke' -d 'Smoke checks'
complete -c pookie -n '__fish_use_subcommand' -a 'context' -d 'Inspect prompt and memory'
complete -c pookie -n '__fish_use_subcommand' -a 'memory' -d 'Brain memory'
complete -c pookie -n '__fish_use_subcommand' -a 'version' -d 'Print version'
complete -c pookie -n '__fish_use_subcommand' -a 'research' -d 'Research subcommands'
complete -c pookie -n '__fish_use_subcommand' -a 'completion' -d 'Shell completion'
complete -c pookie -n '__fish_use_subcommand' -a 'help' -d 'Show help'

# research subcommands
complete -c pookie -n '__fish_seen_subcommand_from research; and not __fish_seen_subcommand_from watchlists refresh schedule status recommendations dossier' -a 'watchlists refresh schedule status recommendations dossier'
complete -c pookie -n '__fish_seen_subcommand_from watchlists' -a 'list apply delete show'
complete -c pookie -n '__fish_seen_subcommand_from recommendations' -a 'list show queue discard'
complete -c pookie -n '__fish_seen_subcommand_from dossier' -a 'list show diff evidence'

# completion shells
complete -c pookie -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish powershell'

# schedule modes
complete -c pookie -n '__fish_seen_subcommand_from schedule' -l mode -a 'manual hourly daily'
`

const powershellCompletion = `# pookie PowerShell completion
# Add to your $PROFILE: . path\to\pookie-completion.ps1

Register-ArgumentCompleter -Native -CommandName pookie -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @('start','status','run','install','init','chat','list','sessions','approvals','audit','doctor','smoke','context','memory','version','research','completion','help')
    $researchSubs = @('watchlists','refresh','schedule','status','recommendations','dossier')
    $watchlistsSubs = @('list','apply','delete','show')
    $recSubs = @('list','show','queue','discard')
    $dossierSubs = @('list','show','diff','evidence')
    $shells = @('bash','zsh','fish','powershell')

    $tokens = $commandAst.CommandElements | Where-Object { $_.GetType().Name -eq 'StringConstantExpressionAst' } | ForEach-Object { $_.Value }
    $count = $tokens.Count

    if ($count -le 1) {
        $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    switch ($tokens[1]) {
        'research' {
            if ($count -le 2) {
                $researchSubs | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            } elseif ($count -le 3) {
                $sub = switch ($tokens[2]) {
                    'watchlists' { $watchlistsSubs }
                    'recommendations' { $recSubs }
                    'dossier' { $dossierSubs }
                    default { @() }
                }
                $sub | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                    [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
                }
            }
        }
        'completion' {
            $shells | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
    }
}
`
