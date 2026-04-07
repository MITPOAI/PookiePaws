// Package webfetch provides a lightweight HTTP fetcher that strips HTML into
// clean text. It is used by both the ResearcherSkill and the WebSearchTool.
package webfetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Result holds the cleaned output from a web page fetch.
type Result struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	RawText string `json:"raw_text"`
}

// Fetch retrieves a URL, strips HTML, and returns clean text.
// The response body is capped at 1 MB. RawText is capped at maxChars.
func Fetch(ctx context.Context, targetURL string, maxChars int) (Result, error) {
	if maxChars <= 0 {
		maxChars = 10000
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "PookiePaws/1.0 (marketing-research-bot)")

	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("fetch url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return Result{}, fmt.Errorf("fetch returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("read body: %w", err)
	}

	html := string(body)
	title := ExtractHTMLTitle(html)
	rawText := StripHTML(html)

	if len(rawText) > maxChars {
		rawText = rawText[:maxChars]
	}

	summary := rawText
	const summaryLen = 500
	if len(summary) > summaryLen {
		summary = summary[:summaryLen] + "..."
	}

	return Result{
		URL:     targetURL,
		Title:   title,
		Summary: summary,
		RawText: rawText,
	}, nil
}

// ExtractHTMLTitle extracts the content between <title> and </title> tags.
func ExtractHTMLTitle(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title")
	if start < 0 {
		return ""
	}
	close := strings.IndexByte(lower[start:], '>')
	if close < 0 {
		return ""
	}
	start += close + 1
	end := strings.Index(lower[start:], "</title>")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(html[start : start+end])
}

// StripHTML removes HTML tags and script/style blocks, returning plain text.
func StripHTML(html string) string {
	result := RemoveTagBlocks(html, "script")
	result = RemoveTagBlocks(result, "style")

	var buf strings.Builder
	buf.Grow(len(result))
	inTag := false
	for i := 0; i < len(result); i++ {
		switch {
		case result[i] == '<':
			inTag = true
		case result[i] == '>':
			inTag = false
		case !inTag:
			buf.WriteByte(result[i])
		}
	}
	text := buf.String()

	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

// RemoveTagBlocks strips everything between <tag...> and </tag> (case-insensitive).
func RemoveTagBlocks(html string, tag string) string {
	lower := strings.ToLower(html)
	openTag := "<" + tag
	closeTag := "</" + tag + ">"
	var buf strings.Builder
	buf.Grow(len(html))
	cursor := 0
	for {
		start := strings.Index(lower[cursor:], openTag)
		if start < 0 {
			buf.WriteString(html[cursor:])
			break
		}
		buf.WriteString(html[cursor : cursor+start])
		end := strings.Index(lower[cursor+start:], closeTag)
		if end < 0 {
			break
		}
		cursor = cursor + start + end + len(closeTag)
	}
	return buf.String()
}
