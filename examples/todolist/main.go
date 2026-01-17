package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/todolist/core/change"
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
	// view.ViewRegistry.RegisterRoutes(mux, client)

	// Start server
	logger.Info("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
