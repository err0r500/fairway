//go:generate go run github.com/err0r500/fairway/cmd

package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"slices"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/realworldapp/automate"
	"github.com/err0r500/fairway/examples/realworldapp/change"
	"github.com/err0r500/fairway/examples/realworldapp/ui"
	"github.com/err0r500/fairway/examples/realworldapp/view"
)

func main() {
	// Setup FDB
	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// core
	coreStore := dcb.NewDcbStore(db, "realworldapp", dcb.StoreOptions{}.WithLogger(logger))

	// Start automations
	stopAutomations, err := automate.Registry.StartAll(context.Background(), coreStore, automate.AllDeps{
		EmailSender: &LoggingEmailSender{},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer stopAutomations()

	// Setup router
	mux := http.NewServeMux()
	runner := fairway.NewCommandRunner(coreStore)
	reader := fairway.NewReader(coreStore)
	change.ChangeRegistry.RegisterRoutes(mux, runner)
	view.ViewRegistry.RegisterRoutes(mux, reader)
	ui.NewHandlers(runner, reader).RegisterRoutes(mux)

	// Start server
	for _, route := range slices.Concat(
		change.ChangeRegistry.RegisteredRoutes(),
		view.ViewRegistry.RegisteredRoutes(),
	) {
		slog.Info("Registered route: " + route)
	}

	logger.Info("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", panicLogMiddleware(mux)))
}

func panicLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"panic", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"query", r.URL.RawQuery,
					"remote_addr", r.RemoteAddr,
					"stack", string(debug.Stack()),
				)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
