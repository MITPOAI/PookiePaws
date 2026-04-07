package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

// cmdChat launches an interactive REPL that routes natural-language prompts
// through the PookiePaws brain service. Each prompt may produce a workflow
// submission, a safety intervention, or a friendly error.
func cmdChat(args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
	sessionID := fs.String("session", "", "resume an existing session id")
	_ = fs.Parse(args)

	p := cli.Stdout()
	p.Banner()

	runtimeRoot, workspaceRoot, err := resolveRoots(*home)
	if err != nil {
		p.Error("resolve runtime: %v", err)
		os.Exit(1)
	}

	spin := p.NewSpinner("Initialising engine…")
	spin.Start()
	stack, err := buildStack(runtimeRoot, workspaceRoot)
	if err != nil {
		spin.Stop(false, "Engine initialisation failed")
		p.Error("%v", err)
		os.Exit(1)
	}
	defer stack.Close()
	spin.Stop(true, "Engine ready")

	if !stack.brainSvc.Available() {
		p.Error("No LLM provider configured")
		p.Blank()
		p.Dim("Run  pookie init  to configure a model provider, then try again.")
		p.Blank()
		os.Exit(1)
	}

	ctx := context.Background()
	health := checkStackBrainHealth(ctx, stack)
	if !health.Healthy() {
		p.Error("Brain provider validation failed")
		p.Blank()
		printBrainHealth(p, health)
		printBrainRemediation(p, health)
		os.Exit(1)
	}

	p.Blank()
	p.Info("Chat with Pookie - your marketing co-pilot")
	brainStatus := stack.brainSvc.Status()
	if brainStatus.Model != "" {
		p.Dim("Connected to %s via %s", brainStatus.Model, brainStatus.Provider)
	}
	p.Dim("Type a marketing goal in plain English. Pookie will pick the best skill.")
	p.Dim("Commands:  /skills  /sessions  /approvals  /doctor  /clear  /exit")
	p.Blank()

	session, err := loadOrCreateChatSession(ctx, stack.store, *sessionID)
	if err != nil {
		p.Error("session setup: %v", err)
		os.Exit(1)
	}
	p.Dim("Session: %s", session.ID)
	p.Blank()

	for {
		line, ok := cli.ReadLine(p, "Pookie > ")
		if !ok {
			// Ctrl+C or EOF — exit gracefully.
			p.Blank()
			p.Dim("Goodbye! — Pookie")
			p.Blank()
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		switch strings.ToLower(line) {
		case "/exit", "/quit":
			p.Blank()
			p.Dim("Goodbye! — Pookie")
			p.Blank()
			return

		case "/skills":
			printSkillList(p, stack.coord.SkillDefinitions())
			continue

		case "/sessions":
			sessions, err := stack.store.ListSessions(ctx)
			if err != nil {
				p.Error("list sessions: %v", err)
				p.Blank()
				continue
			}
			if len(sessions) == 0 {
				p.Warning("No persisted sessions yet")
				p.Blank()
				continue
			}
			for _, session := range sessions {
				p.Box("Session", [][2]string{
					{"id", session.ID},
					{"updated", session.UpdatedAt.Format(time.RFC3339)},
					{"messages", fmt.Sprintf("%d", len(session.Messages))},
					{"status", firstChatValue(string(session.LastStatus), "-")},
				})
				p.Blank()
			}
			continue

		case "/approvals":
			approvals, err := stack.coord.ListApprovals(ctx)
			if err != nil {
				p.Error("list approvals: %v", err)
				p.Blank()
				continue
			}
			found := false
			for _, approval := range approvals {
				if approval.State != engine.ApprovalPending {
					continue
				}
				found = true
				p.Box("Pending Approval", [][2]string{
					{"id", approval.ID},
					{"workflow", approval.WorkflowID},
					{"adapter", approval.Adapter},
					{"action", approval.Action},
				})
				p.Blank()
			}
			if !found {
				p.Success("No pending approvals")
				p.Blank()
			}
			continue

		case "/doctor":
			status, err := stack.coord.Status(ctx)
			if err != nil {
				p.Error("status: %v", err)
				p.Blank()
				continue
			}
			health := checkStackBrainHealth(ctx, stack)
			p.Box("Runtime", [][2]string{
				{"workflows", fmt.Sprintf("%d", status.Workflows)},
				{"approvals", fmt.Sprintf("%d", status.PendingApprovals)},
				{"file permissions", fmt.Sprintf("%d", status.PendingFilePermissions)},
				{"brain", fmt.Sprintf("%t / %s", stack.brainSvc.Available(), stack.brainSvc.Status().Provider)},
			})
			p.Blank()
			printBrainHealth(p, health)
			continue

		case "/clear":
			fmt.Fprint(os.Stdout, "\033[2J\033[H")
			p.Banner()
			continue

		case "/help":
			p.Blank()
			p.Plain("  /skills   List available marketing skills")
			p.Plain("  /sessions Show persisted control-plane sessions")
			p.Plain("  /approvals List pending approvals")
			p.Plain("  /doctor   Show runtime diagnostics")
			p.Plain("  /clear    Clear the screen")
			p.Plain("  /exit     Leave the chat")
			p.Blank()
			continue
		}

		// Dispatch the prompt through the brain service.
		spin := p.NewSpinner("Thinking…")
		spin.Start()

		run, sessionState, err := beginChatRun(ctx, stack.store, session.ID, line)
		if err != nil {
			spin.Stop(false, "")
			p.Error("%v", err)
			p.Blank()
			continue
		}

		result, err := stack.brainSvc.DispatchPrompt(ctx, line)
		if err != nil {
			_ = finishChatRunFailure(ctx, stack.store, stack.brainSvc, &sessionState, run.ID, line, err)
			spin.Stop(false, "")
			p.Error("%v", err)
			technical := technicalDispatchError(err)
			if technical != "" && technical != err.Error() {
				p.Dim("technical: %s", technical)
			}
			if strings.Contains(strings.ToLower(technical), "model") {
				p.Dim("%s", brainHealthRemediation(brain.ProviderHealth{
					Provider:    brainStatus.Provider,
					Model:       brainStatus.Model,
					FailureCode: brain.ProviderFailureModel,
				}))
			}
			p.Blank()
			continue
		}
		spin.Stop(true, "")
		session = finishChatRunSuccess(ctx, stack.store, sessionState, run.ID, result)

		// Casual chat — display the conversational response.
		if result.Command.Action == "casual_chat" {
			p.Blank()
			printWrapped(p, result.Command.Explanation)
			p.Blank()
			continue
		}

		// Chained pipeline — show steps and final output.
		if result.Command.Action == "run_chain" {
			p.Blank()
			if result.Command.Explanation != "" {
				printWrapped(p, result.Command.Explanation)
				p.Blank()
			}
			if result.Blocked != nil {
				p.Warning("Chain halted: %s", result.Blocked.Reason)
				p.Blank()
			}
			if result.Workflow != nil {
				printChatWorkflow(p, result.Workflow, "")
				if path, ok := result.Workflow.Output["path"].(string); ok {
					p.Success("Exported to: %s", path)
					p.Blank()
				}
			}
			continue
		}

		// Safety intervention — the request was blocked.
		if result.Blocked != nil {
			p.Warning("Blocked: %s", result.Blocked.Reason)
			if result.Alternative != nil && result.Alternative.Message != "" {
				p.Blank()
				printWrapped(p, result.Alternative.Message)
			}
			p.Blank()
			continue
		}

		// Workflow was submitted successfully.
		if result.Workflow != nil {
			printChatWorkflow(p, result.Workflow, result.Command.Explanation)
			continue
		}

		// Fallback — raw model output (shouldn't happen often).
		if result.Raw != "" {
			p.Blank()
			printWrapped(p, result.Raw)
			p.Blank()
		}
	}
}

// printChatWorkflow shows a concise workflow summary inside the chat REPL.
func printChatWorkflow(p *cli.Printer, wf *engine.Workflow, explanation string) {
	p.Blank()
	if explanation != "" {
		printWrapped(p, explanation)
		p.Blank()
	}
	rows := [][2]string{
		{"skill", wf.Skill},
		{"status", string(wf.Status)},
		{"workflow", wf.ID},
	}
	p.Box("Workflow Submitted", rows)
	if wf.Status == engine.WorkflowWaitingApproval {
		p.Info("This workflow needs approval — open the web console or use  pookie status")
	}
	p.Blank()
}

// printSkillList shows available skills inline within the chat REPL.
func printSkillList(p *cli.Printer, defs []engine.SkillDefinition) {
	p.Blank()
	for _, d := range defs {
		desc := d.Description
		if desc == "" {
			desc = "—"
		}
		p.Plain("  %-30s  %s", d.Name, desc)
	}
	p.Blank()
}

// printWrapped outputs text with basic word-wrapping at 76 columns.
func printWrapped(p *cli.Printer, text string) {
	const maxWidth = 76
	for _, paragraph := range strings.Split(text, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			p.Blank()
			continue
		}
		words := strings.Fields(paragraph)
		line := "  "
		for _, w := range words {
			if len(line)+len(w)+1 > maxWidth && len(line) > 2 {
				fmt.Fprintln(os.Stdout, line)
				line = "  "
			}
			if len(line) > 2 {
				line += " "
			}
			line += w
		}
		if len(line) > 2 {
			fmt.Fprintln(os.Stdout, line)
		}
	}
}

func firstChatValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func loadOrCreateChatSession(ctx context.Context, store engine.StateStore, sessionID string) (engine.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		return store.GetSession(ctx, sessionID)
	}
	now := time.Now().UTC()
	session := engine.Session{
		SessionSummary: engine.SessionSummary{
			ID:        localChatID("chat"),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages: []engine.SessionMessage{},
		Runs:     []engine.SessionRun{},
	}
	return session, store.SaveSession(ctx, session)
}

func beginChatRun(ctx context.Context, store engine.StateStore, sessionID string, prompt string) (engine.SessionRun, engine.Session, error) {
	session, err := store.GetSession(ctx, sessionID)
	if err != nil {
		return engine.SessionRun{}, engine.Session{}, err
	}
	now := time.Now().UTC()
	session.Messages = append(session.Messages, engine.SessionMessage{
		ID:        localChatID("msg"),
		SessionID: session.ID,
		Role:      "user",
		Kind:      "prompt",
		Content:   prompt,
		CreatedAt: now,
	})
	run := engine.SessionRun{
		ID:         localChatID("run"),
		SessionID:  session.ID,
		Prompt:     prompt,
		Status:     engine.SessionRunning,
		AcceptedAt: now,
		StartedAt:  now,
	}
	session.Runs = append(session.Runs, run)
	session.UpdatedAt = now
	session.MessageCount = len(session.Messages)
	session.LastStatus = engine.SessionRunning
	return run, session, store.SaveSession(ctx, session)
}

func finishChatRunFailure(ctx context.Context, store engine.StateStore, debugPrompt interface{ DebugRoutingPrompt() string }, session *engine.Session, runID string, prompt string, dispatchErr error) error {
	now := time.Now().UTC()
	session.Messages = append(session.Messages, engine.SessionMessage{
		ID:        localChatID("msg"),
		SessionID: session.ID,
		Role:      "assistant",
		Kind:      "error",
		Content:   dispatchErr.Error(),
		Status:    "failed",
		CreatedAt: now,
	})
	technical := technicalDispatchError(dispatchErr)
	trace := &engine.SessionPromptTrace{
		Mode:       string(brain.PromptModeOperator),
		UserPrompt: strings.TrimSpace(prompt),
		Error:      technical,
		CreatedAt:  now,
	}
	if debugPrompt != nil {
		trace.SystemPrompt = strings.TrimSpace(debugPrompt.DebugRoutingPrompt())
	}
	for index := range session.Runs {
		if session.Runs[index].ID != runID {
			continue
		}
		session.Runs[index].Status = engine.SessionFailed
		session.Runs[index].Error = dispatchErr.Error()
		session.Runs[index].TechnicalError = technical
		session.Runs[index].Trace = trace
		session.Runs[index].FinishedAt = now
	}
	session.UpdatedAt = now
	session.MessageCount = len(session.Messages)
	session.LastStatus = engine.SessionFailed
	return store.SaveSession(ctx, *session)
}

func finishChatRunSuccess(ctx context.Context, store engine.StateStore, session engine.Session, runID string, result brain.DispatchResult) engine.Session {
	now := time.Now().UTC()
	session.Messages = append(session.Messages, buildCLIChatAssistantMessage(session.ID, result))

	status := engine.SessionCompleted
	workflowID := ""
	if result.Workflow != nil {
		workflowID = result.Workflow.ID
		if result.Workflow.Status == engine.WorkflowWaitingApproval {
			status = engine.SessionAwaitingApproval
		}
	}
	if result.Blocked != nil {
		status = engine.SessionBlocked
	}

	for index := range session.Runs {
		if session.Runs[index].ID != runID {
			continue
		}
		session.Runs[index].Status = status
		session.Runs[index].WorkflowID = workflowID
		session.Runs[index].Skill = result.Command.Skill
		session.Runs[index].Error = ""
		session.Runs[index].TechnicalError = ""
		session.Runs[index].FinishedAt = now
		session.Runs[index].Trace = translateCLITrace(result.PromptTrace)
		session.Runs[index].AlternativeTrace = translateCLITrace(result.AltTrace)
	}
	session.UpdatedAt = now
	session.MessageCount = len(session.Messages)
	session.LastStatus = status
	_ = store.SaveSession(ctx, session)
	return session
}

func buildCLIChatAssistantMessage(sessionID string, result brain.DispatchResult) engine.SessionMessage {
	message := engine.SessionMessage{
		ID:        localChatID("msg"),
		SessionID: sessionID,
		Role:      "assistant",
		Kind:      "assistant",
		CreatedAt: time.Now().UTC(),
		Model:     result.Model,
		Skill:     result.Command.Skill,
	}
	switch {
	case result.Command.Action == "casual_chat":
		message.Kind = "chat"
		message.Status = "completed"
		message.Content = result.Command.Explanation
	case result.Blocked != nil:
		alternativeMessage := ""
		if result.Alternative != nil {
			alternativeMessage = result.Alternative.Message
		}
		message.Kind = "blocked"
		message.Status = "blocked"
		message.Content = firstChatValue(alternativeMessage, result.Blocked.Reason, "Request blocked by policy.")
	case result.Workflow != nil:
		message.Status = "completed"
		message.WorkflowID = result.Workflow.ID
		message.Content = fmt.Sprintf("Routed into %s using %s.", firstChatValue(result.Workflow.Name, "a workflow"), firstChatValue(result.Command.Skill, "the selected skill"))
	default:
		message.Status = "completed"
		message.Content = fmt.Sprintf("Routed into %s.", firstChatValue(result.Command.Skill, "the selected skill"))
	}
	return message
}

func translateCLITrace(trace *brain.PromptTrace) *engine.SessionPromptTrace {
	if trace == nil {
		return nil
	}
	return &engine.SessionPromptTrace{
		Mode:         string(trace.Mode),
		SystemPrompt: trace.SystemPrompt,
		UserPrompt:   trace.UserPrompt,
		Model:        trace.Model,
		RawResponse:  trace.RawResponse,
		Error:        trace.Error,
		CreatedAt:    time.Now().UTC(),
	}
}

func localChatID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
}
