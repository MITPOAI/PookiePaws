package dossier

import (
	"context"
	"testing"
)

type fakeSecretsForMigrate struct {
	values map[string]string
}

func (f *fakeSecretsForMigrate) Get(name string) (string, error) {
	return f.values[name], nil
}

func TestMigrateImportsLegacyWhenStateEmpty(t *testing.T) {
	svc := newDossierServiceForTest(t)
	secrets := &fakeSecretsForMigrate{values: map[string]string{
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
	svc := newDossierServiceForTest(t)
	_, _ = svc.SaveWatchlists(context.Background(), []Watchlist{{ID: "existing", Name: "existing"}})
	secrets := &fakeSecretsForMigrate{values: map[string]string{
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
	svc := newDossierServiceForTest(t)
	secrets := &fakeSecretsForMigrate{values: map[string]string{}}
	n, err := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 when no legacy data, got %d", n)
	}
}

func TestMigrateInvalidJSONReturnsError(t *testing.T) {
	svc := newDossierServiceForTest(t)
	secrets := &fakeSecretsForMigrate{values: map[string]string{
		"research_watchlists": "not json",
	}}
	if _, err := MigrateLegacyWatchlists(context.Background(), svc, secrets); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	svc := newDossierServiceForTest(t)
	secrets := &fakeSecretsForMigrate{values: map[string]string{
		"research_watchlists": `[{"id":"wl-1","name":"alpha"}]`,
	}}
	n1, _ := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	n2, _ := MigrateLegacyWatchlists(context.Background(), svc, secrets)
	if n1 != 1 || n2 != 0 {
		t.Fatalf("first=%d second=%d, want 1 then 0", n1, n2)
	}
}
