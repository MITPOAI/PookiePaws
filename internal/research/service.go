package research

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mitpoai/pookiepaws/internal/conv"
	"github.com/mitpoai/pookiepaws/internal/engine"
)

const (
	defaultFirecrawlBaseURL      = "https://api.firecrawl.dev"
	defaultJinaBaseURL           = "https://r.jina.ai"
	defaultInternalSearchBaseURL = "https://html.duckduckgo.com/html/"

	envFirecrawlBaseURL      = "POOKIEPAWS_FIRECRAWL_BASE_URL"
	envJinaBaseURL           = "POOKIEPAWS_JINA_BASE_URL"
	envInternalSearchBaseURL = "POOKIEPAWS_INTERNAL_SEARCH_BASE_URL"
)

type AnalyzeRequest struct {
	Company     string
	Competitors []string
	Domains     []string
	Pages       []string
	FocusAreas  []string
	Market      string
	Country     string
	Location    string
	MaxSources  int
	Provider    string
	Debug       bool
}

type CompetitorNote struct {
	Competitor  string   `json:"competitor"`
	Note        string   `json:"note"`
	Highlights  []string `json:"highlights,omitempty"`
	SourceCount int      `json:"source_count"`
}

type Source struct {
	URL         string `json:"url"`
	Host        string `json:"host"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Competitor  string `json:"competitor,omitempty"`
	PageType    string `json:"page_type,omitempty"`
	Query       string `json:"query,omitempty"`
	Excerpt     string `json:"excerpt,omitempty"`
	Markdown    string `json:"markdown,omitempty"`
}

type Coverage struct {
	Mode       string `json:"mode"`
	Queries    int    `json:"queries"`
	Discovered int    `json:"discovered"`
	Scraped    int    `json:"scraped"`
	Kept       int    `json:"kept"`
	Skipped    int    `json:"skipped"`
}

type Result struct {
	Summary         string           `json:"summary"`
	Findings        []string         `json:"findings"`
	CompetitorNotes []CompetitorNote `json:"competitor_notes"`
	Sources         []Source         `json:"sources"`
	Warnings        []string         `json:"warnings,omitempty"`
	Coverage        Coverage         `json:"coverage"`
	Provider        string           `json:"provider,omitempty"`
	FallbackReason  string           `json:"fallback_reason,omitempty"`
}

type Service struct {
	client           *http.Client
	firecrawlBaseURL string
	jinaBaseURL      string
	searchBaseURL    string
	robotsMu         sync.Mutex
	robotsCache      map[string]robotsPolicy
}

type firecrawlSearchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Web []struct {
			Title       string         `json:"title"`
			Description string         `json:"description"`
			URL         string         `json:"url"`
			Markdown    string         `json:"markdown"`
			Metadata    map[string]any `json:"metadata"`
		} `json:"web"`
	} `json:"data"`
	Warning string `json:"warning"`
}

type searchCandidate struct {
	Source
	Markdown string
	score    int
}

func NewService() *Service {
	firecrawlBaseURL := strings.TrimSpace(os.Getenv(envFirecrawlBaseURL))
	if firecrawlBaseURL == "" {
		firecrawlBaseURL = defaultFirecrawlBaseURL
	}
	jinaBaseURL := strings.TrimSpace(os.Getenv(envJinaBaseURL))
	if jinaBaseURL == "" {
		jinaBaseURL = defaultJinaBaseURL
	}
	searchBaseURL := strings.TrimSpace(os.Getenv(envInternalSearchBaseURL))
	if searchBaseURL == "" {
		searchBaseURL = defaultInternalSearchBaseURL
	}
	return &Service{
		client:           &http.Client{Timeout: 30 * time.Second},
		firecrawlBaseURL: strings.TrimRight(firecrawlBaseURL, "/"),
		jinaBaseURL:      strings.TrimRight(jinaBaseURL, "/"),
		searchBaseURL:    strings.TrimRight(searchBaseURL, "/"),
		robotsCache:      map[string]robotsPolicy{},
	}
}

func (s *Service) WithHTTPClient(client *http.Client) *Service {
	if s != nil && client != nil {
		s.client = client
	}
	return s
}

func (s *Service) Analyze(ctx context.Context, req AnalyzeRequest, secrets engine.SecretProvider) (Result, error) {
	normalized := normalizeRequest(req)
	competitors := normalized.competitors()
	if len(competitors) == 0 {
		return Result{}, fmt.Errorf("bounded research requires at least one competitor or company")
	}
	if usesFixtureDomains(normalized.Domains) {
		result := fixtureResult(normalized)
		result.Provider = researchProviderFixture
		return result, nil
	}
	return s.analyzeWithRouting(ctx, normalized, secrets)
}

func (s *Service) analyzeLive(ctx context.Context, req AnalyzeRequest, firecrawlKey string, secrets engine.SecretProvider) (Result, error) {
	queries := buildQueries(req)
	candidates := make([]searchCandidate, 0, len(queries)*5+len(req.Pages))
	warnings := make([]string, 0)
	discovered := 0

	if len(req.Pages) > 0 {
		pageCandidates, pageWarnings := discoverExplicitPages(req)
		candidates = append(candidates, pageCandidates...)
		warnings = append(warnings, pageWarnings...)
		discovered += len(pageCandidates)
	}

	perQueryTimeout := time.Duration(20) * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < perQueryTimeout {
			perQueryTimeout = remaining
		}
	}
	if perQueryTimeout <= 0 {
		perQueryTimeout = 5 * time.Second
	}

	for _, query := range queries {
		queryCtx, cancel := context.WithTimeout(ctx, perQueryTimeout)
		response, err := s.firecrawlSearch(queryCtx, firecrawlKey, query, req)
		cancel()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Search query %q failed: %v", query, err))
			continue
		}
		if strings.TrimSpace(response.Warning) != "" {
			warnings = append(warnings, strings.TrimSpace(response.Warning))
		}
		for _, item := range response.Data.Web {
			discovered++
			candidate := searchCandidate{
				Source: Source{
					URL:         strings.TrimSpace(item.URL),
					Title:       strings.TrimSpace(item.Title),
					Description: strings.TrimSpace(item.Description),
					Query:       query,
					Competitor:  detectCompetitor(query, req),
				},
				Markdown: strings.TrimSpace(item.Markdown),
			}
			if candidate.Source.URL == "" {
				continue
			}
			if sourceURL := strings.TrimSpace(conv.AsString(item.Metadata["sourceURL"])); sourceURL != "" {
				candidate.Source.URL = sourceURL
			}
			candidate.Source.Host = hostname(candidate.Source.URL)
			candidate.Source.PageType = detectPageType(candidate.Source.URL, candidate.Source.Title)
			candidate.score = scoreCandidate(candidate, req)
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		return Result{}, fmt.Errorf("live research did not return any usable public pages")
	}

	selected, skipped, selectionWarnings := selectCandidates(candidates, req.MaxSources)
	warnings = append(warnings, selectionWarnings...)

	scraped := 0
	for i := range selected {
		if strings.TrimSpace(selected[i].Markdown) == "" {
			markdown, err := s.fetchJina(ctx, selected[i].URL, secrets)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Could not read %s: %v", selected[i].URL, err))
			} else {
				selected[i].Markdown = markdown
			}
		}
		if strings.TrimSpace(selected[i].Markdown) != "" {
			scraped++
		}
	}

	result := buildResult(req, selected, warnings)
	result.Coverage = Coverage{
		Mode:       "live",
		Queries:    len(queries),
		Discovered: discovered,
		Scraped:    scraped,
		Kept:       len(selected),
		Skipped:    skipped,
	}
	return result, nil
}

func (s *Service) analyzeSeedDomains(ctx context.Context, req AnalyzeRequest, secrets engine.SecretProvider) (Result, error) {
	candidates := make([]searchCandidate, 0, len(req.Domains)+len(req.Pages))
	warnings := make([]string, 0)
	if len(req.Pages) > 0 {
		for _, candidate := range buildSeedPageCandidates(req) {
			markdown, err := s.fetchJina(ctx, candidate.URL, secrets)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Could not read %s: %v", candidate.URL, err))
				continue
			}
			candidate.Markdown = markdown
			candidate.score = scoreCandidate(candidate, req)
			candidates = append(candidates, candidate)
		}
	}
	for _, domain := range req.Domains {
		targetURL, err := normalizeSeedURL(domain)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Skipped seed domain %q: %v", domain, err))
			continue
		}
		markdown, err := s.fetchJina(ctx, targetURL, secrets)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not read %s: %v", targetURL, err))
			continue
		}
		candidate := searchCandidate{
			Source: Source{
				URL:         targetURL,
				Host:        hostname(targetURL),
				Title:       targetURL,
				Competitor:  closestCompetitor(hostname(targetURL), req.competitors()),
				PageType:    "homepage",
				Description: "Seed-domain fallback page",
			},
			Markdown: markdown,
		}
		candidate.score = scoreCandidate(candidate, req)
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return Result{}, fmt.Errorf("seed-domain fallback could not read any public pages")
	}
	selected, skipped, selectionWarnings := selectCandidates(candidates, req.MaxSources)
	warnings = append(warnings, selectionWarnings...)
	result := buildResult(req, selected, warnings)
	result.Coverage = Coverage{
		Mode:       "seed_domains",
		Queries:    len(req.Domains),
		Discovered: len(candidates),
		Scraped:    len(candidates),
		Kept:       len(selected),
		Skipped:    skipped,
	}
	return result, nil
}

func buildSeedPageCandidates(req AnalyzeRequest) []searchCandidate {
	candidates, _ := discoverExplicitPages(req)
	return candidates
}

func (s *Service) firecrawlSearch(ctx context.Context, apiKey, query string, req AnalyzeRequest) (firecrawlSearchResponse, error) {
	payload := map[string]any{
		"query":    query,
		"limit":    5,
		"sources":  []string{"web"},
		"country":  req.Country,
		"location": req.Location,
		"timeout":  60000,
		"scrapeOptions": map[string]any{
			"formats": []string{"markdown"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return firecrawlSearchResponse{}, err
	}
	endpoint := s.firecrawlBaseURL + "/v2/search"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return firecrawlSearchResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)

	response, err := s.client.Do(request)
	if err != nil {
		return firecrawlSearchResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return firecrawlSearchResponse{}, fmt.Errorf("firecrawl search returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(raw)))
	}
	var decoded firecrawlSearchResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&decoded); err != nil {
		return firecrawlSearchResponse{}, err
	}
	return decoded, nil
}

func (s *Service) fetchJina(ctx context.Context, targetURL string, secrets engine.SecretProvider) (string, error) {
	if err := validatePublicURL(targetURL); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.jinaBaseURL+"/"+targetURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "text/markdown")
	request.Header.Set("User-Agent", "PookiePaws/1.1 (marketing-research-bot)")
	if secrets != nil {
		if apiKey, _ := secrets.Get("jina_api_key"); strings.TrimSpace(apiKey) != "" {
			request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
		}
	}
	response, err := s.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("jina returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func buildResult(req AnalyzeRequest, selected []searchCandidate, warnings []string) Result {
	findings := make([]string, 0, len(selected))
	sources := make([]Source, 0, len(selected))
	byCompetitor := map[string][]searchCandidate{}

	for _, candidate := range selected {
		byCompetitor[candidate.Competitor] = append(byCompetitor[candidate.Competitor], candidate)
		excerpt := excerptForCandidate(candidate, req.FocusAreas)
		finding := fmt.Sprintf("%s: %s page on %s", firstValue(candidate.Competitor, "Research"), candidate.Source.PageType, candidate.Source.Host)
		if excerpt != "" {
			finding += " highlights " + excerpt
		}
		findings = append(findings, finding)

		source := candidate.Source
		source.Excerpt = excerpt
		if req.Debug {
			source.Markdown = candidate.Markdown
		}
		sources = append(sources, source)
	}

	competitors := req.competitors()
	notes := make([]CompetitorNote, 0, len(competitors))
	for _, competitor := range competitors {
		group := byCompetitor[competitor]
		if len(group) == 0 {
			continue
		}
		types := uniquePageTypes(group)
		highlights := collectHighlights(group, req.FocusAreas)
		note := CompetitorNote{
			Competitor:  competitor,
			SourceCount: len(group),
			Highlights:  highlights,
			Note: fmt.Sprintf(
				"%s coverage leans on %s pages across %d public sources.",
				competitor,
				strings.Join(types, ", "),
				len(group),
			),
		}
		notes = append(notes, note)
	}

	summary := fmt.Sprintf(
		"Collected %d bounded public sources across %d competitor%s for %s.",
		len(sources),
		len(notes),
		pluralSuffix(len(notes)),
		firstValue(req.Company, firstValue(req.competitors()...)),
	)
	if len(notes) > 0 {
		summary += " " + notes[0].Note
	}

	return Result{
		Summary:         summary,
		Findings:        findings,
		CompetitorNotes: notes,
		Sources:         sources,
		Warnings:        dedupeStrings(warnings),
	}
}

func normalizeRequest(req AnalyzeRequest) AnalyzeRequest {
	req.Company = strings.TrimSpace(req.Company)
	req.Market = strings.TrimSpace(req.Market)
	req.Country = strings.TrimSpace(req.Country)
	if req.Country == "" {
		req.Country = "AU"
	}
	req.Location = strings.TrimSpace(req.Location)
	if req.Location == "" {
		req.Location = "Australia"
	}
	if req.MaxSources <= 0 || req.MaxSources > 6 {
		req.MaxSources = 6
	}
	req.Provider = normalizeResearchProvider(req.Provider)
	req.Competitors = dedupeStrings(req.Competitors)
	req.Domains = dedupeStrings(req.Domains)
	req.Pages = dedupeStrings(req.Pages)
	req.FocusAreas = dedupeStrings(req.FocusAreas)
	if len(req.FocusAreas) == 0 {
		req.FocusAreas = []string{"pricing", "positioning", "tone", "offer structure"}
	}
	return req
}

func fixtureResult(req AnalyzeRequest) Result {
	competitor := firstValue(req.competitors()...)
	sources := make([]Source, 0, len(req.Domains))
	findings := make([]string, 0, len(req.Domains))
	for _, domain := range req.Domains {
		host := hostname("https://" + strings.TrimPrefix(domain, "https://"))
		source := Source{
			URL:         "https://" + strings.TrimPrefix(domain, "https://"),
			Host:        host,
			Title:       strings.TrimSuffix(host, ".example"),
			Competitor:  competitor,
			PageType:    "fixture",
			Description: "Deterministic scenario fixture",
			Excerpt:     "Fixture coverage keeps the scenario smoke offline and predictable.",
		}
		sources = append(sources, source)
		findings = append(findings, fmt.Sprintf("%s fixture source %s keeps deterministic smoke coverage offline.", firstValue(source.Competitor, "Research"), source.Host))
	}
	return Result{
		Summary:  fmt.Sprintf("Prepared deterministic offline research fixtures for %s.", firstValue(req.Company, competitor)),
		Findings: findings,
		CompetitorNotes: []CompetitorNote{{
			Competitor:  competitor,
			Note:        fmt.Sprintf("%s stays in offline fixture mode so the deterministic smoke remains fast and stable.", competitor),
			SourceCount: len(sources),
			Highlights:  []string{"Scenario fixtures avoid outbound network access during deterministic smoke runs."},
		}},
		Sources: sources,
		Warnings: []string{
			"Deterministic fixture mode is active because the scenario uses repo-owned .example domains.",
		},
		Coverage: Coverage{
			Mode:       "fixture",
			Queries:    0,
			Discovered: len(sources),
			Scraped:    len(sources),
			Kept:       len(sources),
			Skipped:    0,
		},
	}
}

func (r AnalyzeRequest) competitors() []string {
	if len(r.Competitors) > 0 {
		return r.Competitors
	}
	if r.Company != "" {
		return []string{r.Company}
	}
	return nil
}

func buildQueries(req AnalyzeRequest) []string {
	templates := []string{
		`"%s" pricing %s`,
		`"%s" about %s`,
		`"%s" offers %s`,
	}
	queries := make([]string, 0, len(req.competitors())*len(templates))
	for _, competitor := range req.competitors() {
		for _, template := range templates {
			queries = append(queries, strings.TrimSpace(fmt.Sprintf(template, competitor, req.Market)))
		}
	}
	return dedupeStrings(queries)
}

func detectCompetitor(query string, req AnalyzeRequest) string {
	lowerQuery := strings.ToLower(query)
	for _, competitor := range req.competitors() {
		if strings.Contains(lowerQuery, strings.ToLower(competitor)) {
			return competitor
		}
	}
	return firstValue(req.competitors()...)
}

func scoreCandidate(candidate searchCandidate, req AnalyzeRequest) int {
	if err := validatePublicURL(candidate.URL); err != nil {
		return -1000
	}
	if shouldDropURL(candidate.URL) {
		return -1000
	}

	score := 10
	host := candidate.Source.Host
	pageType := candidate.Source.PageType
	if isOfficialHost(host, candidate.Competitor, req.Domains) {
		score += 50
	}
	if strings.TrimSpace(candidate.Markdown) != "" {
		score += 25
	}
	switch pageType {
	case "pricing":
		score += 30
	case "offer":
		score += 24
	case "about":
		score += 18
	case "faq":
		score += 16
	case "product":
		score += 14
	case "homepage":
		score += 8
	}
	text := strings.ToLower(candidate.Source.Title + " " + candidate.Source.Description + " " + candidate.Source.URL)
	if token := slugToken(candidate.Competitor); token != "" && strings.Contains(text, token) {
		score += 15
	}
	if strings.Contains(text, "review") || strings.Contains(text, "reddit") || strings.Contains(text, "comparison") {
		score -= 12
	}
	return score
}

func selectCandidates(candidates []searchCandidate, maxSources int) ([]searchCandidate, int, []string) {
	filtered := make([]searchCandidate, 0, len(candidates))
	warnings := make([]string, 0)
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate.score < 0 {
			continue
		}
		key := normalizedURL(candidate.URL)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, candidate)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].score == filtered[j].score {
			return filtered[i].URL < filtered[j].URL
		}
		return filtered[i].score > filtered[j].score
	})

	selected := make([]searchCandidate, 0, maxSources)
	perHost := map[string]int{}
	for _, candidate := range filtered {
		if len(selected) >= maxSources {
			break
		}
		host := candidate.Source.Host
		if perHost[host] >= 2 {
			continue
		}
		perHost[host]++
		selected = append(selected, candidate)
	}
	if len(selected) == 0 {
		warnings = append(warnings, "All discovered pages were filtered out by the bounded ranking rules.")
	}
	return selected, len(candidates) - len(selected), warnings
}

func excerptForCandidate(candidate searchCandidate, focusAreas []string) string {
	text := strings.TrimSpace(candidate.Markdown)
	if text == "" {
		text = strings.TrimSpace(candidate.Source.Description)
	}
	if text == "" {
		return ""
	}
	clean := collapseWhitespace(strings.NewReplacer("#", " ", "*", " ", "`", " ").Replace(text))
	lower := strings.ToLower(clean)
	for _, focus := range focusAreas {
		needle := strings.ToLower(strings.TrimSpace(focus))
		if needle == "" {
			continue
		}
		index := strings.Index(lower, needle)
		if index < 0 {
			continue
		}
		start := max(0, index-40)
		end := min(len(clean), index+180)
		return strings.Trim(clean[start:end], " .,;:-")
	}
	if len(clean) > 180 {
		clean = clean[:180]
	}
	return strings.Trim(clean, " .,;:-")
}

func collectHighlights(candidates []searchCandidate, focusAreas []string) []string {
	highlights := make([]string, 0, min(3, len(candidates)))
	for _, candidate := range candidates {
		excerpt := excerptForCandidate(candidate, focusAreas)
		if excerpt == "" {
			continue
		}
		highlights = append(highlights, excerpt)
		if len(highlights) == 3 {
			break
		}
	}
	return highlights
}

func uniquePageTypes(candidates []searchCandidate) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		pageType := firstValue(candidate.Source.PageType, "page")
		if _, ok := seen[pageType]; ok {
			continue
		}
		seen[pageType] = struct{}{}
		result = append(result, pageType)
	}
	return result
}

func detectPageType(rawURL string, title string) string {
	text := strings.ToLower(rawURL + " " + title)
	switch {
	case strings.Contains(text, "pricing") || strings.Contains(text, "price") || strings.Contains(text, "plan"):
		return "pricing"
	case strings.Contains(text, "about"):
		return "about"
	case strings.Contains(text, "faq"):
		return "faq"
	case strings.Contains(text, "collection") || strings.Contains(text, "offer") || strings.Contains(text, "bundle"):
		return "offer"
	case strings.Contains(text, "product") || strings.Contains(text, "shop"):
		return "product"
	default:
		return "homepage"
	}
}

func isOfficialHost(host, competitor string, domains []string) bool {
	lowerHost := strings.ToLower(strings.TrimSpace(host))
	for _, domain := range domains {
		if lowerHost == hostname(domain) {
			return true
		}
	}
	token := slugToken(competitor)
	return token != "" && strings.Contains(lowerHost, token)
}

func shouldDropURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	if strings.HasSuffix(lower, ".pdf") {
		return true
	}
	for _, blocked := range []string{"/login", "/signin", "/cart", "/checkout", "/account"} {
		if strings.Contains(lower, blocked) {
			return true
		}
	}
	return false
}

func normalizeSeedURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("seed domain is empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	if err := validatePublicURL(raw); err != nil {
		return "", err
	}
	parsed, _ := url.Parse(raw)
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

func validatePublicURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("only http and https URLs are supported")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("local hosts are blocked from public research")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("private-network targets are blocked from public research")
		}
	}
	return nil
}

func hostname(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname()))
}

func normalizedURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return strings.TrimSpace(rawURL)
	}
	parsed.Fragment = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}

func closestCompetitor(host string, competitors []string) string {
	for _, competitor := range competitors {
		if token := slugToken(competitor); token != "" && strings.Contains(host, token) {
			return competitor
		}
	}
	return firstValue(competitors...)
}

func slugToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "", "-", "", "_", "").Replace(value)
	return value
}

func firstValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
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
		result = append(result, value)
	}
	return result
}

func usesFixtureDomains(domains []string) bool {
	if len(domains) == 0 {
		return false
	}
	for _, domain := range domains {
		host := hostname("https://" + strings.TrimPrefix(strings.TrimSpace(domain), "https://"))
		if !strings.HasSuffix(host, ".example") {
			return false
		}
	}
	return true
}

func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func safeURLLabel(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	label := parsed.Hostname()
	if parsed.Path != "" && parsed.Path != "/" {
		label += path.Clean(parsed.Path)
	}
	return label
}
