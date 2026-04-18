package dossier

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// LegacySecrets is the minimal interface MigrateLegacyWatchlists needs.
// It deliberately does NOT depend on engine.SecretProvider so this package
// stays free of the engine import cycle.
type LegacySecrets interface {
	Get(name string) (string, error)
}

// MigrateLegacyWatchlists imports watchlists from the legacy
// research_watchlists vault key into state-backed storage iff state is
// empty. Returns the number of watchlists imported (0 means no-op).
//
// Safe to call on every daemon start: short-circuits when state already
// has any watchlists, so user edits made via the new path are never
// overwritten. The legacy vault key is intentionally NOT cleared after
// import — the gateway will refuse to write to it.
func MigrateLegacyWatchlists(ctx context.Context, svc *Service, secrets LegacySecrets) (int, error) {
	existing, err := svc.ListWatchlists(ctx)
	if err != nil {
		return 0, fmt.Errorf("list state watchlists: %w", err)
	}
	if len(existing) > 0 {
		return 0, nil
	}

	raw, _ := secrets.Get("research_watchlists")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	var legacy []Watchlist
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return 0, fmt.Errorf("parse legacy research_watchlists: %w", err)
	}
	if len(legacy) == 0 {
		return 0, nil
	}
	saved, err := svc.SaveWatchlists(ctx, legacy)
	if err != nil {
		return 0, fmt.Errorf("save migrated watchlists: %w", err)
	}
	return len(saved), nil
}
