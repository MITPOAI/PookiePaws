package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/cli"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

// cmdRun executes a named skill headlessly (no HTTP server) and prints the
// result in the terminal. Input values are supplied via repeated --input
// key=value flags; the workflow coordinator handles validation.
func cmdRun(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pookie run <skill-name> [--input key=value ...]")
		os.Exit(1)
	}

	skillName := args[0]

	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var inputPairs []string
	fs.Func("input", "workflow input as key=value (repeatable)", func(s string) error {
		inputPairs = append(inputPairs, s)
		return nil
	})
	timeout := fs.Duration("timeout", 60*time.Second, "maximum time to wait for completion")
	home := fs.String("home", "", "override runtime home directory")
	_ = fs.Parse(args[1:])

	// Build input map from --input flags.
	input := make(map[string]any, len(inputPairs))
	for _, kv := range inputPairs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "invalid --input %q: expected key=value\n", kv)
			os.Exit(1)
		}
		input[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}

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
	spin.Stop(true, "Engine ready")

	// Verify the skill exists before submitting.
	defs := stack.coord.SkillDefinitions()
	var skillDesc string
	found := false
	for _, d := range defs {
		if d.Name == skillName {
			found = true
			skillDesc = d.Description
			break
		}
	}
	if !found {
		p.Error("skill %q not found", skillName)
		p.Blank()
		p.Plain("Available skills:")
		for _, d := range defs {
			p.Plain("  %-30s  %s", d.Name, d.Description)
		}
		p.Blank()
		os.Exit(1)
	}
	if skillDesc != "" {
		p.Info("%s", skillDesc)
	}

	// Subscribe to events before submitting to avoid missing early completions.
	ctx := context.Background()
	sub := stack.bus.Subscribe(32)
	defer stack.bus.Unsubscribe(sub.ID)

	wf, err := stack.coord.SubmitWorkflow(ctx, engine.WorkflowDefinition{
		Name:  skillName,
		Skill: skillName,
		Input: input,
	})
	if err != nil {
		p.Error("Failed to submit workflow: %v", err)
		os.Exit(1)
	}
	p.Info("Workflow %s submitted", wf.ID)

	execSpin := p.NewSpinner(fmt.Sprintf("Running %s…", skillName))
	execSpin.Start()

	deadline := time.Now().Add(*timeout)
	reader := bufio.NewReader(os.Stdin)

	for time.Now().Before(deadline) {
		// Refresh workflow state.
		all, listErr := stack.coord.ListWorkflows(ctx)
		if listErr == nil {
			for _, w := range all {
				if w.ID == wf.ID {
					wf = w
					break
				}
			}
		}

		switch wf.Status {
		case engine.WorkflowCompleted:
			execSpin.Stop(true, fmt.Sprintf("%s completed", skillName))
			printResult(p, wf)
			return

		case engine.WorkflowFailed:
			execSpin.Stop(false, fmt.Sprintf("%s failed", skillName))
			if wf.Error != "" {
				p.Error("%s", wf.Error)
			}
			os.Exit(1)

		case engine.WorkflowRejected:
			execSpin.Stop(false, fmt.Sprintf("%s rejected", skillName))
			os.Exit(1)

		case engine.WorkflowWaitingApproval:
			execSpin.Stop(true, "Approval required — review the action below")
			if !handlePendingApprovals(ctx, p, stack.coord, reader) {
				p.Error("No approvals handled; aborting")
				os.Exit(1)
			}
			// Restart spinner for the remainder of the workflow.
			execSpin = p.NewSpinner(fmt.Sprintf("Continuing %s…", skillName))
			execSpin.Start()
		}

		// Wait for an event or poll interval, whichever comes first.
		select {
		case <-sub.C:
		case <-time.After(300 * time.Millisecond):
		}
	}

	execSpin.Stop(false, "Timed out")
	p.Error("Workflow did not complete within %s", *timeout)
	os.Exit(1)
}

func printResult(p *cli.Printer, wf engine.Workflow) {
	rows := [][2]string{
		{"skill", wf.Skill},
		{"status", string(wf.Status)},
		{"id", wf.ID},
	}
	for k, v := range wf.Output {
		rows = append(rows, [2]string{k, fmt.Sprintf("%v", v)})
	}
	p.Blank()
	p.Box("Result", rows)
	p.Blank()
}

func handlePendingApprovals(
	ctx context.Context,
	p *cli.Printer,
	coord engine.WorkflowCoordinator,
	reader *bufio.Reader,
) bool {
	approvals, err := coord.ListApprovals(ctx)
	if err != nil {
		p.Error("list approvals: %v", err)
		return false
	}

	handled := false
	for _, a := range approvals {
		if a.State != engine.ApprovalPending {
			continue
		}
		p.Blank()
		p.Box("Approval Required", [][2]string{
			{"id", a.ID},
			{"adapter", a.Adapter},
			{"action", a.Action},
			{"workflow", a.WorkflowID},
		})
		p.Plain("Approve this action? [y/N] ")
		fmt.Fprint(os.Stdout, "  > ")

		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))

		if line == "y" || line == "yes" {
			if _, err := coord.Approve(ctx, a.ID); err != nil {
				p.Error("approve: %v", err)
			} else {
				p.Success("Approved")
				handled = true
			}
		} else {
			if _, err := coord.Reject(ctx, a.ID); err != nil {
				p.Error("reject: %v", err)
			} else {
				p.Warning("Rejected")
				handled = true
			}
		}
	}
	return handled
}
