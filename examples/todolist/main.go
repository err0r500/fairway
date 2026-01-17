//go:generate go run github.com/err0r500/fairway/cmd

package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/todolist/core/change"
	"github.com/err0r500/fairway/examples/todolist/core/view"
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
	coreStore := dcb.NewDcbStore(db, "core", dcb.StoreOptions{}.WithLogger(logger))

	// Setup router
	mux := http.NewServeMux()
	change.ChangeRegistry.RegisterRoutes(mux, fairway.NewCommandRunner(coreStore))
	view.ViewRegistry.RegisterRoutes(mux, fairway.NewReader(coreStore))

	// Start server
	for _, route := range slices.Concat(
		change.ChangeRegistry.RegisteredRoutes(),
		view.ViewRegistry.RegisteredRoutes(),
	) {
		slog.Info("Registered route: " + route)
	}

	logger.Info("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
