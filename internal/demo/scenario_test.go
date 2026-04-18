package demo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/security"
	"github.com/mitpoai/pookiepaws/internal/skills"
	"github.com/mitpoai/pookiepaws/internal/state"
)

func TestRunScenarioSmokeSavesArtifactAndLatestResult(t *testing.T) {
	root := t.TempDir()
	runtimeRoot := filepath.Join(root, ".pookiepaws")
	workspaceRoot := filepath.Join(runtimeRoot, "workspace")

	bus := engine.NewEventBus()
	subturns := engine.NewSubTurnManager(engine.SubTurnManagerConfig{
		MaxDepth:           4,
		MaxConcurrent:      2,
		ConcurrencyTimeout: time.Second,
		DefaultTimeout:     time.Second,
		Bus:                bus,
	})
	defer subturns.Close()
	defer bus.Close()

	sandbox, err := security.NewWorkspaceSandbox(runtimeRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("sandbox: %v", err)
	}
	secrets, err := security.NewJSONSecretProvider(runtimeRoot)
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}
	store, err := state.NewFileStore(filepath.Join(runtimeRoot, "state"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	registry, err := skills.NewDefaultRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	coord, err := engine.NewWorkflowCoordinator(engine.WorkflowCoordinatorConfig{
		Bus:         bus,
		SubTurns:    subturns,
		Store:       store,
		Skills:      registry,
		Sandbox:     sandbox,
		Secrets:     secrets,
		Interceptor: security.NewSkillExecutionInterceptor(),
		CRMAdapter:  adapters.NewMockSalesmanagoAdapter(),
		SMSAdapter:  adapters.NewMockMittoAdapter(),
		WhatsApp:    adapters.NewMockWhatsAppAdapter(),
		RuntimeRoot: runtimeRoot,
		Workspace:   workspaceRoot,
	})
	if err != nil {
		t.Fatalf("coordinator: %v", err)
	}

	result, err := RunScenarioSmoke(context.Background(), coord, runtimeRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("run scenario smoke: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected scenario smoke to pass, got %+v", result)
	}
	if result.ArtifactPath == "" {
		t.Fatalf("expected artifact path")
	}

	artifact, err := os.ReadFile(result.ArtifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	artifactText := string(artifact)
	for _, needle := range []string{"PookiePaws Reserve", "OpenClaw", "Recommendations", "Next Actions"} {
		if !strings.Contains(artifactText, needle) {
			t.Fatalf("expected artifact to contain %q", needle)
		}
	}

	latest, err := LoadLatest(runtimeRoot)
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if latest == nil || latest.ArtifactPath != result.ArtifactPath {
		t.Fatalf("expected latest result to be stored, got %+v", latest)
	}
}
