package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/persistence"
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
	dossier     *dossier.Service
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
	storageFormat := persistence.NormalizeFormat(optionalSecret(secrets, "storage_format"))

	store, err := state.NewFileStoreWithOptions(filepath.Join(runtimeRoot, "state"), state.Options{
		Format: storageFormat,
	})
	if err != nil {
		return nil, fmt.Errorf("state store: %w", err)
	}

	providers := brain.NewSecretBackedProviderFactory(secrets)
	memory, err := brain.NewPersistentMemoryWithOptions(runtimeRoot, providers, bus, storageFormat)
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

	dossierSvc, err := dossier.NewService(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("dossier service: %w", err)
	}

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
		dossier:     dossierSvc,
		sandbox:     sandbox,
		runtimeRoot: runtimeRoot,
		workspace:   workspaceRoot,
	}, nil
}

func optionalSecret(secrets *security.JSONSecretProvider, key string) string {
	if secrets == nil {
		return ""
	}
	value, err := secrets.Get(key)
	if err != nil {
		return ""
	}
	return value
}

// Close shuts down background goroutines. Best-effort; errors are ignored.
func (s *appStack) Close() {
	_ = s.subturns.Close()
	s.bus.Close()
}

// currentHome returns the user's home directory, or "." if it cannot be
// determined. Used by the single-result resolver helpers below so one-shot
// CLI subcommands can derive runtime paths without juggling errors.
func currentHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// resolveRuntimeRoot returns the runtime root for one-shot CLI subcommands
// that don't accept a --home flag. Honours POOKIEPAWS_HOME via resolveRoots.
func resolveRuntimeRoot() string {
	rt, _, _ := resolveRoots("")
	if rt != "" {
		return rt
	}
	rt, _, _ = resolveRoots(currentHome())
	return rt
}

// resolveWorkspaceRoot mirrors resolveRuntimeRoot for the workspace path.
func resolveWorkspaceRoot() string {
	_, ws, _ := resolveRoots("")
	if ws != "" {
		return ws
	}
	_, ws, _ = resolveRoots(currentHome())
	return ws
}

// writeVaultSecret updates a single secret in the on-disk JSON vault while
// preserving every other key. Uses the canonical JSONSecretProvider.Update
// path so the file format and locking semantics match the rest of the app.
func writeVaultSecret(key, value string) error {
	provider, err := security.NewJSONSecretProvider(resolveRuntimeRoot())
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}
	if err := provider.Update(map[string]string{key: value}); err != nil {
		return fmt.Errorf("update vault: %w", err)
	}
	return nil
}
