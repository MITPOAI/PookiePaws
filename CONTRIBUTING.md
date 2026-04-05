# Contributing To PookiePaws

## Before You Start

PookiePaws welcomes contributions that improve the agent, add marketing channels, fix bugs, or enhance documentation. Every contribution should make the repository easier to trust and the tool more useful for marketers.

Good contribution areas:

- New marketing channel adapters (see the guide below)
- Bug fixes and performance improvements
- Documentation improvements
- Architecture review and critique
- Test coverage expansion
- UI/UX polish

## Contribution Standard

Every contribution should:

- Keep claims accurate and distinguish current from planned behavior
- Prefer explicit tradeoffs over vague optimism
- Maintain the pure-Go, stdlib-first philosophy (no heavy SDKs)
- Keep the compiled binary under 10MB
- Use atomic writes for all file operations

## Documentation Governance

Unless clearly not applicable, the same pull request must review and update:

- [README.md](./README.md)
- [ARCHITECTURE.md](./ARCHITECTURE.md)
- [CHANGELOG.md](./CHANGELOG.md)

## How To Write a Marketing Channel Plugin

PookiePaws uses the `MarketingChannel` interface to standardise all integrations. Adding a new channel (e.g. Mailchimp, Brevo, ActiveCampaign) follows these steps.

### Step 1: Implement the MarketingChannel Interface

Create `internal/adapters/yourservice.go`. Your adapter must implement all 6 methods:

```go
package adapters

import (
    "context"
    "github.com/mitpoai/pookiepaws/internal/engine"
)

type YourServiceAdapter struct {
    client *http.Client
}

func NewYourServiceAdapter() *YourServiceAdapter {
    return &YourServiceAdapter{client: newAdapterClient()} // 30s timeout
}

// Name returns the unique adapter identifier.
func (a *YourServiceAdapter) Name() string { return "yourservice" }

// Kind returns the channel category.
// Use: "crm", "sms", "email", "whatsapp", "research", or "export".
func (a *YourServiceAdapter) Kind() string { return "email" }

// Status reports whether required secrets are configured.
func (a *YourServiceAdapter) Status(secrets engine.SecretProvider) engine.ChannelProviderStatus {
    apiKey, _ := secrets.Get("yourservice_api_key")
    configured := strings.TrimSpace(apiKey) != ""
    return engine.ChannelProviderStatus{
        Provider:   "yourservice",
        Channel:    "email",
        Configured: configured,
        Healthy:    configured,
        Message:    "YourService status",
    }
}

// Test verifies the API is reachable and credentials are valid.
func (a *YourServiceAdapter) Test(ctx context.Context, secrets engine.SecretProvider) (engine.ChannelProviderStatus, error) {
    // Make a lightweight API call to verify credentials.
    // Return healthy status on success, error on failure.
}

// Execute runs a channel operation.
func (a *YourServiceAdapter) Execute(ctx context.Context, action engine.AdapterAction, secrets engine.SecretProvider) (engine.AdapterResult, error) {
    switch action.Operation {
    case "send_email":
        // Build and send HTTP request using action.Payload fields.
        // Return AdapterResult with status and details.
    default:
        return engine.AdapterResult{}, fmt.Errorf("unsupported operation %q", action.Operation)
    }
}

// SecretKeys returns the vault keys this adapter needs.
func (a *YourServiceAdapter) SecretKeys() []string {
    return []string{"yourservice_api_key"}
}
```

Refer to `internal/adapters/resend.go` as a complete working example.

### Step 2: Add a Mock Adapter

Add a mock to `internal/adapters/mock.go` for testing:

```go
type MockYourServiceAdapter struct{}

var _ engine.MarketingChannel = (*MockYourServiceAdapter)(nil)

func NewMockYourServiceAdapter() *MockYourServiceAdapter { return &MockYourServiceAdapter{} }
func (a *MockYourServiceAdapter) Name() string           { return "yourservice" }
func (a *MockYourServiceAdapter) Kind() string           { return "email" }
// ... implement remaining methods returning mock data
```

### Step 3: Add Integration Definition

In `cmd/pookie/init_integrations.go`, add your service to the integration definitions so `pookie init` can configure it:

```go
{ID: "yourservice", Label: "YourService", Keys: []string{"yourservice_api_key"}}
```

Add a `configureYourServiceIntegration()` function following the existing patterns.

### Step 4: Register in the Channel Registry

In the appropriate stack setup code, register your adapter:

```go
registry.Register(adapters.NewYourServiceAdapter())
```

### Step 5: Add Security Policy

If your adapter is used by a skill, add a security policy in `internal/security/interceptor.go`:

```go
"skill-using-yourservice": {
    risk:        "high",
    allowedKeys: setOf("to", "subject", "body"),
    altPrompt:   "Suggest a narrower workflow.",
},
```

### Step 6: Write Tests

Create `internal/adapters/yourservice_test.go` using `net/http/httptest`:

```go
func TestYourServiceSendEmail(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(200)
        w.Write([]byte(`{"id": "test-123"}`))
    }))
    defer server.Close()
    // ... test Execute() with mock server
}
```

### Step 7: Update Documentation

Update `CHANGELOG.md`, `ARCHITECTURE.md`, and `README.md` to mention the new channel.

## Shared Helpers

The `internal/adapters/live.go` file provides three helpers every adapter should reuse:

- `newAdapterClient()` - returns `*http.Client` with 30-second timeout
- `readAdapterResponse(resp, name)` - reads response body (1MB limit), checks status codes, unmarshals JSON
- `secretWithFallback(secrets, key, fallback)` - retrieves a secret with a default value

## Pull Request Expectations

Each pull request should include:

- A concise summary of the change
- Updated documentation (README, ARCHITECTURE, CHANGELOG)
- Tests with httptest mocks for any new HTTP integrations
- Any assumptions or deferred work noted

## Review Criteria

Reviewers look for:

- Accuracy of project claims
- Consistency with the architecture contract
- Clean separation between current state and roadmap
- Security and governance implications
- Binary size impact (must stay under 10MB)

## Community Conduct

By participating in this repository, you agree to follow [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md).

## Security

If your contribution touches trust boundaries, secrets, integrations, or execution policy, read [SECURITY.md](./SECURITY.md) before opening a pull request.
