package skills

import (
	"fmt"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type Field struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

type RPCContract struct {
	Transport     string
	Method        string
	Notifications []string
}

type Manifest struct {
	Name           string
	Description    string
	Category       string
	Version        string
	Tags           []string
	Tools          []string
	Events         []engine.EventType
	Runtime        string
	Entrypoint     string
	ApprovalPolicy string
	Timeout        time.Duration
	RPC            RPCContract
	InputFields    []Field
	OutputFields   []Field
	Prompt         string
}

func ParseSkillMarkdown(content string) (Manifest, error) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		manifest := Manifest{}
		if err := parseSkillBody(trimmed, &manifest); err != nil {
			return Manifest{}, err
		}
		if manifest.Prompt == "" {
			manifest.Prompt = trimmed
		}
		return manifest, nil
	}

	parts := strings.SplitN(trimmed, "\n---", 2)
	if len(parts) != 2 {
		return Manifest{}, fmt.Errorf("invalid SKILL.md frontmatter")
	}

	header := strings.TrimPrefix(parts[0], "---")
	body := strings.TrimLeft(parts[1], "\r\n")
	manifest := Manifest{}
	var currentList string

	for _, rawLine := range strings.Split(header, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "- ") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			switch currentList {
			case "tags":
				manifest.Tags = append(manifest.Tags, value)
			case "tools":
				manifest.Tools = append(manifest.Tools, value)
			case "events":
				manifest.Events = append(manifest.Events, engine.EventType(value))
			case "rpc_notifications":
				manifest.RPC.Notifications = append(manifest.RPC.Notifications, value)
			}
			continue
		}

		currentList = ""
		key, value, found := strings.Cut(line, ":")
		if !found {
			return Manifest{}, fmt.Errorf("invalid frontmatter line %q", rawLine)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			manifest.Name = value
		case "description":
			manifest.Description = value
		case "category":
			manifest.Category = value
		case "version":
			manifest.Version = value
		case "runtime":
			manifest.Runtime = value
		case "entrypoint":
			manifest.Entrypoint = value
		case "approval_policy":
			manifest.ApprovalPolicy = value
		case "timeout":
			if value == "" {
				continue
			}
			duration, err := time.ParseDuration(value)
			if err != nil {
				return Manifest{}, fmt.Errorf("invalid timeout %q: %w", value, err)
			}
			manifest.Timeout = duration
		case "transport":
			manifest.RPC.Transport = value
		case "rpc_method":
			manifest.RPC.Method = value
		case "tags":
			currentList = "tags"
			if value != "" {
				manifest.Tags = append(manifest.Tags, splitInlineList(value)...)
			}
		case "tools":
			currentList = "tools"
			if value != "" {
				manifest.Tools = append(manifest.Tools, splitInlineList(value)...)
			}
		case "events":
			currentList = "events"
			for _, item := range splitInlineList(value) {
				manifest.Events = append(manifest.Events, engine.EventType(item))
			}
		case "rpc_notifications":
			currentList = "rpc_notifications"
			if value != "" {
				manifest.RPC.Notifications = append(manifest.RPC.Notifications, splitInlineList(value)...)
			}
		}
	}

	if err := parseSkillBody(body, &manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Prompt == "" {
		manifest.Prompt = strings.TrimSpace(body)
	}
	return manifest, nil
}

func parseSkillBody(body string, manifest *Manifest) error {
	currentSection := "prompt"
	promptLines := make([]string, 0)

	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)
		switch strings.ToLower(trimmed) {
		case "## inputs", "## input":
			currentSection = "inputs"
			continue
		case "## outputs", "## output":
			currentSection = "outputs"
			continue
		case "## rpc notifications", "## notifications":
			currentSection = "notifications"
			continue
		}

		switch currentSection {
		case "inputs":
			field, ok, err := parseFieldLine(trimmed)
			if err != nil {
				return err
			}
			if ok {
				manifest.InputFields = append(manifest.InputFields, field)
			}
		case "outputs":
			field, ok, err := parseFieldLine(trimmed)
			if err != nil {
				return err
			}
			if ok {
				manifest.OutputFields = append(manifest.OutputFields, field)
			}
		case "notifications":
			if strings.HasPrefix(trimmed, "- ") {
				manifest.RPC.Notifications = append(manifest.RPC.Notifications, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			}
		default:
			promptLines = append(promptLines, line)
		}
	}

	manifest.Prompt = strings.TrimSpace(strings.Join(promptLines, "\n"))
	return nil
}

func parseFieldLine(line string) (Field, bool, error) {
	if !strings.HasPrefix(line, "- ") {
		return Field{}, false, nil
	}

	raw := strings.TrimSpace(strings.TrimPrefix(line, "- "))
	namePart, description, _ := strings.Cut(raw, ":")
	namePart = strings.TrimSpace(namePart)
	description = strings.TrimSpace(description)

	field := Field{Description: description}
	if open := strings.Index(namePart, "("); open >= 0 && strings.HasSuffix(namePart, ")") {
		field.Name = strings.TrimSpace(namePart[:open])
		meta := strings.TrimSpace(namePart[open+1 : len(namePart)-1])
		for _, item := range splitInlineList(meta) {
			switch strings.ToLower(item) {
			case "required":
				field.Required = true
			case "optional":
			default:
				if field.Type == "" {
					field.Type = item
				}
			}
		}
	} else {
		field.Name = namePart
	}

	if field.Type == "" {
		field.Type = "string"
	}
	if field.Name == "" {
		return Field{}, false, fmt.Errorf("invalid field line %q", line)
	}
	return field, true, nil
}

func splitInlineList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
