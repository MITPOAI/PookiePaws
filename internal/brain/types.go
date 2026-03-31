package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type CompletionClient interface {
	Complete(ctx context.Context, request CompletionRequest) (CompletionResponse, error)
}

type CompletionRequest struct {
	SystemPrompt string
	UserPrompt   string
}

type CompletionResponse struct {
	Raw        string
	Model      string
	PromptText string
}

type Status struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	Mode     string `json:"mode"`
	Model    string `json:"model,omitempty"`
}

type Command struct {
	Action      string         `json:"action"`
	Name        string         `json:"name,omitempty"`
	Skill       string         `json:"skill,omitempty"`
	Input       map[string]any `json:"input,omitempty"`
	Explanation string         `json:"explanation,omitempty"`
}

type DispatchResult struct {
	Command  Command          `json:"command"`
	Workflow *engine.Workflow `json:"workflow,omitempty"`
	Model    string           `json:"model,omitempty"`
	Raw      string           `json:"raw,omitempty"`
}

func (c Command) Validate(skillNames []string) error {
	switch c.Action {
	case "run_workflow":
	default:
		return fmt.Errorf("unsupported command action %q", c.Action)
	}

	if strings.TrimSpace(c.Skill) == "" {
		return fmt.Errorf("command skill is required")
	}
	if c.Input == nil {
		return fmt.Errorf("command input is required")
	}

	for _, name := range skillNames {
		if name == c.Skill {
			return nil
		}
	}
	return fmt.Errorf("unknown skill %q", c.Skill)
}

func ParseCommand(raw string) (Command, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var command Command
	if err := json.Unmarshal([]byte(cleaned), &command); err != nil {
		return Command{}, err
	}
	return command, nil
}
