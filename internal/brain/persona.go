package brain

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type FriendlyError struct {
	Message   string
	Technical error
}

func (e FriendlyError) Error() string {
	return strings.TrimSpace(e.Message)
}

func (e FriendlyError) Unwrap() error {
	return e.Technical
}

type Persona struct {
	Name string
}

func DefaultPookiePersona() Persona {
	return Persona{Name: "Pookie"}
}

func (p Persona) RoutingPrompt(defs []engine.SkillDefinition, memory MemorySnapshot, turns []ConversationTurn, tools ...Tool) string {
	return NewPromptBuilder(PromptModeOperator).BuildOperatorPrompt(p.Name, defs, memory, turns, tools...)
}

func (p Persona) Humanize(err error) error {
	if err == nil {
		return nil
	}
	var friendly FriendlyError
	if errors.As(err, &friendly) {
		return err
	}

	message := "Something slipped while I was planning that workflow. Please try again, and if it keeps happening we should check the provider settings together."
	switch {
	case errors.Is(err, ErrProviderNotConfigured):
		message = "I'm ready to help, but I still need a model provider configured in Settings before I can plan this workflow."
	case strings.Contains(strings.ToLower(err.Error()), "timeout"), strings.Contains(strings.ToLower(err.Error()), "deadline exceeded"):
		message = "I ran out of time while planning that request. Please try again or shorten the prompt so I can route it more reliably."
	case strings.Contains(strings.ToLower(err.Error()), "unknown skill"),
		strings.Contains(strings.ToLower(err.Error()), "unsupported command"),
		strings.Contains(strings.ToLower(err.Error()), "invalid character"),
		strings.Contains(strings.ToLower(err.Error()), "cannot unmarshal"):
		message = "I couldn't confidently turn that into a safe workflow just yet. Please rephrase the goal in plain language and I'll try again."
	}
	return FriendlyError{
		Message:   message,
		Technical: err,
	}
}

func renderMemoryVariables(values map[string]string, limit int) []string {
	if limit <= 0 {
		limit = len(values)
	}
	lines := make([]string, 0, len(values))
	for key, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s = %s", key, value))
	}
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return lines
}
