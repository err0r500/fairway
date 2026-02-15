package fairway

import (
	"net/http"

	"github.com/err0r500/fairway/dcb"
)

type HttpChangeRegistry struct {
	// registeredCommands stores all registered command routes
	registeredCommands []changeRegistration
	// idempotencyStore, if set, enables idempotent change request handling
	idempotencyStore dcb.IdempotencyStore
}

// changeRegistration represents a command route registration
type changeRegistration struct {
	Pattern string
	Handler func(CommandRunner) http.HandlerFunc
}

// RegisterCommand registers a command handler
func (registry *HttpChangeRegistry) RegisterCommand(pattern string, handler func(CommandRunner) http.HandlerFunc) {
	registry.registeredCommands = append(registry.registeredCommands, changeRegistration{
		Pattern: pattern,
		Handler: handler,
	})
}

// WithIdempotency configures the registry to use an idempotency store.
// When set, requests with an Idempotency-Key header will be deduplicated:
// the first request is processed normally and its status code is cached;
// subsequent requests with the same key return the cached status code.
func (registry *HttpChangeRegistry) WithIdempotency(store dcb.IdempotencyStore) {
	registry.idempotencyStore = store
}

// RegisterRoutes registers all command routes to the mux
func (registry HttpChangeRegistry) RegisterRoutes(mux *http.ServeMux, runner CommandRunner) {
	for _, reg := range registry.registeredCommands {
		handler := reg.Handler(runner)
		if registry.idempotencyStore != nil {
			handler = idempotencyMiddleware(registry.idempotencyStore, handler)
		}
		mux.HandleFunc(reg.Pattern, handler)
	}
}

func (registry HttpChangeRegistry) RegisteredRoutes() []string {
	result := []string{}
	for _, c := range registry.registeredCommands {
		result = append(result, c.Pattern)
	}
	return result
}

type HttpViewRegistry struct {
	registeredViews []viewRegistration
}

// viewRegistration represents a query route registration
type viewRegistration struct {
	Pattern string
	Handler func(EventsReader) http.HandlerFunc
}

// RegisterQuery registers a query handler factory
func (registry *HttpViewRegistry) RegisterView(pattern string, handler func(EventsReader) http.HandlerFunc) {
	registry.registeredViews = append(registry.registeredViews, viewRegistration{
		Pattern: pattern,
		Handler: handler,
	})
}

// RegisterRoutes registers all query routes to the mux
func (registry HttpViewRegistry) RegisterRoutes(mux *http.ServeMux, client EventsReader) {
	for _, reg := range registry.registeredViews {
		mux.HandleFunc(reg.Pattern, reg.Handler(client))
	}
}

func (registry HttpViewRegistry) RegisteredRoutes() []string {
	result := []string{}
	for _, c := range registry.registeredViews {
		result = append(result, c.Pattern)
	}
	return result
}
