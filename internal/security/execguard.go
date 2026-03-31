package security

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type commandRule func(args []string) error

type CommandExecGuard struct {
	rules map[string]commandRule
}

var _ engine.ExecGuard = (*CommandExecGuard)(nil)

func NewCommandExecGuard() *CommandExecGuard {
	return &CommandExecGuard{
		rules: map[string]commandRule{
			"cat":     allowPathArgumentsOnly,
			"findstr": allowPathArgumentsOnly,
			"git":     validateGitReadOnly,
			"go":      validateGoReadOnly,
			"rg":      allowPathArgumentsOnly,
			"type":    allowPathArgumentsOnly,
			"where":   allowPathArgumentsOnly,
			"whoami":  denyControlOperators,
		},
	}
}

func (g *CommandExecGuard) Validate(command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("command is required")
	}

	name := normalizeCommandName(command[0])
	rule, ok := g.rules[name]
	if !ok {
		return fmt.Errorf("command %q is not allowlisted", name)
	}
	if err := denyControlOperators(command[1:]); err != nil {
		return err
	}
	return rule(command[1:])
}

func normalizeCommandName(name string) string {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(name)))
	for _, suffix := range []string{".exe", ".cmd", ".bat", ".com"} {
		base = strings.TrimSuffix(base, suffix)
	}
	return base
}

func validateGitReadOnly(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("git subcommand is required")
	}
	switch strings.ToLower(args[0]) {
	case "status", "diff", "log", "show", "rev-parse", "branch", "ls-files", "grep":
		return denyControlOperators(args[1:])
	default:
		return fmt.Errorf("git subcommand %q is not allowlisted", args[0])
	}
}

func validateGoReadOnly(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("go subcommand is required")
	}
	switch strings.ToLower(args[0]) {
	case "env", "list", "test", "version":
		return denyControlOperators(args[1:])
	default:
		return fmt.Errorf("go subcommand %q is not allowlisted", args[0])
	}
}

func allowPathArgumentsOnly(args []string) error {
	for _, arg := range args {
		if isUnsafeArgument(arg) {
			return fmt.Errorf("argument %q is not allowed", arg)
		}
	}
	return nil
}

func denyControlOperators(args []string) error {
	for _, arg := range args {
		if isUnsafeArgument(arg) {
			return fmt.Errorf("argument %q is not allowed", arg)
		}
	}
	return nil
}

func isUnsafeArgument(arg string) bool {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return false
	}
	unsafeFragments := []string{
		";",
		"&&",
		"||",
		"|",
		">",
		"<",
		"`",
		"$(",
		"%(",
	}
	for _, fragment := range unsafeFragments {
		if strings.Contains(arg, fragment) {
			return true
		}
	}
	return false
}
