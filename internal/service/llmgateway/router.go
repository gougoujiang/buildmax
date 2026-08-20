package llmgateway

import (
	"context"
	"errors"
	"sync"
	"time"

	cllm "github.com/gougoujiang/buildmax/internal/core/llm"
)

// ErrFactoryNotConfigured is returned when a router has no way to build clients.
var ErrFactoryNotConfigured = errors.New("model client factory is not configured")

// ClientFactory builds a client for an operator-approved target.
//
// The factory, not this package, resolves Target.CredentialRef into a real
// credential. That keeps provider secrets out of resolution, listing, and
// diagnostics, and keeps provider packages out of the service layer.
type ClientFactory func(ctx context.Context, target Target) (cllm.LLMClient, error)

// RoutedClient is a resolved alias together with the client that calls it.
type RoutedClient struct {
	// Resolution is the alias and target this request resolved to. Metering and
	// the call ledger attribute the call with it.
	Resolution Resolution
	// Client calls the upstream target.
	Client cllm.LLMClient
}

// clientKey is the part of a target that determines how its client is built.
// Two targets with the same key can share one client; a target whose key
// changes needs a new one.
type clientKey struct {
	providerType  string
	endpoint      string
	credentialRef string
	upstreamModel string
	contextWindow int
	callTimeout   time.Duration
	maxTokens     int
	reasoning     string
	promptCache   bool
	vision        bool
}

func keyOf(target Target) clientKey {
	return clientKey{
		providerType:  target.ProviderType,
		endpoint:      target.Endpoint,
		credentialRef: target.CredentialRef,
		upstreamModel: target.UpstreamModel,
		contextWindow: target.ContextWindow,
		callTimeout:   target.CallTimeout,
		maxTokens:     target.MaxTokens,
		reasoning:     target.Reasoning,
		promptCache:   target.PromptCache,
		vision:        target.Vision,
	}
}

type cachedClient struct {
	key    clientKey
	client cllm.LLMClient
}

// Router turns a team's model alias into a client for the approved upstream.
//
// It caches one client per catalog target. A cached client is rebuilt when the
// target's connection details change, so a catalog that becomes editable later
// does not serve calls through a retired endpoint or a rotated credential.
type Router struct {
	// Resolver maps (team, alias) to an approved target.
	Resolver *Resolver
	// Factory builds a client for a resolved target.
	Factory ClientFactory

	mu      sync.Mutex
	clients map[string]cachedClient
}

// ClientFor resolves the request and returns a client for the approved target.
func (r *Router) ClientFor(ctx context.Context, req ResolveRequest) (RoutedClient, error) {
	if r == nil {
		return RoutedClient{}, ErrCatalogNotConfigured
	}
	if r.Factory == nil {
		return RoutedClient{}, ErrFactoryNotConfigured
	}
	resolution, err := r.Resolver.Resolve(ctx, req)
	if err != nil {
		return RoutedClient{}, err
	}
	client, err := r.clientForTarget(ctx, resolution.Target)
	if err != nil {
		return RoutedClient{}, err
	}
	return RoutedClient{Resolution: resolution, Client: client}, nil
}

// ClientForTarget returns a client for a catalog target the deployment itself
// selected, bypassing team policy. Resolution.Alias is empty in the result
// because no alias was involved.
//
// This is the path for Server-owned inference. It shares the resolver's
// capability checks and the router's client cache with managed calls, so
// in-process work and team calls cannot drift apart.
func (r *Router) ClientForTarget(ctx context.Context, targetID string, requires []Capability) (RoutedClient, error) {
	if r == nil {
		return RoutedClient{}, ErrCatalogNotConfigured
	}
	if r.Factory == nil {
		return RoutedClient{}, ErrFactoryNotConfigured
	}
	target, err := r.Resolver.ResolveTargetByID(ctx, targetID, requires)
	if err != nil {
		return RoutedClient{}, err
	}
	client, err := r.clientForTarget(ctx, target)
	if err != nil {
		return RoutedClient{}, err
	}
	return RoutedClient{Resolution: Resolution{Target: target}, Client: client}, nil
}

// Available lists the aliases a team may use. It delegates to the resolver so
// callers depend on one type for both routing and discovery.
func (r *Router) Available(ctx context.Context, teamID string) ([]AvailableModel, error) {
	if r == nil {
		return nil, ErrCatalogNotConfigured
	}
	return r.Resolver.Available(ctx, teamID)
}

func (r *Router) clientForTarget(ctx context.Context, target Target) (cllm.LLMClient, error) {
	key := keyOf(target)

	// Construction is local work, so one lock keeps the cache simple and stops
	// concurrent callers from building the same client twice.
	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.clients[target.ID]; ok && cached.key == key {
		return cached.client, nil
	}
	client, err := r.Factory(ctx, target)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrFactoryNotConfigured
	}
	if r.clients == nil {
		r.clients = make(map[string]cachedClient)
	}
	r.clients[target.ID] = cachedClient{key: key, client: client}
	return client, nil
}
