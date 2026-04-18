package dossier

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/research"
)

type Service struct {
	root     string
	research *research.Service
}

func NewService(runtimeRoot string) (*Service, error) {
	return NewServiceWithResearch(runtimeRoot, research.NewService())
}

func NewServiceWithResearch(runtimeRoot string, researchService *research.Service) (*Service, error) {
	if researchService == nil {
		researchService = research.NewService()
	}
	root := filepath.Join(runtimeRoot, "state", "research")
	for _, dir := range []string{
		root,
		filepath.Join(root, "watchlists"),
		filepath.Join(root, "dossiers"),
		filepath.Join(root, "evidence"),
		filepath.Join(root, "changes"),
		filepath.Join(root, "recommendations"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return &Service{
		root:     root,
		research: researchService,
	}, nil
}

func ParseTrustedDomains(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})
	return dedupeStrings(parts)
}

func ParseWatchlists(raw string, trustedDomains []string) ([]Watchlist, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var watchlists []Watchlist
	if strings.HasPrefix(raw, "{") {
		var single Watchlist
		if err := json.Unmarshal([]byte(raw), &single); err != nil {
			return nil, fmt.Errorf("decode watchlist: %w", err)
		}
		watchlists = []Watchlist{single}
	} else {
		if err := json.Unmarshal([]byte(raw), &watchlists); err != nil {
			return nil, fmt.Errorf("decode watchlists: %w", err)
		}
	}

	now := time.Now().UTC()
	for index := range watchlists {
		normalizeWatchlist(&watchlists[index], trustedDomains, now)
	}
	return watchlists, nil
}

func (s *Service) ListWatchlists(_ context.Context) ([]Watchlist, error) {
	watchlists, err := listRecords[Watchlist](filepath.Join(s.root, "watchlists"))
	if err != nil {
		return nil, err
	}
	sort.Slice(watchlists, func(i, j int) bool {
		return watchlists[i].Name < watchlists[j].Name
	})
	return watchlists, nil
}

func (s *Service) ListDossiers(_ context.Context, limit int) ([]Dossier, error) {
	items, err := listRecords[Dossier](filepath.Join(s.root, "dossiers"))
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return limitRecords(items, limit), nil
}

func (s *Service) ListEvidence(_ context.Context, dossierID string, limit int) ([]EvidenceRecord, error) {
	items, err := listRecords[EvidenceRecord](filepath.Join(s.root, "evidence"))
	if err != nil {
		return nil, err
	}
	filtered := make([]EvidenceRecord, 0, len(items))
	for _, item := range items {
		if dossierID != "" && item.DossierID != dossierID {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ObservedAt.After(filtered[j].ObservedAt)
	})
	return limitRecords(filtered, limit), nil
}

func (s *Service) ListChanges(_ context.Context, watchlistID string, limit int) ([]ChangeRecord, error) {
	items, err := listRecords[ChangeRecord](filepath.Join(s.root, "changes"))
	if err != nil {
		return nil, err
	}
	filtered := make([]ChangeRecord, 0, len(items))
	for _, item := range items {
		if watchlistID != "" && item.WatchlistID != watchlistID {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ObservedAt.After(filtered[j].ObservedAt)
	})
	return limitRecords(filtered, limit), nil
}

func (s *Service) ListRecommendations(_ context.Context, status RecommendationStatus, limit int) ([]Recommendation, error) {
	items, err := listRecords[Recommendation](filepath.Join(s.root, "recommendations"))
	if err != nil {
		return nil, err
	}
	filtered := make([]Recommendation, 0, len(items))
	for _, item := range items {
		if status != "" && item.Status != status {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	return limitRecords(filtered, limit), nil
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	watchlists, err := s.ListWatchlists(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	dossiers, err := s.ListDossiers(ctx, 6)
	if err != nil {
		return Snapshot{}, err
	}
	changes, err := s.ListChanges(ctx, "", 8)
	if err != nil {
		return Snapshot{}, err
	}
	recommendations, err := s.ListRecommendations(ctx, "", 8)
	if err != nil {
		return Snapshot{}, err
	}
	evidence, err := s.ListEvidence(ctx, "", 12)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Watchlists:      watchlists,
		Dossiers:        dossiers,
		Evidence:        evidence,
		Changes:         changes,
		Recommendations: recommendations,
	}, nil
}

func (s *Service) SaveWatchlists(_ context.Context, watchlists []Watchlist) ([]Watchlist, error) {
	now := time.Now().UTC()
	items := make([]Watchlist, 0, len(watchlists))
	for _, item := range watchlists {
		normalizeWatchlist(&item, item.TrustedDomains, now)
		if err := saveRecord(filepath.Join(s.root, "watchlists"), item.ID, item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) RefreshConfiguredWatchlists(ctx context.Context, secrets engine.SecretProvider) (RefreshResult, error) {
	watchlists, err := watchlistsFromSecrets(secrets)
	if err != nil {
		return RefreshResult{}, err
	}
	if len(watchlists) == 0 {
		return RefreshResult{}, fmt.Errorf("research_watchlists is empty; save at least one watchlist before running refresh")
	}
	return s.RefreshWatchlists(ctx, watchlists, secrets)
}

func (s *Service) RefreshWatchlists(ctx context.Context, watchlists []Watchlist, secrets engine.SecretProvider) (RefreshResult, error) {
	now := time.Now().UTC()
	result := RefreshResult{
		Watchlists:      make([]Watchlist, 0, len(watchlists)),
		Dossiers:        make([]Dossier, 0, len(watchlists)),
		Changes:         []ChangeRecord{},
		Recommendations: []Recommendation{},
	}
	trustedDomains := trustedDomainsFromSecrets(secrets)
	for _, watchlist := range watchlists {
		normalizeWatchlist(&watchlist, trustedDomains, now)
		generated, err := s.GenerateDossier(ctx, GenerateRequest{
			WatchlistID:    watchlist.ID,
			Name:           watchlist.Name,
			Topic:          watchlist.Topic,
			Company:        watchlist.Company,
			Competitors:    watchlist.Competitors,
			Domains:        watchlist.Domains,
			Pages:          watchlist.Pages,
			Market:         watchlist.Market,
			FocusAreas:     watchlist.FocusAreas,
			TrustedDomains: watchlist.TrustedDomains,
		}, secrets)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", firstNonEmpty(watchlist.Name, watchlist.Topic, watchlist.ID), err))
			continue
		}
		result.Watchlists = append(result.Watchlists, generated.Watchlist)
		result.Dossiers = append(result.Dossiers, generated.Dossier)
		result.Changes = append(result.Changes, generated.Changes...)
		result.Recommendations = append(result.Recommendations, generated.Recommendations...)
	}
	if len(result.Dossiers) == 0 && len(result.Warnings) > 0 {
		return result, fmt.Errorf("watchlist refresh completed without usable dossiers")
	}
	return result, nil
}

func (s *Service) GenerateDossier(ctx context.Context, req GenerateRequest, secrets engine.SecretProvider) (GeneratedDossier, error) {
	now := time.Now().UTC()
	watchlist := Watchlist{
		ID:             req.WatchlistID,
		Name:           req.Name,
		Topic:          req.Topic,
		Company:        req.Company,
		Competitors:    dedupeStrings(req.Competitors),
		Domains:        dedupeStrings(req.Domains),
		Pages:          dedupeStrings(req.Pages),
		Market:         strings.TrimSpace(req.Market),
		FocusAreas:     dedupeStrings(req.FocusAreas),
		TrustedDomains: dedupeStrings(req.TrustedDomains),
	}
	normalizeWatchlist(&watchlist, trustedDomainsFromSecrets(secrets), now)

	analysis, err := s.research.Analyze(ctx, research.AnalyzeRequest{
		Company:     watchlist.Company,
		Competitors: watchlist.Competitors,
		Domains:     watchlist.Domains,
		Pages:       watchlist.Pages,
		FocusAreas:  watchlist.FocusAreas,
		Market:      watchlist.Market,
		Provider:    strings.TrimSpace(req.Provider),
		Debug:       req.Debug,
	}, secrets)
	if err != nil {
		return GeneratedDossier{}, err
	}

	dossierID := newID("dossier", watchlist.ID)
	evidence := buildEvidenceRecords(dossierID, watchlist, analysis, now)
	previousEvidence, err := s.latestEvidenceByWatchlist(ctx, watchlist.ID)
	if err != nil {
		return GeneratedDossier{}, err
	}
	changes := diffEvidence(previousEvidence, evidence, watchlist.ID, dossierID, now)
	recommendations := buildRecommendations(watchlist, dossierID, analysis, evidence, changes, now)

	dossier := Dossier{
		ID:                dossierID,
		WatchlistID:       watchlist.ID,
		Topic:             firstNonEmpty(watchlist.Topic, watchlist.Name),
		Company:           watchlist.Company,
		Summary:           summarizeDossier(analysis, changes),
		Provider:          analysis.Provider,
		FallbackReason:    analysis.FallbackReason,
		Coverage:          analysis.Coverage,
		Findings:          append([]string(nil), analysis.Findings...),
		CompetitorNotes:   append([]research.CompetitorNote(nil), analysis.CompetitorNotes...),
		Warnings:          append([]string(nil), analysis.Warnings...),
		EvidenceIDs:       collectEvidenceIDs(evidence),
		ChangeIDs:         collectChangeIDs(changes),
		RecommendationIDs: collectRecommendationIDs(recommendations),
		CreatedAt:         now,
	}

	watchlist.UpdatedAt = now
	watchlist.LastRunAt = &now
	watchlist.LastDossierID = dossier.ID

	if err := saveRecord(filepath.Join(s.root, "watchlists"), watchlist.ID, watchlist); err != nil {
		return GeneratedDossier{}, err
	}
	if err := saveRecord(filepath.Join(s.root, "dossiers"), dossier.ID, dossier); err != nil {
		return GeneratedDossier{}, err
	}
	for _, item := range evidence {
		if err := saveRecord(filepath.Join(s.root, "evidence"), item.ID, item); err != nil {
			return GeneratedDossier{}, err
		}
	}
	for _, item := range changes {
		if err := saveRecord(filepath.Join(s.root, "changes"), item.ID, item); err != nil {
			return GeneratedDossier{}, err
		}
	}
	for _, item := range recommendations {
		if err := saveRecord(filepath.Join(s.root, "recommendations"), item.ID, item); err != nil {
			return GeneratedDossier{}, err
		}
	}

	return GeneratedDossier{
		Watchlist:       watchlist,
		Dossier:         dossier,
		Evidence:        evidence,
		Changes:         changes,
		Recommendations: recommendations,
	}, nil
}

func (s *Service) DiffLatest(ctx context.Context, watchlistID string) (DiffView, error) {
	dossiers, err := s.ListDossiers(ctx, 32)
	if err != nil {
		return DiffView{}, err
	}
	filtered := make([]Dossier, 0, len(dossiers))
	for _, dossier := range dossiers {
		if watchlistID != "" && dossier.WatchlistID != watchlistID {
			continue
		}
		filtered = append(filtered, dossier)
	}
	if len(filtered) == 0 {
		return DiffView{}, engine.ErrNotFound
	}
	changes, err := s.ListChanges(ctx, firstNonEmpty(watchlistID, filtered[0].WatchlistID), 16)
	if err != nil {
		return DiffView{}, err
	}
	diff := DiffView{
		WatchlistID: filtered[0].WatchlistID,
		DossierID:   filtered[0].ID,
		Summary:     summarizeChangeSet(changes),
		Changes:     changes,
	}
	if diff.Summary == "" {
		diff.Summary = "No significant changes were detected in the latest dossier."
	}
	return diff, nil
}

func (s *Service) GetRecommendation(_ context.Context, id string) (Recommendation, error) {
	return loadRecord[Recommendation](filepath.Join(s.root, "recommendations"), id)
}

func (s *Service) UpdateRecommendation(_ context.Context, id string, update RecommendationUpdate) (Recommendation, error) {
	recommendation, err := loadRecord[Recommendation](filepath.Join(s.root, "recommendations"), id)
	if err != nil {
		return Recommendation{}, err
	}
	if strings.TrimSpace(update.Title) != "" {
		recommendation.Title = strings.TrimSpace(update.Title)
	}
	if strings.TrimSpace(update.Summary) != "" {
		recommendation.Summary = strings.TrimSpace(update.Summary)
	}
	if update.Confidence != nil {
		recommendation.Confidence = *update.Confidence
	}
	if strings.TrimSpace(update.ApprovalStatus) != "" {
		recommendation.ApprovalStatus = strings.TrimSpace(update.ApprovalStatus)
	}
	if update.ProposedWorkflow != nil {
		recommendation.ProposedWorkflow = *update.ProposedWorkflow
	}
	recommendation.UpdatedAt = time.Now().UTC()
	if err := saveRecord(filepath.Join(s.root, "recommendations"), recommendation.ID, recommendation); err != nil {
		return Recommendation{}, err
	}
	return recommendation, nil
}

func (s *Service) MarkRecommendationQueued(_ context.Context, id string, workflowID string) (Recommendation, error) {
	recommendation, err := loadRecord[Recommendation](filepath.Join(s.root, "recommendations"), id)
	if err != nil {
		return Recommendation{}, err
	}
	recommendation.Status = RecommendationSubmitted
	recommendation.QueuedWorkflowID = strings.TrimSpace(workflowID)
	recommendation.UpdatedAt = time.Now().UTC()
	if err := saveRecord(filepath.Join(s.root, "recommendations"), recommendation.ID, recommendation); err != nil {
		return Recommendation{}, err
	}
	return recommendation, nil
}

func (s *Service) DiscardRecommendation(_ context.Context, id string) (Recommendation, error) {
	recommendation, err := loadRecord[Recommendation](filepath.Join(s.root, "recommendations"), id)
	if err != nil {
		return Recommendation{}, err
	}
	recommendation.Status = RecommendationDiscarded
	recommendation.UpdatedAt = time.Now().UTC()
	if err := saveRecord(filepath.Join(s.root, "recommendations"), recommendation.ID, recommendation); err != nil {
		return Recommendation{}, err
	}
	return recommendation, nil
}

func (s *Service) latestEvidenceByWatchlist(ctx context.Context, watchlistID string) (map[string]EvidenceRecord, error) {
	if strings.TrimSpace(watchlistID) == "" {
		return map[string]EvidenceRecord{}, nil
	}
	dossiers, err := s.ListDossiers(ctx, 32)
	if err != nil {
		return nil, err
	}
	var latestID string
	for _, dossier := range dossiers {
		if dossier.WatchlistID == watchlistID {
			latestID = dossier.ID
			break
		}
	}
	if latestID == "" {
		return map[string]EvidenceRecord{}, nil
	}
	evidence, err := s.ListEvidence(ctx, latestID, 64)
	if err != nil {
		return nil, err
	}
	records := make(map[string]EvidenceRecord, len(evidence))
	for _, item := range evidence {
		records[item.SourceURL] = item
	}
	return records, nil
}

func watchlistsFromSecrets(secrets engine.SecretProvider) ([]Watchlist, error) {
	if secrets == nil {
		return nil, nil
	}
	trusted := trustedDomainsFromSecrets(secrets)
	raw, _ := secrets.Get("research_watchlists")
	return ParseWatchlists(raw, trusted)
}

func trustedDomainsFromSecrets(secrets engine.SecretProvider) []string {
	if secrets == nil {
		return nil
	}
	raw, _ := secrets.Get("trusted_domains")
	return ParseTrustedDomains(raw)
}

func normalizeWatchlist(watchlist *Watchlist, defaultTrusted []string, now time.Time) {
	if watchlist == nil {
		return
	}
	watchlist.Name = strings.TrimSpace(watchlist.Name)
	watchlist.Topic = strings.TrimSpace(watchlist.Topic)
	watchlist.Company = strings.TrimSpace(watchlist.Company)
	watchlist.Market = strings.TrimSpace(watchlist.Market)
	watchlist.Competitors = dedupeStrings(watchlist.Competitors)
	watchlist.Domains = dedupeStrings(watchlist.Domains)
	watchlist.Pages = dedupeStrings(watchlist.Pages)
	watchlist.FocusAreas = dedupeStrings(watchlist.FocusAreas)
	watchlist.TrustedDomains = dedupeStrings(append(watchlist.TrustedDomains, defaultTrusted...))
	if watchlist.Name == "" {
		watchlist.Name = firstNonEmpty(watchlist.Topic, firstNonEmpty(watchlist.Competitors...), firstNonEmpty(watchlist.Domains...), firstNonEmpty(watchlist.Pages...))
	}
	if watchlist.Topic == "" {
		watchlist.Topic = firstNonEmpty(watchlist.Name, watchlist.Company, firstNonEmpty(watchlist.Competitors...))
	}
	if watchlist.Company == "" {
		watchlist.Company = firstNonEmpty(watchlist.Topic, watchlist.Name)
	}
	if len(watchlist.FocusAreas) == 0 {
		watchlist.FocusAreas = []string{"pricing", "positioning", "offers"}
	}
	if watchlist.ID == "" {
		watchlist.ID = slug(firstNonEmpty(watchlist.Name, watchlist.Topic, watchlist.Company))
	}
	if watchlist.CreatedAt.IsZero() {
		watchlist.CreatedAt = now
	}
	watchlist.UpdatedAt = now
}

func buildEvidenceRecords(dossierID string, watchlist Watchlist, analysis research.Result, now time.Time) []EvidenceRecord {
	items := make([]EvidenceRecord, 0, len(analysis.Sources))
	for index, source := range analysis.Sources {
		claim := firstNonEmpty(source.Excerpt, source.Description, firstNonEmpty(analysis.Findings...))
		entity := firstNonEmpty(source.Competitor, watchlist.Topic, watchlist.Name)
		record := EvidenceRecord{
			ID:          newID("evidence", fmt.Sprintf("%s-%d", dossierID, index)),
			WatchlistID: watchlist.ID,
			DossierID:   dossierID,
			Entity:      entity,
			EntityType:  "page",
			SourceURL:   source.URL,
			Host:        source.Host,
			Title:       source.Title,
			PageType:    source.PageType,
			Excerpt:     source.Excerpt,
			Claim:       claim,
			Provider:    analysis.Provider,
			Query:       source.Query,
			ObservedAt:  now,
			Fingerprint: fingerprint(source.URL, source.Title, source.PageType, claim),
		}
		items = append(items, record)
	}
	return items
}

func diffEvidence(previous map[string]EvidenceRecord, current []EvidenceRecord, watchlistID string, dossierID string, now time.Time) []ChangeRecord {
	changes := make([]ChangeRecord, 0)
	currentByURL := make(map[string]EvidenceRecord, len(current))
	for _, item := range current {
		currentByURL[item.SourceURL] = item
		previousItem, exists := previous[item.SourceURL]
		switch {
		case !exists:
			changes = append(changes, ChangeRecord{
				ID:          newID("change", item.ID+"-added"),
				WatchlistID: watchlistID,
				DossierID:   dossierID,
				Entity:      item.Entity,
				SourceURL:   item.SourceURL,
				Kind:        "added",
				Summary:     fmt.Sprintf("Observed a new %s page for %s.", firstNonEmpty(item.PageType, "research"), firstNonEmpty(item.Entity, "the topic")),
				EvidenceID:  item.ID,
				ObservedAt:  now,
			})
		case previousItem.Fingerprint != item.Fingerprint:
			changes = append(changes, ChangeRecord{
				ID:                 newID("change", item.ID+"-modified"),
				WatchlistID:        watchlistID,
				DossierID:          dossierID,
				Entity:             item.Entity,
				SourceURL:          item.SourceURL,
				Kind:               "modified",
				Summary:            fmt.Sprintf("Updated evidence detected on %s.", firstNonEmpty(item.Title, item.SourceURL)),
				EvidenceID:         item.ID,
				PreviousEvidenceID: previousItem.ID,
				ObservedAt:         now,
			})
		}
	}
	for sourceURL, previousItem := range previous {
		if _, ok := currentByURL[sourceURL]; ok {
			continue
		}
		changes = append(changes, ChangeRecord{
			ID:                 newID("change", previousItem.ID+"-removed"),
			WatchlistID:        watchlistID,
			DossierID:          dossierID,
			Entity:             previousItem.Entity,
			SourceURL:          previousItem.SourceURL,
			Kind:               "removed",
			Summary:            fmt.Sprintf("Previously observed evidence disappeared from %s.", firstNonEmpty(previousItem.Title, previousItem.SourceURL)),
			PreviousEvidenceID: previousItem.ID,
			ObservedAt:         now,
		})
	}
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Summary < changes[j].Summary
	})
	return changes
}

func buildRecommendations(watchlist Watchlist, dossierID string, analysis research.Result, evidence []EvidenceRecord, changes []ChangeRecord, now time.Time) []Recommendation {
	sourceURLs := collectSourceURLs(evidence)
	evidenceIDs := collectEvidenceIDs(evidence)
	approvalStatus := "report_only"
	if len(changes) > 0 {
		approvalStatus = "queue_ready"
	}

	briefContent := buildEvidenceBundle(watchlist, analysis, evidence, changes)
	recommendations := []Recommendation{
		{
			ID:             newID("recommendation", dossierID+"-creative"),
			WatchlistID:    watchlist.ID,
			DossierID:      dossierID,
			Topic:          firstNonEmpty(watchlist.Topic, watchlist.Name),
			Title:          fmt.Sprintf("Draft a response angle for %s", firstNonEmpty(watchlist.Topic, watchlist.Name)),
			Summary:        fmt.Sprintf("Use the grounded findings from %d source%s to prepare a market response angle.", len(evidence), pluralSuffix(len(evidence))),
			Confidence:     recommendationConfidence(evidence, changes, 0.72),
			EvidenceIDs:    evidenceIDs,
			SourceURLs:     sourceURLs,
			ApprovalStatus: approvalStatus,
			Status:         RecommendationDraft,
			Provider:       analysis.Provider,
			ProposedWorkflow: engine.WorkflowDefinition{
				Name:  "Dossier response angle",
				Skill: "mitpo-creative-director",
				Input: map[string]any{
					"brand_name":   firstNonEmpty(watchlist.Company, watchlist.Topic, watchlist.Name),
					"tone":         "confident, evidence-backed",
					"audience":     "marketing operators",
					"content_type": "campaign response angle",
					"guidelines":   briefContent,
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:             newID("recommendation", dossierID+"-export"),
			WatchlistID:    watchlist.ID,
			DossierID:      dossierID,
			Topic:          firstNonEmpty(watchlist.Topic, watchlist.Name),
			Title:          fmt.Sprintf("Export an evidence brief for %s", firstNonEmpty(watchlist.Topic, watchlist.Name)),
			Summary:        "Persist the latest dossier and evidence bundle into workspace exports for review and sharing.",
			Confidence:     recommendationConfidence(evidence, changes, 0.9),
			EvidenceIDs:    evidenceIDs,
			SourceURLs:     sourceURLs,
			ApprovalStatus: "report_only",
			Status:         RecommendationDraft,
			Provider:       analysis.Provider,
			ProposedWorkflow: engine.WorkflowDefinition{
				Name:  "Export dossier evidence brief",
				Skill: "mitpo-markdown-export",
				Input: map[string]any{
					"title":    fmt.Sprintf("%s Dossier", firstNonEmpty(watchlist.Topic, watchlist.Name)),
					"filename": "dossier-evidence",
					"content":  briefContent,
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	if len(evidence) > 0 {
		recommendations = append(recommendations, Recommendation{
			ID:             newID("recommendation", dossierID+"-seo"),
			WatchlistID:    watchlist.ID,
			DossierID:      dossierID,
			Topic:          firstNonEmpty(watchlist.Topic, watchlist.Name),
			Title:          fmt.Sprintf("Audit a tracked page against the latest dossier for %s", firstNonEmpty(watchlist.Topic, watchlist.Name)),
			Summary:        "Run a bounded page audit on one tracked source to validate messaging and search visibility against the observed competitor movement.",
			Confidence:     recommendationConfidence(evidence, changes, 0.58),
			EvidenceIDs:    evidenceIDs[:min(3, len(evidenceIDs))],
			SourceURLs:     sourceURLs[:min(3, len(sourceURLs))],
			ApprovalStatus: "report_only",
			Status:         RecommendationDraft,
			Provider:       analysis.Provider,
			ProposedWorkflow: engine.WorkflowDefinition{
				Name:  "Audit tracked dossier page",
				Skill: "mitpo-seo-auditor",
				Input: map[string]any{
					"url":      sourceURLs[0],
					"keywords": dedupeStrings(append(watchlist.FocusAreas, watchlist.Competitors...)),
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	return recommendations
}

func summarizeDossier(analysis research.Result, changes []ChangeRecord) string {
	summary := strings.TrimSpace(analysis.Summary)
	changeSummary := summarizeChangeSet(changes)
	switch {
	case summary == "":
		return firstNonEmpty(changeSummary, "Dossier generated from bounded research.")
	case changeSummary == "":
		return summary
	default:
		return summary + " " + changeSummary
	}
}

func summarizeChangeSet(changes []ChangeRecord) string {
	if len(changes) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, change := range changes {
		counts[change.Kind]++
	}
	parts := make([]string, 0, len(counts))
	if counts["added"] > 0 {
		parts = append(parts, fmt.Sprintf("%d new", counts["added"]))
	}
	if counts["modified"] > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", counts["modified"]))
	}
	if counts["removed"] > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", counts["removed"]))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Detected " + strings.Join(parts, ", ") + " evidence items since the previous run."
}

func buildEvidenceBundle(watchlist Watchlist, analysis research.Result, evidence []EvidenceRecord, changes []ChangeRecord) string {
	var builder strings.Builder
	builder.WriteString("Scenario: ")
	builder.WriteString(firstNonEmpty(watchlist.Topic, watchlist.Name))
	builder.WriteString("\n\nSummary\n")
	builder.WriteString(firstNonEmpty(analysis.Summary, "No summary available."))
	builder.WriteString("\n\nFindings\n")
	for _, finding := range analysis.Findings {
		builder.WriteString("- ")
		builder.WriteString(strings.TrimSpace(finding))
		builder.WriteString("\n")
	}
	if len(changes) > 0 {
		builder.WriteString("\nDetected changes\n")
		for _, change := range changes {
			builder.WriteString("- ")
			builder.WriteString(change.Summary)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nEvidence\n")
	for _, item := range evidence {
		builder.WriteString("- ")
		builder.WriteString(firstNonEmpty(item.Title, item.SourceURL))
		builder.WriteString(": ")
		builder.WriteString(firstNonEmpty(item.Claim, item.Excerpt))
		builder.WriteString(" (")
		builder.WriteString(item.SourceURL)
		builder.WriteString(")\n")
	}
	return strings.TrimSpace(builder.String())
}

func collectEvidenceIDs(items []EvidenceRecord) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func collectChangeIDs(items []ChangeRecord) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func collectRecommendationIDs(items []Recommendation) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func collectSourceURLs(items []EvidenceRecord) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.SourceURL)
	}
	return dedupeStrings(out)
}

func recommendationConfidence(evidence []EvidenceRecord, changes []ChangeRecord, base float64) float64 {
	score := base + float64(min(3, len(evidence)))*0.05 + float64(min(2, len(changes)))*0.04
	if score > 0.98 {
		score = 0.98
	}
	return score
}

func listRecords[T any](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	items := make([]T, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}
		item, err := loadRecord[T](dir, strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func loadRecord[T any](dir string, id string) (T, error) {
	var item T
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return item, err
	}
	if err := json.Unmarshal(data, &item); err != nil {
		return item, err
	}
	return item, nil
}

func saveRecord(dir string, id string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, id+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func limitRecords[T any](items []T, limit int) []T {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return append([]T(nil), items[:limit]...)
}

func fingerprint(parts ...string) string {
	hash := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:])
}

func newID(prefix string, value string) string {
	if strings.TrimSpace(value) == "" {
		value = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return prefix + "-" + slug(value) + "-" + time.Now().UTC().Format("20060102150405")
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "item"
	}
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "item"
	}
	return result
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
