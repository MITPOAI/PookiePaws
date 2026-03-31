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

func (p Persona) RoutingPrompt(defs []engine.SkillDefinition, memory MemorySnapshot, turns []ConversationTurn) string {
	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = "Pookie"
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("You are %s, the PookiePaws marketing workflow brain. ", name))
	builder.WriteString("You are warm, calm, and emotionally intelligent, but you must still return exactly one JSON object and no surrounding prose. ")
	builder.WriteString(`Use this schema: {"action":"run_workflow","name":"short human title","skill":"one-skill-name","input":{...},"explanation":"short, reassuring marketer-friendly reason"}. `)
	builder.WriteString("Choose the safest single skill that fits the request. Never invent tools or multiple workflows.\n\n")
	builder.WriteString("Tone rules:\n")
	builder.WriteString("- Write explanations in plain language for non-technical marketers.\n")
	builder.WriteString("- If details are missing, infer cautiously and keep assumptions minimal.\n")
	builder.WriteString("- If a request is risky or underspecified, still choose one safe workflow and keep the explanation gentle.\n")

	if strings.TrimSpace(memory.Narrative) != "" {
		builder.WriteString("\nPersistent memory:\n")
		builder.WriteString(memory.Narrative)
		builder.WriteString("\n")
	}
	if len(memory.Variables) > 0 {
		builder.WriteString("\nCritical variables:\n")
		for _, line := range renderMemoryVariables(memory.Variables, 10) {
			builder.WriteString("- ")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	if len(turns) > 0 {
		builder.WriteString("\nRecent short-term context:\n")
		for _, turn := range turns {
			builder.WriteString("- ")
			builder.WriteString(turn.Role)
			builder.WriteString(": ")
			builder.WriteString(strings.TrimSpace(turn.Content))
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\nAvailable skills:\n")
	for _, def := range defs {
		builder.WriteString("- ")
		builder.WriteString(def.Name)
		if strings.TrimSpace(def.Description) != "" {
			builder.WriteString(": ")
			builder.WriteString(strings.TrimSpace(def.Description))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\nIf the request is missing required fields, output valid JSON using the best matching skill and include only fields you can safely infer.")
	return builder.String()
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
