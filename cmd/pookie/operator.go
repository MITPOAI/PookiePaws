package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/state"
)

func cmdSessions(args []string) {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	sessionID := fs.String("id", "", "show one session in detail")
	trace := fs.Bool("trace", false, "include prompt traces when showing one session")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		p.Error("initialise runtime: %v", err)
		os.Exit(1)
	}
	defer stack.Close()

	sessions, err := stack.store.ListSessions(context.Background())
	if err != nil {
		p.Error("list sessions: %v", err)
		os.Exit(1)
	}
	if *sessionID == "" {
		if len(sessions) == 0 {
			p.Warning("No sessions recorded yet")
			p.Blank()
			return
		}
		for _, session := range sessions {
			lastRun := ""
			if len(session.Runs) > 0 {
				lastRun = fmt.Sprintf("%s / %s", session.Runs[len(session.Runs)-1].Status, firstValue(session.Runs[len(session.Runs)-1].Skill, "-"))
			}
			p.Box("Session", [][2]string{
				{"id", session.ID},
				{"updated", session.UpdatedAt.Format(time.RFC3339)},
				{"messages", fmt.Sprintf("%d", len(session.Messages))},
				{"status", firstValue(string(session.LastStatus), "-")},
				{"last run", firstValue(lastRun, "-")},
			})
			p.Blank()
		}
		return
	}

	var selected *engine.Session
	for index := range sessions {
		if sessions[index].ID == *sessionID {
			selected = &sessions[index]
			break
		}
	}
	if selected == nil {
		p.Error("session %s not found", *sessionID)
		os.Exit(1)
	}

	p.Box("Session Detail", [][2]string{
		{"id", selected.ID},
		{"created", selected.CreatedAt.Format(time.RFC3339)},
		{"updated", selected.UpdatedAt.Format(time.RFC3339)},
		{"messages", fmt.Sprintf("%d", len(selected.Messages))},
		{"runs", fmt.Sprintf("%d", len(selected.Runs))},
		{"status", firstValue(string(selected.LastStatus), "-")},
	})
	p.Blank()
	for _, msg := range selected.Messages {
		p.Plain("[%s] %s: %s", msg.CreatedAt.Format("15:04:05"), msg.Role, msg.Content)
	}
	if len(selected.Messages) > 0 {
		p.Blank()
	}
	for _, run := range selected.Runs {
		p.Box("Run", [][2]string{
			{"id", run.ID},
			{"status", string(run.Status)},
			{"skill", firstValue(run.Skill, "-")},
			{"workflow", firstValue(run.WorkflowID, "-")},
			{"accepted", run.AcceptedAt.Format(time.RFC3339)},
			{"error", firstValue(run.Error, "-")},
			{"technical", firstValue(run.TechnicalError, "-")},
		})
		if *trace {
			printTrace(p, "Prompt Trace", run.Trace)
			printTrace(p, "Alternative Trace", run.AlternativeTrace)
		}
		p.Blank()
	}
}

func cmdApprovals(args []string) {
	fs := flag.NewFlagSet("approvals", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	approveID := fs.String("approve", "", "approve a pending approval id")
	rejectID := fs.String("reject", "", "reject a pending approval id")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		p.Error("initialise runtime: %v", err)
		os.Exit(1)
	}
	defer stack.Close()

	ctx := context.Background()
	switch {
	case *approveID != "":
		approval, err := stack.coord.Approve(ctx, *approveID)
		if err != nil {
			p.Error("approve %s: %v", *approveID, err)
			os.Exit(1)
		}
		p.Success("Approved %s for workflow %s", approval.ID, approval.WorkflowID)
		p.Blank()
		return
	case *rejectID != "":
		approval, err := stack.coord.Reject(ctx, *rejectID)
		if err != nil {
			p.Error("reject %s: %v", *rejectID, err)
			os.Exit(1)
		}
		p.Warning("Rejected %s for workflow %s", approval.ID, approval.WorkflowID)
		p.Blank()
		return
	}

	approvals, err := stack.coord.ListApprovals(ctx)
	if err != nil {
		p.Error("list approvals: %v", err)
		os.Exit(1)
	}
	pending := 0
	for _, approval := range approvals {
		if approval.State != engine.ApprovalPending {
			continue
		}
		pending++
		p.Box("Pending Approval", [][2]string{
			{"id", approval.ID},
			{"workflow", approval.WorkflowID},
			{"skill", approval.Skill},
			{"adapter", approval.Adapter},
			{"action", approval.Action},
			{"created", approval.CreatedAt.Format(time.RFC3339)},
		})
		p.Blank()
	}
	if pending == 0 {
		p.Success("No pending approvals")
		p.Blank()
	}
}

func cmdAudit(args []string) {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	lines := fs.Int("n", 20, "number of recent audit lines to show")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, _, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	entries, err := state.ReadRecentAuditEntries(filepath.Join(runtimeRoot, "state"), *lines)
	if err != nil {
		p.Error("read audit log: %v", err)
		os.Exit(1)
	}
	if len(entries) == 0 {
		p.Warning("No audit entries recorded yet")
		p.Blank()
		return
	}
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			p.Error("format audit entry: %v", err)
			os.Exit(1)
		}
		p.Plain("%s", data)
	}
	p.Blank()
}

func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	brainOnly := fs.Bool("brain", false, "validate the configured brain provider and model")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		p.Error("initialise runtime: %v", err)
		os.Exit(1)
	}
	defer stack.Close()

	brainHealth := checkStackBrainHealth(context.Background(), stack)
	if *brainOnly {
		printBrainHealth(p, brainHealth)
		if brainHealth.Healthy() {
			p.Success("Brain provider validation passed")
			p.Blank()
			return
		}
		printBrainRemediation(p, brainHealth)
		os.Exit(1)
	}

	ctx := context.Background()
	status, err := stack.coord.Status(ctx)
	if err != nil {
		p.Error("read status: %v", err)
		os.Exit(1)
	}
	channels, err := stack.coord.Channels(ctx)
	if err != nil {
		p.Error("read channels: %v", err)
		os.Exit(1)
	}

	p.Box("Runtime", [][2]string{
		{"runtime root", status.RuntimeRoot},
		{"workspace", status.WorkspaceRoot},
		{"workflows", fmt.Sprintf("%d", status.Workflows)},
		{"approvals", fmt.Sprintf("%d", status.PendingApprovals)},
		{"file permissions", fmt.Sprintf("%d", status.PendingFilePermissions)},
		{"brain", fmt.Sprintf("%t / %s / %s", stack.brainSvc.Available(), stack.brainSvc.Status().Provider, stack.brainSvc.Status().Mode)},
	})
	p.Blank()
	printBrainHealth(p, brainHealth)
	if !brainHealth.Healthy() {
		printBrainRemediation(p, brainHealth)
	}

	for _, warning := range startupWarnings(stack.secrets) {
		p.Warning("%s", warning)
	}
	if len(channels) == 0 {
		p.Warning("No channels registered")
		p.Blank()
		return
	}
	for _, channel := range channels {
		p.Box("Channel", [][2]string{
			{"channel", channel.Channel},
			{"provider", channel.Provider},
			{"configured", fmt.Sprintf("%t", channel.Configured)},
			{"healthy", fmt.Sprintf("%t", channel.Healthy)},
			{"detail", firstValue(channel.Message, "-")},
		})
		p.Blank()
	}
}

func printTrace(p *cli.Printer, title string, trace *engine.SessionPromptTrace) {
	if trace == nil {
		return
	}
	p.Box(title, [][2]string{
		{"mode", trace.Mode},
		{"model", firstValue(trace.Model, "-")},
		{"error", firstValue(trace.Error, "-")},
	})
	if text := strings.TrimSpace(trace.SystemPrompt); text != "" {
		p.Plain("system:")
		for _, line := range wrapLines(text, 88) {
			p.Plain("  %s", line)
		}
	}
	if text := strings.TrimSpace(trace.UserPrompt); text != "" {
		p.Plain("user:")
		for _, line := range wrapLines(text, 88) {
			p.Plain("  %s", line)
		}
	}
	if text := strings.TrimSpace(trace.RawResponse); text != "" {
		p.Plain("raw:")
		for _, line := range wrapLines(text, 88) {
			p.Plain("  %s", line)
		}
	}
}

func tailLines(path string, limit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0, limit)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if limit > 0 && len(lines) > limit {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}

func wrapLines(value string, max int) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if max <= 0 || len(value) <= max {
		return []string{value}
	}
	words := strings.Fields(value)
	lines := make([]string, 0, len(words)/2+1)
	current := ""
	for _, word := range words {
		next := word
		if current != "" {
			next = current + " " + word
		}
		if len(next) > max && current != "" {
			lines = append(lines, current)
			current = word
			continue
		}
		current = next
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func firstValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cmdContext(args []string) {
	fs := flag.NewFlagSet("context", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	showPrompt := fs.Bool("prompt", false, "render the full system routing prompt")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}
	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		p.Error("initialise runtime: %v", err)
		os.Exit(1)
	}
	defer stack.Close()

	ctx := context.Background()

	// Brain status.
	brainStatus := stack.brainSvc.Status()
	p.Box("Brain", [][2]string{
		{"enabled", fmt.Sprintf("%t", brainStatus.Enabled)},
		{"provider", firstValue(brainStatus.Provider, "-")},
		{"mode", firstValue(brainStatus.Mode, "-")},
		{"model", firstValue(brainStatus.Model, "-")},
	})
	p.Blank()

	// Conversation window.
	windowPath := filepath.Join(runtimeRoot, "state", "runtime", "conversation-window.json")
	if info, statErr := os.Stat(windowPath); statErr == nil {
		p.Box("Conversation Window", [][2]string{
			{"file", windowPath},
			{"size", fmt.Sprintf("%d bytes", info.Size())},
		})
	} else {
		p.Box("Conversation Window", [][2]string{
			{"file", windowPath},
			{"status", "empty"},
		})
	}
	p.Blank()

	// Persistent memory.
	memoryPath := brain.DetectPersistentMemoryPath(runtimeRoot)
	memInfo, memErr := os.Stat(memoryPath)
	memoryReader, readerErr := brain.NewPersistentMemory(runtimeRoot, nil, nil)
	if readerErr == nil {
		snapshot, snapshotErr := memoryReader.Snapshot(ctx)
		if snapshotErr == nil && (len(snapshot.Recent) > 0 || len(snapshot.Variables) > 0 || strings.TrimSpace(snapshot.Narrative) != "" || !snapshot.LastFlush.IsZero()) {
			fileSize := int64(0)
			if memErr == nil {
				fileSize = memInfo.Size()
			}
			p.Box("Persistent Memory", [][2]string{
				{"entries", fmt.Sprintf("%d", len(snapshot.Recent))},
				{"variables", fmt.Sprintf("%d", len(snapshot.Variables))},
				{"last flush", snapshot.LastFlush.Format(time.RFC3339)},
				{"file size", fmt.Sprintf("%d bytes", fileSize)},
			})
			p.Blank()
			if narrative := strings.TrimSpace(snapshot.Narrative); narrative != "" {
				p.Accent("Narrative:")
				for _, line := range wrapLines(narrative, 88) {
					p.Plain("  %s", line)
				}
				p.Blank()
			}
			if len(snapshot.Variables) > 0 {
				p.Accent("Variables:")
				for key, value := range snapshot.Variables {
					p.Plain("  %s = %s", key, value)
				}
				p.Blank()
			}
			if len(snapshot.Recent) > 0 {
				p.Accent("Recent entries:")
				for _, entry := range snapshot.Recent {
					p.Plain("  [%s] %s: %s", entry.Status, entry.Skill, truncate(entry.Summary, 72))
				}
				p.Blank()
			}
		} else {
			p.Box("Persistent Memory", [][2]string{{"status", "empty"}})
			p.Blank()
		}
	} else {
		p.Box("Persistent Memory", [][2]string{{"status", "empty"}})
		p.Blank()
	}

	// Skills list.
	defs := stack.coord.SkillDefinitions()
	p.Accent("Registered skills: %d", len(defs))
	for _, def := range defs {
		p.Plain("  %s: %s", def.Name, truncate(def.Description, 60))
	}
	p.Blank()

	// Render full routing prompt if requested.
	if *showPrompt {
		_ = ctx // keep ctx used
		prompt := stack.brainSvc.DebugRoutingPrompt()
		p.Accent("Full routing prompt (%d chars):", len(prompt))
		p.Rule("Prompt Start")
		fmt.Println(prompt)
		p.Rule("Prompt End")
		p.Blank()
	}
}

func cmdMemory(args []string) {
	fs := flag.NewFlagSet("memory", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	prune := fs.Bool("prune", false, "remove all persistent memory entries and variables")
	pruneWindow := fs.Bool("prune-window", false, "clear the conversation window only")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, _, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	if *prune {
		if pruneErr := brain.PrunePersistentMemory(runtimeRoot); pruneErr != nil {
			p.Error("write memory: %v", pruneErr)
			os.Exit(1)
		}
		p.Success("Persistent memory pruned (entries, variables, and narrative cleared)")
		p.Blank()
		return
	}

	if *pruneWindow {
		windowPath := filepath.Join(runtimeRoot, "state", "runtime", "conversation-window.json")
		if writeErr := os.WriteFile(windowPath, []byte("[]"), 0o644); writeErr != nil {
			p.Error("write window: %v", writeErr)
			os.Exit(1)
		}
		p.Success("Conversation window cleared")
		p.Blank()
		return
	}

	// Default: show memory summary (same as pookie context but just memory).
	memoryPath := brain.DetectPersistentMemoryPath(runtimeRoot)
	memoryReader, err := brain.NewPersistentMemory(runtimeRoot, nil, nil)
	if err != nil {
		p.Error("open memory: %v", err)
		os.Exit(1)
	}
	snapshot, readErr := memoryReader.Snapshot(context.Background())
	if readErr != nil || (len(snapshot.Recent) == 0 && len(snapshot.Variables) == 0 && strings.TrimSpace(snapshot.Narrative) == "" && snapshot.LastFlush.IsZero()) {
		p.Warning("No persistent memory recorded yet")
		p.Blank()
		return
	}
	p.Box("Persistent Memory", [][2]string{
		{"entries", fmt.Sprintf("%d / 16", len(snapshot.Recent))},
		{"variables", fmt.Sprintf("%d / 24", len(snapshot.Variables))},
		{"last flush", snapshot.LastFlush.Format(time.RFC3339)},
		{"file", memoryPath},
	})
	p.Blank()
	for _, entry := range snapshot.Recent {
		p.Plain("  [%s] %s %s: %s", entry.RecordedAt.Format("01-02 15:04"), entry.Status, entry.Skill, truncate(entry.Summary, 60))
	}
	if len(snapshot.Recent) > 0 {
		p.Blank()
	}
	if len(snapshot.Variables) > 0 {
		p.Accent("Variables:")
		for key, value := range snapshot.Variables {
			p.Plain("  %s = %s", key, value)
		}
		p.Blank()
	}
	p.Info("Use --prune to clear all memory, --prune-window to clear conversation window only")
	p.Blank()
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
