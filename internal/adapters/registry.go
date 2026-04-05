package adapters

import (
	"sort"
	"sync"

	"github.com/mitpoai/pookiepaws/internal/engine"
)

// InMemoryChannelRegistry is a thread-safe in-process registry for marketing
// channel plugins. It implements engine.MarketingChannelRegistry.
type InMemoryChannelRegistry struct {
	mu       sync.RWMutex
	channels map[string]engine.MarketingChannel
}

var _ engine.MarketingChannelRegistry = (*InMemoryChannelRegistry)(nil)

// NewChannelRegistry creates an empty channel registry.
func NewChannelRegistry() *InMemoryChannelRegistry {
	return &InMemoryChannelRegistry{
		channels: make(map[string]engine.MarketingChannel),
	}
}

// Register adds or replaces a marketing channel in the registry.
func (r *InMemoryChannelRegistry) Register(channel engine.MarketingChannel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.channels[channel.Name()] = channel
}

// Get returns a channel by name and whether it was found.
func (r *InMemoryChannelRegistry) Get(name string) (engine.MarketingChannel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ch, ok := r.channels[name]
	return ch, ok
}

// List returns all registered channels sorted by name.
func (r *InMemoryChannelRegistry) List() []engine.MarketingChannel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]engine.MarketingChannel, 0, len(r.channels))
	for _, ch := range r.channels {
		list = append(list, ch)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
	return list
}

// ByKind returns all channels matching the given kind (e.g. "crm", "email").
func (r *InMemoryChannelRegistry) ByKind(kind string) []engine.MarketingChannel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var matches []engine.MarketingChannel
	for _, ch := range r.channels {
		if ch.Kind() == kind {
			matches = append(matches, ch)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name() < matches[j].Name()
	})
	return matches
}
