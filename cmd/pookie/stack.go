package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/security"
	"github.com/mitpoai/pookiepaws/internal/skills"
	"github.com/mitpoai/pookiepaws/internal/state"
)

// appStack holds every initialised runtime component.
// cmdStart adds an HTTP server on top; cmdRun uses it headlessly.
type appStack struct {
	bus         engine.EventBus
	subturns    engine.SubTurnManager
	coord       engine.WorkflowCoordinator
	store       engine.StateStore
	secrets     *security.JSONSecretProvider
	brainSvc    *brain.DynamicService
	tools       *brain.ToolRegistry
	sandbox     engine.Sandbox
	runtimeRoot string
	workspace   string
}

// buildStack initialises all runtime components in dependency order.
// It mirrors the setup in the original cmd/pookiepaws/main.go.
func buildStack(runtimeRoot, workspaceRoot string) (*appStack, error) {
	bus := engine.NewEventBus()

	subturns := engine.NewSubTurnManager(engine.SubTurnManagerConfig{
		MaxDepth:           4,
		MaxConcurrent:      8,
		ConcurrencyTimeout: 10 * time.Second,
		DefaultTimeout:     30 * time.Second,
		Bus:                bus,
	})

	sandbox, err := security.NewWorkspaceSandbox(runtimeRoot, workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}

	secrets, err := security.NewJSONSecretProvider(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}

	store, err := state.NewFileStore(filepath.Join(runtimeRoot, "state"))
	if err != nil {
		return nil, fmt.Errorf("state store: %w", err)
	}

	providers := brain.NewSecretBackedProviderFactory(secrets)
	memory, err := brain.NewPersistentMemory(runtimeRoot, providers, bus)
	if err != nil {
		return nil, fmt.Errorf("persistent memory: %w", err)
	}

	interceptor := security.NewSkillExecutionInterceptor()

	registry, err := skills.NewDefaultRegistry()
	if err != nil {
		return nil, fmt.Errorf("skill registry: %w", err)
	}

	coord, err := engine.NewWorkflowCoordinator(engine.WorkflowCoordinatorConfig{
		Bus:         bus,
		SubTurns:    subturns,
		Store:       store,
		Skills:      registry,
		Sandbox:     sandbox,
		Secrets:     secrets,
		Memory:      memory,
		Interceptor: interceptor,
		CRMAdapter:  adapters.NewSalesmanagoAdapter(),
		SMSAdapter:  adapters.NewMittoAdapter(),
		WhatsApp:    adapters.NewWhatsAppAdapter(),
		RuntimeRoot: runtimeRoot,
		Workspace:   workspaceRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("coordinator: %w", err)
	}

	permSandbox := security.NewPermissionedSandbox(sandbox, coord, bus)
	coord.SetSandbox(permSandbox)

	windowPath := filepath.Join(runtimeRoot, "state", "runtime", "conversation-window.json")
	brainSvc := brain.NewDynamicService(secrets, coord, bus).
		WithWindowPath(windowPath).
		WithMemory(memory)

	// Build the ReAct tool registry.
	tools := brain.NewToolRegistry()
	tools.Register(&brain.JinaScraperTool{})
	tools.Register(&brain.ExportMarkdownTool{Sandbox: sandbox})
	tools.Register(&brain.ReadLocalFileTool{Sandbox: sandbox})
	tools.Register(&brain.OSCommandTool{
		Guard: security.NewCommandExecGuard(),
		// Approve is set per-caller (CLI prompts the user; HTTP auto-denies).
	})

	return &appStack{
		bus:         bus,
		subturns:    subturns,
		coord:       coord,
		store:       store,
		secrets:     secrets,
		brainSvc:    brainSvc,
		tools:       tools,
		sandbox:     sandbox,
		runtimeRoot: runtimeRoot,
		workspace:   workspaceRoot,
	}, nil
}

// Close shuts down background goroutines. Best-effort; errors are ignored.
func (s *appStack) Close() {
	_ = s.subturns.Close()
	s.bus.Close()
}
