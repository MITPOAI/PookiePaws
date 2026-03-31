package skills

import (
	"fmt"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

type Manifest struct {
	Name        string
	Description string
	Tools       []string
	Events      []engine.EventType
	Prompt      string
}

func ParseSkillMarkdown(content string) (Manifest, error) {
	if !strings.HasPrefix(content, "---") {
		return Manifest{Prompt: strings.TrimSpace(content)}, nil
	}

	parts := strings.SplitN(content, "\n---", 2)
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
			case "tools":
				manifest.Tools = append(manifest.Tools, value)
			case "events":
				manifest.Events = append(manifest.Events, engine.EventType(value))
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
		}
	}

	manifest.Prompt = strings.TrimSpace(body)
	return manifest, nil
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
		part = strings.Trim(strings.TrimSpace(part), `"`)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}
