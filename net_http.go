package fairway

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"
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

// type HttpViewRegistry struct {
// 	registeredReadModels []viewRegistration
// }
//
// // viewRegistration represents a query route registration
// type viewRegistration struct {
// 	Pattern string
// 	Handler func(*Client) http.HandlerFunc
// }
//
// // RegisterQuery registers a query handler factory
// func (registry *HttpViewRegistry) RegisterReadModel(pattern string, handler func(*Client) http.HandlerFunc) {
// 	registry.registeredReadModels = append(registry.registeredReadModels, viewRegistration{
// 		Pattern: pattern,
// 		Handler: handler,
// 	})
// }
//
// // RegisterRoutes registers all query routes to the mux
// func (registry HttpViewRegistry) RegisterRoutes(mux *http.ServeMux, client *Client) {
// 	for _, reg := range registry.registeredReadModels {
// 		mux.HandleFunc(reg.Pattern, reg.Handler(client))
// 	}
// }

// TODO : this function does too much, should return errors and
// let the client handle them
// JsonParse decodes JSON and validates struct
// Returns true if successful, false if failed (400 written)
func JsonParse[T any](w http.ResponseWriter, r *http.Request, v *T) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	if err := validator.New().Struct(v); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	return true
}
