package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultRepo is the GitHub `owner/repo` identifier checked by the notifier.
const DefaultRepo = "MITPOAI/PookiePaws"

// DefaultBaseURL is the GitHub REST API base. Override in tests via NewClient.
const DefaultBaseURL = "https://api.github.com"

// Release is the trimmed subset of the GitHub release payload we care about.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

// Client fetches the latest release for a single repo.
type Client struct {
	baseURL    string
	repo       string
	userAgent  string
	httpClient *http.Client
}

// NewClient builds a Client with the given API base and timeout. `currentVersion`
// is embedded in the User-Agent so GitHub's logs identify the caller.
func NewClient(baseURL, currentVersion string, timeout time.Duration) *Client {
	return &Client{
		baseURL:   baseURL,
		repo:      DefaultRepo,
		userAgent: "pookiepaws-update-check/" + currentVersion,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// WithRepo overrides the default repository.
func (c *Client) WithRepo(repo string) *Client {
	c.repo = repo
	return c
}

// FetchLatest returns the latest non-draft, non-prerelease GitHub release.
// Drafts and prereleases are intentionally treated as "no release" — the
// notifier should never push users to unpublished or experimental builds.
func (c *Client) FetchLatest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github returned %d: %s", resp.StatusCode, string(body))
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if rel.Draft || rel.Prerelease {
		return nil, fmt.Errorf("latest release is draft or prerelease (%s)", rel.TagName)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("release missing tag_name")
	}
	return &rel, nil
}
