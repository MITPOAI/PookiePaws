package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

// cmdChat launches an interactive REPL that routes natural-language prompts
// through the PookiePaws brain service. Each prompt may produce a workflow
// submission, a safety intervention, or a friendly error.
func cmdChat(args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	home := fs.String("home", "", "override runtime home directory")
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

	p.Blank()
	p.Info("Chat with Pookie — your marketing co-pilot")
	p.Dim("Type a marketing goal in plain English. Pookie will pick the best skill.")
	p.Dim("Commands:  /skills  /clear  /exit")
	p.Blank()

	ctx := context.Background()
	scanner := bufio.NewScanner(os.Stdin)

	for {
		// Print the prompt in accent colour.
		if p.IsColor() {
			fmt.Fprint(os.Stdout, ansiBoldMagenta+"  Pookie > "+ansiReset)
		} else {
			fmt.Fprint(os.Stdout, "  Pookie > ")
		}

		if !scanner.Scan() {
			// EOF (Ctrl+D) — exit gracefully.
			p.Blank()
			p.Dim("Goodbye! — Pookie")
			p.Blank()
			return
		}

		line := strings.TrimSpace(scanner.Text())
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

		case "/clear":
			fmt.Fprint(os.Stdout, "\033[2J\033[H")
			p.Banner()
			continue

		case "/help":
			p.Blank()
			p.Plain("  /skills   List available marketing skills")
			p.Plain("  /clear    Clear the screen")
			p.Plain("  /exit     Leave the chat")
			p.Blank()
			continue
		}

		// Dispatch the prompt through the brain service.
		spin := p.NewSpinner("Thinking…")
		spin.Start()

		result, err := stack.brainSvc.DispatchPrompt(ctx, line)
		if err != nil {
			spin.Stop(false, "")
			p.Error("%v", err)
			p.Blank()
			continue
		}
		spin.Stop(true, "")

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

// ansiBoldMagenta and ansiReset are re-declared here because they are
// unexported constants in the cli package. We use the same values for
// prompt colouring in this file.
const (
	ansiBoldMagenta = "\033[1;35m"
	ansiReset       = "\033[0m"
)
