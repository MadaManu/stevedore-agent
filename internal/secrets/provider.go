package secrets

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrProviderNotFound = errors.New("secret provider not configured")

// Provider resolves secret values for a provider-specific path.
type Provider interface {
	Resolve(path string) (string, error)
}

// Resolver dispatches secret lookups to named providers.
type Resolver struct {
	providers map[string]Provider
}

func NewResolver(providers map[string]Provider) *Resolver {
	out := make(map[string]Provider, len(providers))
	for name, provider := range providers {
		key := strings.TrimSpace(name)
		if key == "" || provider == nil {
			continue
		}
		out[key] = provider
	}
	return &Resolver{providers: out}
}

func (r *Resolver) Resolve(providerName, path string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("%w: %q", ErrProviderNotFound, providerName)
	}
	provider, ok := r.providers[strings.TrimSpace(providerName)]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrProviderNotFound, providerName)
	}
	return provider.Resolve(path)
}

func (r *Resolver) ProviderNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
