# Gateway / UI Watchlist Migration + Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make state-backed watchlists (`state/research/watchlists/`) the canonical source. Demote the `research_watchlists` vault key to deprecated import-only input. Remove the raw JSON textarea from the vault settings form. Surface scheduler state in the gateway/console diagnostics views.

**Architecture:** A one-time idempotent migration runs on daemon startup: if state-backed watchlists exist, do nothing; otherwise, parse `research_watchlists` from the vault and persist to state, then audit. The HTTP API stops accepting watchlist JSON via the vault PUT endpoint and surfaces watchlists exclusively through the existing `/api/v1/research/watchlists` endpoints. The settings form drops the textarea; watchlist editing happens in the existing research panel. Scheduler state is exposed via the already-extended `/api/v1/status` (Plan 2) and rendered in `app.js`.

**Tech Stack:** Go 1.22 stdlib, embedded HTML/CSS/JS in `internal/gateway/ui/`. No new dependencies.

**Depends on:** Plan 2 (scheduler state exists at `state/research/scheduler.json`; `dossier.Service.SaveWatchlists` is the canonical write path).

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `internal/dossier/migrate.go` | NEW — `MigrateLegacyWatchlists(ctx, svc, secrets) (count, error)` runs once at startup |
| `internal/dossier/migrate_test.go` | NEW — idempotency, parse failure, no-op when state already populated |
| `cmd/pookie/main.go` | MODIFY — call `dossier.MigrateLegacyWatchlists` in `cmdStart` after `buildStack` |
| `internal/gateway/server.go` | MODIFY — reject `research_watchlists` writes via vault PUT with 400; document deprecation in error |
| `internal/gateway/server_test.go` | MODIFY — add tests asserting vault PUT rejects `research_watchlists` and that schedule still validates |
| `internal/gateway/ui/index.html` | MODIFY — remove `<textarea name="research_watchlists">` block; add a `<section id="research-scheduler">` placeholder |
| `internal/gateway/ui/app.js` | MODIFY — remove the form code paths that read/post `research_watchlists`; add a `renderSchedulerStatus()` that polls `/api/v1/status` |
| `internal/gateway/ui/style.css` | MODIFY — minimal styles for the new scheduler block |
| `CHANGELOG.md` | MODIFY — note breaking API change |
| `README.md` | MODIFY — point users at `pookie research watchlists apply` |

---

## Phase A — Backend migration

### Task A1: `MigrateLegacyWatchlists`

**Files:**
- Create: `internal/dossier/migrate.go`
- Create: `internal/dossier/migrate_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/dossier/migrate_test.go`:

```go
package dossier

import (
	"context"
	"testing"
)

type fakeSecrets struct {
	values map[string]string
}

func (f *fakeSecrets) Get(name string) (string, error) {
	return f.values[name], nil
}

func TestMigrateImportsLegacyWhenStateEmpty(t *testing.T) {
	svc := newTestService(t)
	secrets := &fakeSecrets{values: map[string]string{
		"research_watchlists": `[{"id":"wl-1","name":"alpha","topic":"AI"}]`,
	}}
	n, err := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n != 1 {
		t.Fatalf("imported = %d, want 1", n)
	}
	all, _ := svc.ListWatchlists(context.Background())
	if len(all) != 1 || all[0].Name != "alpha" {
		t.Fatalf("watchlists = %+v", all)
	}
}

func TestMigrateNoopWhenStatePopulated(t *testing.T) {
	svc := newTestService(t)
	_, _ = svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "existing", Name: "existing"}})
	secrets := &fakeSecrets{values: map[string]string{
		"research_watchlists": `[{"id":"new","name":"new"}]`,
	}}
	n, err := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected no import when state already populated, got %d", n)
	}
	all, _ := svc.ListWatchlists(context.Background())
	if len(all) != 1 || all[0].Name != "existing" {
		t.Fatalf("state was overwritten: %+v", all)
	}
}

func TestMigrateNoopWhenLegacyEmpty(t *testing.T) {
	svc := newTestService(t)
	secrets := &fakeSecrets{values: map[string]string{}}
	n, err := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 when no legacy data, got %d", n)
	}
}

func TestMigrateInvalidJSONReturnsError(t *testing.T) {
	svc := newTestService(t)
	secrets := &fakeSecrets{values: map[string]string{
		"research_watchlists": "not json",
	}}
	_, err := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	svc := newTestService(t)
	secrets := &fakeSecrets{values: map[string]string{
		"research_watchlists": `[{"id":"wl-1","name":"alpha"}]`,
	}}
	n1, _ := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	n2, _ := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	if n1 != 1 || n2 != 0 {
		t.Fatalf("first=%d second=%d, want 1 then 0", n1, n2)
	}
}
```

- [ ] **Step 2: Verify it fails**

Run: `go test ./internal/dossier/... -run TestMigrate -v`
Expected: FAIL — `MigrateLegacyWatchlists` undefined.

- [ ] **Step 3: Implement `migrate.go`**

Create `internal/dossier/migrate.go`:

```go
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
// `research_watchlists` vault key into state-backed storage iff state is
// empty. Returns the number of watchlists imported (0 means no-op).
//
// This is safe to call on every daemon start: it short-circuits when state
// already has any watchlists, so user edits made via the new path are never
// overwritten.
//
// The legacy vault key is intentionally NOT cleared after import — the user
// may still want to inspect it. The gateway will refuse to write to it.
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/dossier/... -v`
Expected: PASS for all migrate tests.

- [ ] **Step 5: Commit**

```bash
git add internal/dossier/migrate.go internal/dossier/migrate_test.go
git commit -m "feat(dossier): import legacy research_watchlists vault key into state once"
```

---

### Task A2: Run migration on daemon start

**Files:**
- Modify: `cmd/pookie/main.go`

- [ ] **Step 1: Add migration call in `cmdStart`**

After `buildStack` returns successfully and before launching the scheduler goroutine (added in Plan 2), insert:

```go
	migrated, err := dossier.MigrateLegacyWatchlists(context.Background(), stack.dossier, stack.secrets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: legacy watchlist migration failed: %v\n", err)
	} else if migrated > 0 {
		fmt.Fprintf(os.Stderr, "migrated %d legacy watchlist(s) from vault into state\n", migrated)
	}
```

Add the import `"github.com/mitpoai/pookiepaws/internal/dossier"` if not already present (Plan 2 already added it).

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 3: Manual smoke**

1. Set `research_watchlists` in `<runtime-root>/security.json` to a JSON array with one entry.
2. Delete any files in `<runtime-root>/state/research/watchlists/`.
3. Run `go run ./cmd/pookie start --addr 127.0.0.1:18800` for ~2 seconds, then stop.
4. Expected: stderr contains "migrated 1 legacy watchlist(s)"; the file appears under `state/research/watchlists/`.
5. Restart the daemon. Expected: no migration message (idempotent).

- [ ] **Step 4: Commit**

```bash
git add cmd/pookie/main.go
git commit -m "feat(start): run legacy watchlist migration once on daemon start"
```

---

### Task A3: Reject `research_watchlists` in vault PUT

**Files:**
- Modify: `internal/gateway/server.go`
- Modify: `internal/gateway/server_test.go`

The vault PUT handler currently accepts a `research_watchlists` field in `VaultUpdateRequest` (around `internal/gateway/server.go:105–128`) and writes the raw string into the secret store. We want to reject any non-empty value with a 400 explaining the migration.

- [ ] **Step 1: Write failing tests**

In `internal/gateway/server_test.go`, append:

```go
func TestVaultPUTRejectsResearchWatchlists(t *testing.T) {
	srv := newTestGateway(t, t.TempDir())
	body := `{"research_watchlists":"[{\"id\":\"wl-1\",\"name\":\"x\"}]"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), "research_watchlists") {
		t.Errorf("expected error to mention the deprecated key: %s", resp.Body.String())
	}
}

func TestVaultPUTAllowsEmptyResearchWatchlists(t *testing.T) {
	// An empty string for the deprecated key is treated as "not set" — must
	// not 400. This keeps form re-submissions from old UIs working until
	// they're refreshed.
	srv := newTestGateway(t, t.TempDir())
	body := `{"research_watchlists":"","research_schedule":"hourly"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
}

func TestVaultPUTStillValidatesSchedule(t *testing.T) {
	srv := newTestGateway(t, t.TempDir())
	body := `{"research_schedule":"weekly"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/vault", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.Code)
	}
}
```

(Use the same `newTestGateway` helper introduced in Plan 2 Task E1.)

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/gateway/... -run TestVaultPUT -v`
Expected: rejection test FAILs (still a 200), the others may already pass or fail depending on prior state.

- [ ] **Step 3: Update the handler**

In `internal/gateway/server.go`, locate `handleSettingsVault` (around line 1375). Find the block that handles `research_watchlists` (search for `"research_watchlists"`). Replace it with:

```go
	// research_watchlists is deprecated as a writable vault field. Watchlists
	// are now stored under state/research/watchlists/ and edited via the
	// research API or `pookie research watchlists apply`. Empty is allowed
	// for backward-compatible form posts; non-empty is rejected.
	if v := strings.TrimSpace(req.ResearchWatchlists); v != "" {
		http.Error(w, "research_watchlists is no longer writable via /api/v1/settings/vault — use POST /api/v1/research/watchlists or `pookie research watchlists apply`", http.StatusBadRequest)
		return
	}
```

Also remove the line that previously called `secrets.Set("research_watchlists", ...)` and the `dossier.SaveWatchlists` call that followed it; the migration path covers initial import, and the research API covers ongoing writes.

The `ResearchWatchlists` field should remain on `VaultUpdateRequest` (so old payloads parse cleanly) but is now write-rejected.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/gateway/... -v`
Expected: PASS for all three new tests and the broader suite.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/server.go internal/gateway/server_test.go
git commit -m "feat(gateway): reject writes to deprecated research_watchlists vault key"
```

---

## Phase B — UI cleanup

### Task B1: Remove watchlist textarea from settings form

**Files:**
- Modify: `internal/gateway/ui/index.html`
- Modify: `internal/gateway/ui/app.js`
- Modify: `internal/gateway/ui/style.css`

- [ ] **Step 1: Locate the textarea in `index.html`**

Search: `grep -n "research_watchlists" internal/gateway/ui/index.html internal/gateway/ui/app.js`

The textarea is around `index.html:468–469`. Capture surrounding context (label + helper text) before deleting.

- [ ] **Step 2: Replace with a deprecation notice**

Replace the entire `<label>...<textarea name="research_watchlists">...</textarea>...</label>` block with:

```html
<div class="setting-deprecated">
  <p><strong>Watchlists moved.</strong> Edit them in the
  <a href="#research">Research panel</a> or via
  <code>pookie research watchlists apply --file watchlists.json</code>.</p>
</div>
```

- [ ] **Step 3: Remove watchlist code paths from `app.js`**

In `internal/gateway/ui/app.js`:

1. Remove any `research_watchlists` occurrence from the form-build path (search the file).
2. Remove any code that includes `research_watchlists` in the `PUT /api/v1/settings/vault` payload — submitting it now causes a 400.
3. Keep `research_schedule` and `research_provider` paths intact.

Search-and-verify command:

```bash
grep -n "research_watchlists" internal/gateway/ui/app.js
```

After your edits this should return zero hits.

- [ ] **Step 4: Add minimal style**

Append to `internal/gateway/ui/style.css`:

```css
.setting-deprecated {
  border-left: 3px solid #c69300;
  background: #fff8e1;
  padding: 0.6rem 0.8rem;
  margin: 0.5rem 0;
  font-size: 0.9rem;
}
.setting-deprecated code {
  background: #f3eccc;
  padding: 0 0.3em;
  border-radius: 3px;
}
```

- [ ] **Step 5: Manual smoke**

```bash
go run ./cmd/pookie start --addr 127.0.0.1:18800
```

Open `http://127.0.0.1:18800/`. In the Settings panel, verify:
- The watchlist textarea is gone.
- A yellow notice with the new instructions appears in its place.
- Saving the form (with schedule changed) succeeds (200).
- The Research panel still lists state-backed watchlists.

- [ ] **Step 6: Commit**

```bash
git add internal/gateway/ui/index.html internal/gateway/ui/app.js internal/gateway/ui/style.css
git commit -m "feat(ui): remove deprecated watchlist textarea from vault settings"
```

---

### Task B2: Render scheduler status in the console

**Files:**
- Modify: `internal/gateway/ui/index.html`
- Modify: `internal/gateway/ui/app.js`

- [ ] **Step 1: Add markup**

In `index.html`, inside the diagnostics/console section (search for an existing block like `<section id="status">` or `<section id="diagnostics">`), add:

```html
<section id="research-scheduler" class="card">
  <h3>Research scheduler</h3>
  <dl>
    <dt>Schedule</dt><dd id="sched-mode">—</dd>
    <dt>Last tick</dt><dd id="sched-last-tick">—</dd>
    <dt>Last success</dt><dd id="sched-last-success">—</dd>
    <dt>Next due</dt><dd id="sched-next-due">—</dd>
    <dt>Last error</dt><dd id="sched-last-error">—</dd>
  </dl>
</section>
```

- [ ] **Step 2: Add render function in `app.js`**

In `app.js`, add (place it next to the existing status renderers):

```javascript
function renderSchedulerStatus(status) {
  const sched = status && status.scheduler ? status.scheduler : {};
  const setText = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value || "—";
  };
  setText("sched-mode", sched.schedule);
  setText("sched-last-tick", sched.last_tick_at);
  setText("sched-last-success", sched.last_success_at);
  setText("sched-next-due", sched.next_due_at);
  setText("sched-last-error", sched.last_error);
}
```

Find the existing status fetch (search for `/api/v1/status`). Inside the success branch, after the existing renderers run, add:

```javascript
renderSchedulerStatus(payload);
```

- [ ] **Step 3: Manual smoke**

Restart the daemon (with `research_schedule=hourly` in vault). Open the console UI; the new "Research scheduler" card should populate within one polling cycle.

- [ ] **Step 4: Commit**

```bash
git add internal/gateway/ui/index.html internal/gateway/ui/app.js
git commit -m "feat(ui): render scheduler diagnostics in console"
```

---

## Phase C — Documentation

### Task C1: CHANGELOG and README

- [ ] **Step 1: Update CHANGELOG**

Append to `[Unreleased]`:

```markdown
### Changed
- `PUT /api/v1/settings/vault` now rejects non-empty `research_watchlists`
  with HTTP 400. Watchlists are edited via `POST /api/v1/research/watchlists`
  or `pookie research watchlists apply`.
- The web settings form no longer contains a watchlist textarea.

### Migrated
- On daemon startup, an existing `research_watchlists` vault value is imported
  into state-backed storage exactly once (no-op when state is non-empty).
```

- [ ] **Step 2: Update README**

In the existing watchlist/research section, add:

```markdown
> **Migration note.** Earlier versions stored watchlists in the
> `research_watchlists` vault key. That value is now imported into
> `state/research/watchlists/` on first startup and the vault key becomes
> read-only. Edit watchlists via:
>
>     pookie research watchlists apply --file watchlists.json
>
> or the Research panel in the web console.
```

- [ ] **Step 3: Final test pass**

Run: `go test ./...`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md README.md
git commit -m "docs: research_watchlists migration and UI changes"
```

---

## Verification Summary

- `go test ./...` green.
- `PUT /api/v1/settings/vault` with non-empty `research_watchlists` returns 400 with a message pointing to the new path.
- `PUT /api/v1/settings/vault` with empty `research_watchlists` plus a valid `research_schedule` returns 200 (back-compat for old form submissions).
- Restarting a daemon with a populated `research_watchlists` vault and an empty `state/research/watchlists/` directory: imports once, logs the migration to stderr, populates state.
- Restarting after migration: no further imports; user edits via the API or `pookie research watchlists apply` are preserved.
- The web settings form no longer shows a watchlist textarea; a yellow deprecation notice points at the Research panel and the CLI.
- The console UI shows a "Research scheduler" card populated from `/api/v1/status`.

## Out of scope (deferred)

- A migration command (`pookie research watchlists migrate`) — startup migration is sufficient.
- Removing the `ResearchWatchlists` field from `VaultUpdateRequest` — keeping it makes back-compat parsing cleaner; deletion can happen in a later cycle.
- Per-watchlist UI editor (add/remove/reorder rows directly in the form) — out of scope; CLI/API is the primary path.
- Packaging/release docs (Plan 4).
