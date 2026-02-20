//go:generate go run github.com/err0r500/fairway/cmd

package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/shoppingcart/automate"
	"github.com/err0r500/fairway/examples/shoppingcart/automate/oninventorychanged"
	"github.com/err0r500/fairway/examples/shoppingcart/automate/onpricechanged"
	"github.com/err0r500/fairway/examples/shoppingcart/change"
	"github.com/err0r500/fairway/examples/shoppingcart/view"
)

func main() {
	// Setup FDB
	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	ctx := context.Background()

	// core
	coreStore := dcb.NewDcbStore(db, "shoppingcart", dcb.StoreOptions{}.WithLogger(logger))

	// Start automations
	stopAutomations, err := automate.Registry.StartAll(ctx, coreStore, automate.AllDeps{})
	if err != nil {
		log.Fatal(err)
	}
	defer stopAutomations()

	// Start read models
	stopReadModels, err := view.ReadModelRegistry.StartAll(ctx, coreStore)
	if err != nil {
		log.Fatal(err)
	}
	defer stopReadModels()

	// Setup router
	mux := http.NewServeMux()
	runner := fairway.NewCommandRunner(coreStore)

	change.ChangeRegistry.RegisterRoutes(mux, runner)
	view.ViewRegistry.RegisterRoutes(mux, fairway.NewReader(coreStore))
	// opencartswithproducts.Register(mux)

	// Register external event webhooks
	oninventorychanged.RegisterHTTP(mux, runner)
	onpricechanged.RegisterHTTP(mux, runner)

	// Start server
	for _, route := range slices.Concat(
		change.ChangeRegistry.RegisteredRoutes(),
		view.ViewRegistry.RegisteredRoutes(),
	) {
		slog.Info("Registered route: " + route)
	}
	slog.Info("Registered route: GET /open-carts/{productId}")

	logger.Info("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
