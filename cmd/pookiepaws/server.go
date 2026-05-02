package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mitpoai/pookiepaws/internal/adapters"
	"github.com/mitpoai/pookiepaws/internal/brain"
	"github.com/mitpoai/pookiepaws/internal/dossier"
	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/gateway"
	"github.com/mitpoai/pookiepaws/internal/persistence"
	"github.com/mitpoai/pookiepaws/internal/security"
	"github.com/mitpoai/pookiepaws/internal/skills"
	"github.com/mitpoai/pookiepaws/internal/state"
)

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:18800", "HTTP listen address")
	homeOverride := fs.String("home", "", "override runtime home")
	_ = fs.Parse(args)

	runtimeRoot, workspaceRoot, err := resolveServeRoots(*homeOverride)
	if err != nil {
		log.Fatalf("resolve runtime roots: %v", err)
	}

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
		log.Fatalf("create sandbox: %v", err)
	}

	secrets, err := security.NewJSONSecretProvider(runtimeRoot)
	if err != nil {
		log.Fatalf("create secret provider: %v", err)
	}
	storageFormat := persistence.NormalizeFormat(optionalSecret(secrets, "storage_format"))

	store, err := state.NewFileStoreWithOptions(filepath.Join(runtimeRoot, "state"), state.Options{
		Format: storageFormat,
	})
	if err != nil {
		log.Fatalf("create state store: %v", err)
	}

	providers := brain.NewSecretBackedProviderFactory(secrets)
	memory, err := brain.NewPersistentMemoryWithOptions(runtimeRoot, providers, bus, storageFormat)
	if err != nil {
		log.Fatalf("create persistent memory: %v", err)
	}
	interceptor := security.NewSkillExecutionInterceptor()

	registry, err := skills.NewDefaultRegistry()
	if err != nil {
		log.Fatalf("load default skills: %v", err)
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
		log.Fatalf("create workflow coordinator: %v", err)
	}

	permSandbox := security.NewPermissionedSandbox(sandbox, coord, bus)
	coord.SetSandbox(permSandbox)

	promptBrain := brain.NewDynamicService(secrets, coord, bus).WithMemory(memory)
	if !promptBrain.Available() {
		log.Printf("brain disabled: no LLM provider configured")
	}

	dossierSvc, err := dossier.NewService(runtimeRoot)
	if err != nil {
		log.Fatalf("create dossier service: %v", err)
	}

	shutdown := make(chan struct{}, 1)
	api := gateway.NewServer(gateway.Config{
		Coordinator: coord,
		EventBus:    bus,
		Brain:       promptBrain,
		Store:       store,
		Vault:       secrets,
		WhatsApp:    adapters.NewWhatsAppAdapter(),
		Dossier:     dossierSvc,
		Address:     *addr,
		RequestShutdown: func() {
			select {
			case shutdown <- struct{}{}:
			default:
			}
		},
	})

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("pookiepaws listening on http://%s", *addr)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
	case <-shutdown:
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	if err := subturns.Close(); err != nil {
		log.Printf("subturn manager close error: %v", err)
	}
	bus.Close()
}

func resolveServeRoots(homeOverride string) (string, string, error) {
	if homeOverride != "" {
		return homeOverride, filepath.Join(homeOverride, "workspace"), nil
	}

	if custom := os.Getenv("POOKIEPAWS_HOME"); custom != "" {
		return custom, filepath.Join(custom, "workspace"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}

	root := filepath.Join(home, ".pookiepaws")
	return root, filepath.Join(root, "workspace"), nil
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
