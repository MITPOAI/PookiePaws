package main

import "strings"

var topLevelCommands = []string{
	"start",
	"chat",
	"list",
	"research",
	"run",
	"status",
	"sessions",
	"approvals",
	"audit",
	"doctor",
	"smoke",
	"context",
	"memory",
	"install",
	"init",
	"completion",
	"version",
	"help",
}

var researchSubcommands = []string{
	"analyze",
	"watchlists",
	"refresh",
	"schedule",
	"status",
	"recommendations",
	"dossier",
	"help",
}

func suggestCommand(input string, choices []string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return ""
	}
	best := ""
	bestDistance := 1 << 30
	for _, choice := range choices {
		normalized := strings.ToLower(strings.TrimSpace(choice))
		if normalized == "" {
			continue
		}
		if normalized == input {
			return choice
		}
		if strings.HasPrefix(normalized, input) || strings.HasPrefix(input, normalized) {
			distance := abs(len(normalized) - len(input))
			if distance < bestDistance {
				bestDistance = distance
				best = choice
			}
			continue
		}
		distance := levenshteinDistance(input, normalized)
		if distance < bestDistance {
			bestDistance = distance
			best = choice
		}
	}
	if best == "" {
		return ""
	}
	threshold := 2
	if len(input) >= 8 {
		threshold = 3
	}
	if bestDistance > threshold {
		return ""
	}
	return best
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return len(b)
	}
	if b == "" {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = min3(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= a && b <= c {
		return b
	}
	return c
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
