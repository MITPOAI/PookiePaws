package research

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mitpoai/pookiepaws/internal/engine"
	"github.com/mitpoai/pookiepaws/internal/webfetch"
)

const (
	researchProviderAuto      = "auto"
	researchProviderInternal  = "internal"
	researchProviderFirecrawl = "firecrawl"
	researchProviderJina      = "jina"
	researchProviderFixture   = "fixture"
)

type robotsPolicy struct {
	allows    []string
	disallows []string
}

type fetchedDocument struct {
	URL   string
	Title string
	Text  string
	HTML  string
}

type fetchJob struct {
	index     int
	candidate searchCandidate
}

type fetchResult struct {
	index     int
	candidate searchCandidate
	err       error
}

var anchorHrefPattern = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)
var htmlTagPattern = regexp.MustCompile(`(?is)<[^>]+>`)

func normalizeResearchProvider(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case researchProviderInternal:
		return researchProviderInternal
	case researchProviderAuto:
		return researchProviderAuto
	case researchProviderFirecrawl:
		return researchProviderFirecrawl
	case researchProviderJina:
		return researchProviderJina
	default:
		return researchProviderInternal
	}
}

func resolveResearchProvider(req AnalyzeRequest, secrets engine.SecretProvider) string {
	if normalized := normalizeResearchProvider(req.Provider); normalized != "" && strings.TrimSpace(req.Provider) != "" {
		return normalized
	}
	if secrets != nil {
		if value, _ := secrets.Get("research_provider"); strings.TrimSpace(value) != "" {
			return normalizeResearchProvider(value)
		}
	}
	return researchProviderInternal
}

func (s *Service) analyzeWithRouting(ctx context.Context, req AnalyzeRequest, secrets engine.SecretProvider) (Result, error) {
	explicitProvider := strings.TrimSpace(req.Provider) != ""
	preference := resolveResearchProvider(req, secrets)
	switch preference {
	case researchProviderFirecrawl:
		firecrawlKey := ""
		if secrets != nil {
			firecrawlKey, _ = secrets.Get("firecrawl_api_key")
		}
		if strings.TrimSpace(firecrawlKey) == "" {
			return Result{}, fmt.Errorf("research_provider=firecrawl requires firecrawl_api_key")
		}
		result, err := s.analyzeLive(ctx, req, strings.TrimSpace(firecrawlKey), secrets)
		if err != nil {
			return Result{}, err
		}
		result.Provider = researchProviderFirecrawl
		return result, nil
	case researchProviderJina:
		if len(req.Domains) == 0 {
			return Result{}, fmt.Errorf("research_provider=jina requires explicit domains because it cannot discover public URLs by itself")
		}
		result, err := s.analyzeSeedDomains(ctx, req, secrets)
		if err != nil {
			return Result{}, err
		}
		result.Provider = researchProviderJina
		return result, nil
	default:
		internalResult, internalErr := s.analyzeInternal(ctx, req)
		if internalErr == nil && internalResult.Coverage.Kept > 0 {
			internalResult.Provider = researchProviderInternal
			return internalResult, nil
		}
		if explicitProvider && preference == researchProviderInternal {
			if internalErr != nil {
				return Result{}, internalErr
			}
			return Result{}, fmt.Errorf("internal provider did not return any usable public pages")
		}

		firecrawlKey := ""
		if secrets != nil {
			firecrawlKey, _ = secrets.Get("firecrawl_api_key")
		}
		if strings.TrimSpace(firecrawlKey) != "" {
			result, err := s.analyzeLive(ctx, req, strings.TrimSpace(firecrawlKey), secrets)
			if err == nil {
				result.Provider = researchProviderFirecrawl
				result.FallbackReason = firstValue(errorText(internalErr), "internal provider requested external fallback")
				if result.FallbackReason != "" {
					result.Warnings = append(result.Warnings, "External fallback engaged: "+result.FallbackReason)
				}
				return result, nil
			}
		}

		if len(req.Domains) > 0 {
			result, err := s.analyzeSeedDomains(ctx, req, secrets)
			if err == nil {
				result.Provider = researchProviderJina
				result.FallbackReason = firstValue(errorText(internalErr), "internal provider could not complete the run")
				if result.FallbackReason != "" {
					result.Warnings = append(result.Warnings, "Seed-domain fallback engaged: "+result.FallbackReason)
				}
				return result, nil
			}
		}

		if internalErr != nil {
			return Result{}, internalErr
		}
		return Result{}, fmt.Errorf("research run failed without an internal or external fallback path")
	}
}

func (s *Service) analyzeInternal(ctx context.Context, req AnalyzeRequest) (Result, error) {
	candidates := make([]searchCandidate, 0, req.MaxSources*2)
	warnings := make([]string, 0)
	discovered := 0
	queries := buildQueries(req)

	if len(req.Pages) > 0 {
		pageCandidates, pageWarnings := discoverExplicitPages(req)
		candidates = append(candidates, pageCandidates...)
		warnings = append(warnings, pageWarnings...)
		discovered += len(pageCandidates)
	}

	if len(req.Domains) > 0 {
		seedCandidates, seedWarnings := s.discoverFromSeedDomains(ctx, req)
		candidates = append(candidates, seedCandidates...)
		warnings = append(warnings, seedWarnings...)
		discovered += len(seedCandidates)
	}

	for _, query := range queries {
		queryCandidates, queryWarnings := s.discoverFromSearch(ctx, query, req)
		candidates = append(candidates, queryCandidates...)
		warnings = append(warnings, queryWarnings...)
		discovered += len(queryCandidates)
	}

	if len(candidates) == 0 {
		return Result{}, fmt.Errorf("internal provider could not discover any usable public pages")
	}

	selected, skipped, selectionWarnings := selectCandidates(candidates, req.MaxSources)
	warnings = append(warnings, selectionWarnings...)
	if len(selected) == 0 {
		return Result{}, fmt.Errorf("internal provider filtered out all discovered pages")
	}

	scrapedCandidates, scraped, fetchWarnings := s.fetchCandidates(ctx, selected)
	warnings = append(warnings, fetchWarnings...)
	if len(scrapedCandidates) == 0 {
		return Result{}, fmt.Errorf("internal provider could not fetch any selected pages")
	}

	result := buildResult(req, scrapedCandidates, warnings)
	result.Coverage = Coverage{
		Mode:       "internal",
		Queries:    len(queries),
		Discovered: discovered,
		Scraped:    scraped,
		Kept:       len(scrapedCandidates),
		Skipped:    skipped,
	}
	return result, nil
}

func (s *Service) discoverFromSearch(ctx context.Context, query string, req AnalyzeRequest) ([]searchCandidate, []string) {
	searchURL := s.searchBaseURL
	if searchURL == "" {
		return nil, []string{"internal search base URL is not configured"}
	}
	u, err := url.Parse(searchURL)
	if err != nil {
		return nil, []string{fmt.Sprintf("internal search base URL is invalid: %v", err)}
	}
	values := u.Query()
	values.Set("q", query)
	u.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, []string{fmt.Sprintf("search request failed for %q: %v", query, err)}
	}
	request.Header.Set("User-Agent", "PookiePaws/1.1 (research-discovery)")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, []string{fmt.Sprintf("search request failed for %q: %v", query, err)}
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return nil, []string{fmt.Sprintf("search request for %q returned HTTP %d", query, response.StatusCode)}
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 512<<10))
	if err != nil {
		return nil, []string{fmt.Sprintf("search response for %q could not be read: %v", query, err)}
	}

	base, _ := url.Parse(u.String())
	results := parseSearchResultsHTML(string(body), base)
	candidates := make([]searchCandidate, 0, min(5, len(results)))
	for _, item := range results {
		candidate := searchCandidate{
			Source: Source{
				URL:         item.URL,
				Host:        hostname(item.URL),
				Title:       item.Title,
				Description: item.Description,
				Query:       query,
				Competitor:  detectCompetitor(query, req),
				PageType:    detectPageType(item.URL, item.Title),
			},
		}
		candidate.score = scoreCandidate(candidate, req)
		if candidate.score >= 0 {
			candidates = append(candidates, candidate)
		}
		if len(candidates) == 5 {
			break
		}
	}
	return candidates, nil
}

func (s *Service) discoverFromSeedDomains(ctx context.Context, req AnalyzeRequest) ([]searchCandidate, []string) {
	candidates := make([]searchCandidate, 0, len(req.Domains)*3)
	warnings := make([]string, 0)
	for _, domain := range req.Domains {
		seedURL, err := normalizeSeedURL(domain)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Seed domain %q is invalid: %v", domain, err))
			continue
		}
		document, err := s.fetchDocument(ctx, seedURL)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not crawl seed %s: %v", seedURL, err))
			continue
		}
		rootCandidate := searchCandidate{
			Source: Source{
				URL:         document.URL,
				Host:        hostname(document.URL),
				Title:       firstValue(document.Title, safeURLLabel(document.URL)),
				Description: excerptFromText(document.Text),
				Competitor:  closestCompetitor(hostname(document.URL), req.competitors()),
				PageType:    detectPageType(document.URL, document.Title),
			},
			Markdown: markdownFromDocument(document),
		}
		rootCandidate.score = scoreCandidate(rootCandidate, req)
		candidates = append(candidates, rootCandidate)

		for _, link := range extractInternalLinks(document.URL, document.HTML) {
			candidate := searchCandidate{
				Source: Source{
					URL:         link,
					Host:        hostname(link),
					Title:       safeURLLabel(link),
					Description: "Discovered from the seed-domain crawl",
					Competitor:  closestCompetitor(hostname(link), req.competitors()),
					PageType:    detectPageType(link, link),
				},
			}
			candidate.score = scoreCandidate(candidate, req)
			if candidate.score >= 0 {
				candidates = append(candidates, candidate)
			}
			if len(candidates) >= req.MaxSources*2 {
				break
			}
		}
	}
	return candidates, warnings
}

func discoverExplicitPages(req AnalyzeRequest) ([]searchCandidate, []string) {
	candidates := make([]searchCandidate, 0, len(req.Pages))
	warnings := make([]string, 0)
	for _, page := range req.Pages {
		page = strings.TrimSpace(page)
		if page == "" {
			continue
		}
		if err := validatePublicURL(page); err != nil {
			warnings = append(warnings, fmt.Sprintf("Skipped explicit page %q: %v", page, err))
			continue
		}
		candidate := searchCandidate{
			Source: Source{
				URL:         page,
				Host:        hostname(page),
				Title:       safeURLLabel(page),
				Description: "Explicit watchlist page",
				Competitor:  closestCompetitor(hostname(page), req.competitors()),
				PageType:    detectPageType(page, page),
				Query:       "watchlist.page",
			},
		}
		candidate.score = scoreCandidate(candidate, req)
		if candidate.score >= 0 {
			candidates = append(candidates, candidate)
		}
	}
	return candidates, warnings
}

func (s *Service) fetchCandidates(ctx context.Context, selected []searchCandidate) ([]searchCandidate, int, []string) {
	jobs := make(chan fetchJob)
	results := make(chan fetchResult, len(selected))
	workerCount := min(3, max(1, len(selected)))

	for worker := 0; worker < workerCount; worker++ {
		go func() {
			for job := range jobs {
				document, err := s.fetchDocument(ctx, job.candidate.URL)
				if err != nil {
					results <- fetchResult{index: job.index, err: err}
					continue
				}
				job.candidate.Source.Title = firstValue(document.Title, job.candidate.Source.Title)
				job.candidate.Source.Description = firstValue(job.candidate.Source.Description, excerptFromText(document.Text))
				job.candidate.Markdown = markdownFromDocument(document)
				results <- fetchResult{index: job.index, candidate: job.candidate}
			}
		}()
	}

	for index, candidate := range selected {
		jobs <- fetchJob{index: index, candidate: candidate}
	}
	close(jobs)

	fetched := make([]searchCandidate, 0, len(selected))
	warnings := make([]string, 0)
	scraped := 0
	for range selected {
		result := <-results
		if result.err != nil {
			warnings = append(warnings, fmt.Sprintf("Internal fetch failed for %s: %v", selected[result.index].URL, result.err))
			continue
		}
		fetched = append(fetched, result.candidate)
		scraped++
	}
	return fetched, scraped, warnings
}

func (s *Service) fetchDocument(ctx context.Context, targetURL string) (fetchedDocument, error) {
	if err := validatePublicURL(targetURL); err != nil {
		return fetchedDocument{}, err
	}
	allowed, err := s.allowedByRobots(ctx, targetURL)
	if err != nil {
		return fetchedDocument{}, err
	}
	if !allowed {
		return fetchedDocument{}, fmt.Errorf("robots policy blocks %s", targetURL)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		document, err := s.fetchDocumentOnce(ctx, targetURL)
		if err == nil {
			return document, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("fetch failed")
	}
	return fetchedDocument{}, lastErr
}

func (s *Service) fetchDocumentOnce(ctx context.Context, targetURL string) (fetchedDocument, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return fetchedDocument{}, err
	}
	request.Header.Set("User-Agent", "PookiePaws/1.1 (research-crawler)")

	response, err := s.client.Do(request)
	if err != nil {
		return fetchedDocument{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return fetchedDocument{}, fmt.Errorf("HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fetchedDocument{}, err
	}
	htmlBody := string(body)
	title := webfetch.ExtractHTMLTitle(htmlBody)
	text := webfetch.StripHTML(htmlBody)
	if len(text) > 10000 {
		text = text[:10000]
	}
	return fetchedDocument{
		URL:   targetURL,
		Title: title,
		Text:  text,
		HTML:  htmlBody,
	}, nil
}

func (s *Service) allowedByRobots(ctx context.Context, targetURL string) (bool, error) {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false, err
	}
	cacheKey := parsed.Scheme + "://" + parsed.Host

	s.robotsMu.Lock()
	if s.robotsCache == nil {
		s.robotsCache = map[string]robotsPolicy{}
	}
	policy, ok := s.robotsCache[cacheKey]
	s.robotsMu.Unlock()
	if !ok {
		policy = s.loadRobotsPolicy(ctx, cacheKey)
		s.robotsMu.Lock()
		s.robotsCache[cacheKey] = policy
		s.robotsMu.Unlock()
	}
	requestPath := parsed.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}
	longestAllow := longestPrefix(policy.allows, requestPath)
	longestDisallow := longestPrefix(policy.disallows, requestPath)
	if longestDisallow == 0 {
		return true, nil
	}
	return longestAllow >= longestDisallow, nil
}

func (s *Service) loadRobotsPolicy(ctx context.Context, baseURL string) robotsPolicy {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/robots.txt", nil)
	if err != nil {
		return robotsPolicy{}
	}
	request.Header.Set("User-Agent", "PookiePaws/1.1 (research-crawler)")
	response, err := s.client.Do(request)
	if err != nil {
		return robotsPolicy{}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode >= http.StatusBadRequest {
		return robotsPolicy{}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return robotsPolicy{}
	}
	return parseRobotsPolicy(string(body))
}

func parseRobotsPolicy(body string) robotsPolicy {
	lines := strings.Split(body, "\n")
	policy := robotsPolicy{}
	matchesAgent := false
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch key {
		case "user-agent":
			agent := strings.ToLower(value)
			matchesAgent = agent == "*" || strings.Contains(agent, "pookie")
		case "allow":
			if matchesAgent && value != "" {
				policy.allows = append(policy.allows, value)
			}
		case "disallow":
			if matchesAgent && value != "" {
				policy.disallows = append(policy.disallows, value)
			}
		}
	}
	return policy
}

func parseSearchResultsHTML(body string, base *url.URL) []Source {
	matches := anchorHrefPattern.FindAllStringSubmatch(body, -1)
	results := make([]Source, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		rawHref := normalizeSearchResultURL(match[1], base)
		if rawHref == "" {
			continue
		}
		if err := validatePublicURL(rawHref); err != nil {
			continue
		}
		key := normalizedURL(rawHref)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		title := cleanAnchorText(match[2])
		if title == "" {
			title = safeURLLabel(rawHref)
		}
		results = append(results, Source{
			URL:   rawHref,
			Title: title,
			Host:  hostname(rawHref),
		})
	}
	return results
}

func normalizeSearchResultURL(rawHref string, base *url.URL) string {
	rawHref = html.UnescapeString(strings.TrimSpace(rawHref))
	if rawHref == "" || strings.HasPrefix(rawHref, "#") || strings.HasPrefix(strings.ToLower(rawHref), "javascript:") {
		return ""
	}
	if strings.HasPrefix(rawHref, "//") {
		rawHref = "https:" + rawHref
	}
	if strings.HasPrefix(rawHref, "/") && base != nil {
		rel, err := url.Parse(rawHref)
		if err != nil {
			return ""
		}
		rawHref = base.ResolveReference(rel).String()
	}
	parsed, err := url.Parse(rawHref)
	if err != nil {
		return ""
	}
	if target := parsed.Query().Get("uddg"); target != "" {
		rawHref = target
	}
	return strings.TrimSpace(rawHref)
}

func cleanAnchorText(value string) string {
	value = html.UnescapeString(value)
	value = strings.NewReplacer("<b>", " ", "</b>", " ", "<strong>", " ", "</strong>", " ").Replace(value)
	return collapseWhitespace(stripHTMLTags(value))
}

func extractInternalLinks(baseURL, htmlBody string) []string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil
	}
	results := make([]string, 0, 6)
	seen := map[string]struct{}{}
	for _, match := range anchorHrefPattern.FindAllStringSubmatch(htmlBody, -1) {
		link := normalizeSearchResultURL(match[1], base)
		if link == "" {
			continue
		}
		if hostname(link) != base.Hostname() {
			continue
		}
		if shouldDropURL(link) {
			continue
		}
		if pageType := detectPageType(link, link); pageType == "homepage" {
			continue
		}
		key := normalizedURL(link)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, link)
		if len(results) == 6 {
			break
		}
	}
	return results
}

func markdownFromDocument(document fetchedDocument) string {
	text := strings.TrimSpace(document.Text)
	if text == "" {
		return ""
	}
	if title := strings.TrimSpace(document.Title); title != "" {
		return "# " + title + "\n\n" + text
	}
	return text
}

func excerptFromText(text string) string {
	text = collapseWhitespace(strings.TrimSpace(text))
	if len(text) > 180 {
		text = text[:180]
	}
	return strings.TrimSpace(text)
}

func stripHTMLTags(value string) string {
	value = htmlTagPattern.ReplaceAllString(value, " ")
	return html.UnescapeString(value)
}

func longestPrefix(patterns []string, path string) int {
	longest := 0
	for _, pattern := range patterns {
		if strings.HasPrefix(path, pattern) && len(pattern) > longest {
			longest = len(pattern)
		}
	}
	return longest
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
