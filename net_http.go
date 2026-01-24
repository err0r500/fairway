package fairway

import (
	"net/http"
)

type HttpChangeRegistry struct {
	// registeredCommands stores all registered command routes
	registeredCommands []changeRegistration
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

// RegisterRoutes registers all command routes to the mux
func (registry HttpChangeRegistry) RegisterRoutes(mux *http.ServeMux, runner CommandRunner) {
	for _, reg := range registry.registeredCommands {
		mux.HandleFunc(reg.Pattern, reg.Handler(runner))
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
