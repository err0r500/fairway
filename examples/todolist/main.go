//go:generate go run github.com/err0r500/fairway/cmd

package main

import (
	"log"
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

	// core
	coreStore := dcb.NewDcbStore(db, "core")

	// Setup router
	mux := http.NewServeMux()
	change.ChangeRegistry.RegisterRoutes(mux, fairway.NewCommandRunner(coreStore))
	// view.ViewRegistry.RegisterRoutes(mux, client)

	// Start server
	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
