package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway/dcb"
)

var (
	concurrency = flag.Int("concurrency", 50, "number of concurrent scenarios")
	fdbCluster  = flag.String("fdb-cluster", "", "FDB cluster file path (default: use FDB_CLUSTER_FILE env)")
	metricsPort = flag.Int("metrics-port", 8080, "port for metrics and pprof")
	mode        = flag.String("mode", "todo", "benchmark mode: todo or write")
	duration    = flag.Duration("duration", 30*time.Second, "write benchmark duration (0 = run until signal)")
	reportEvery = flag.Duration("report-interval", time.Second, "write benchmark reporting interval")
	payloadSize = flag.Int("payload-size", 128, "write benchmark payload size in bytes")
	batchSize   = flag.Int("batch-size", 1, "write benchmark events per append")
)

func main() {
	flag.Parse()

	// Setup FDB
	fdb.MustAPIVersion(740)

	var db fdb.Database
	if *fdbCluster != "" {
		db = fdb.MustOpenDatabase(*fdbCluster)
	} else {
		db = fdb.MustOpenDefault()
	}

	// Create event store with observability
	store := dcb.NewDcbStore(db, "todo-bench", dcb.StoreOptions{}.WithMetrics(prometheusMetrics{}))

	// Start metrics server
	go startMetricsServer(*metricsPort)

	if *mode != "todo" && *mode != "write" {
		log.Fatalf("unsupported mode: %s (expected todo or write)", *mode)
	}

	if *mode == "write" {
		runWriteBenchmark(store)
		return
	}

	// Run benchmark
	log.Printf("Starting benchmark with %d concurrent scenarios", *concurrency)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown gracefully
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sigCount := 0
		for range sigChan {
			sigCount++
			if sigCount == 1 {
				log.Println("Shutting down... (press Ctrl+C again to force)")
				cancel()
				go func() {
					select {
					case <-time.After(10 * time.Second):
						log.Println("Forcing exit after shutdown timeout")
						os.Exit(1)
					case <-ctx.Done():
					}
				}()
				continue
			}
			log.Println("Forcing exit")
			os.Exit(1)
		}
	}()

	var wg sync.WaitGroup
	for i := range *concurrency {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			runScenarios(ctx, store, workerID)
		}(i)
	}

	// Wait for cancellation
	<-ctx.Done()
	wg.Wait()

	log.Printf("Total scenarios completed: %d", totalScenariosCompleted.Load())
	log.Printf("Total appends: %d", totalAppends.Load())
	log.Printf("Total reads: %d", totalReads.Load())
}

var (
	scenarioIDCounter       atomic.Uint64
	totalScenariosCompleted atomic.Uint64
	totalAppends            atomic.Uint64
	totalReads              atomic.Uint64
	writeEventID            atomic.Uint64
)

// runScenarios continuously runs scenarios until context is cancelled
func runScenarios(ctx context.Context, store dcb.DcbStore, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			runSingleScenario(ctx, store, workerID)
		}
	}
}

// runSingleScenario executes one complete todo list lifecycle
func runSingleScenario(ctx context.Context, store dcb.DcbStore, _ int) {
	// scenarioID := scenarioIDCounter.Add(1)
	// listID := fmt.Sprintf("list-%d", scenarioID)

	// Phase 1: Create list
	var wg sync.WaitGroup
	wg.Add(1000)
	for range 1000 {
		go func() {
			defer wg.Done()
			appendEvent(ctx, store, "list_created", []string{}, nil)
		}()
	}
	wg.Wait()
	//
	// Phase 2: Insert 10 items concurrently
	// readByListTag(ctx, store, listID)

	// wg.Add(10)
	// for i := range 10 {
	// 	go func(idx int) {
	// 		defer wg.Done()
	// 		itemID := fmt.Sprintf("item-%d-%d", scenarioID, idx)
	// 		appendEvent(ctx, store, "item_inserted", []string{
	// 			fmt.Sprintf("list:%s", listID),
	// 			fmt.Sprintf("item:%s", itemID),
	// 			"status:pending",
	// 		}, &dcb.AppendCondition{
	// 			Query: dcb.Query{
	// 				Items: []dcb.QueryItem{
	// 					{
	// 						Types: []string{"item_inserted"},
	// 						Tags:  []string{fmt.Sprintf("item:%s", itemID)},
	// 					},
	// 					{
	// 						Types: []string{"list_deleted"},
	// 						Tags:  []string{fmt.Sprintf("list:%s", listID)},
	// 					},
	// 				},
	// 			},
	// 		})
	// 	}(i)
	// }
	// // wg.Wait()
	//
	// // Phase 3: Update 3 items concurrently
	// // readByListAndStatus(ctx, store, listID, "pending")
	//
	// wg.Add(3)
	// for i := range 3 {
	// 	go func(idx int) {
	// 		defer wg.Done()
	// 		itemID := fmt.Sprintf("item-%d-%d", scenarioID, idx)
	// 		appendEvent(ctx, store, "item_updated", []string{
	// 			fmt.Sprintf("list:%s", listID),
	// 			fmt.Sprintf("item:%s", itemID),
	// 			"status:completed",
	// 		}, &dcb.AppendCondition{
	// 			Query: dcb.Query{
	// 				Items: []dcb.QueryItem{
	// 					{
	// 						Types: []string{"item_deleted"},
	// 						Tags:  []string{fmt.Sprintf("item:%s", itemID)},
	// 					},
	// 				},
	// 			},
	// 		})
	// 	}(i)
	// }
	// // wg.Wait()
	//
	// // Phase 4: Delete 2 items concurrently
	// // readByListAndStatus(ctx, store, listID, "completed")
	//
	// wg.Add(2)
	// for i := range 2 {
	// 	go func(idx int) {
	// 		defer wg.Done()
	// 		itemID := fmt.Sprintf("item-%d-%d", scenarioID, idx)
	// 		appendEvent(ctx, store, "item_deleted", []string{
	// 			fmt.Sprintf("list:%s", listID),
	// 			fmt.Sprintf("item:%s", itemID),
	// 		}, &dcb.AppendCondition{
	// 			Query: dcb.Query{
	// 				Items: []dcb.QueryItem{
	// 					{
	// 						Types: []string{"item_deleted"},
	// 						Tags:  []string{fmt.Sprintf("item:%s", itemID)},
	// 					},
	// 				},
	// 			},
	// 		})
	// 	}(i)
	// }
	// // wg.Wait()
	//
	// // Phase 5: Delete list
	// // readByListTag(ctx, store, listID)
	//
	// wg.Add(1)
	// go func() {
	// 	defer wg.Done()
	// 	appendEvent(ctx, store, "list_deleted", []string{
	// 		fmt.Sprintf("list:%s", listID),
	// 	}, &dcb.AppendCondition{
	// 		Query: dcb.Query{
	// 			Items: []dcb.QueryItem{
	// 				{
	// 					Types: []string{"list_deleted"},
	// 					Tags:  []string{fmt.Sprintf("list:%s", listID)},
	// 				},
	// 			},
	// 		},
	// 	})
	// }()
	// wg.Wait()

	// Scenario completed successfully
	totalScenariosCompleted.Add(1)
	scenariosCompleted.Inc()
}

// appendEvent appends a single event and records metrics
func appendEvent(ctx context.Context, store dcb.DcbStore, eventType string, tags []string, condition *dcb.AppendCondition) error {
	payload := fmt.Appendf(nil, `{"timestamp":%d}`, time.Now().Unix())
	events := []dcb.Event{
		{
			Type: eventType,
			Tags: tags,
			Data: payload,
		},
	}

	return appendEvents(ctx, store, events, condition)
}

func appendEvents(ctx context.Context, store dcb.DcbStore, events []dcb.Event, condition *dcb.AppendCondition) error {
	start := time.Now()
	err := store.Append(ctx, events, condition)
	duration := time.Since(start)
	recordAppend(duration, err == nil)
	totalAppends.Add(1)

	return err
}

// readByListTag reads all events for a list
func readByListTag(ctx context.Context, store dcb.DcbStore, listID string) {
	start := time.Now()

	query := dcb.Query{
		Items: []dcb.QueryItem{
			{Tags: []string{fmt.Sprintf("list:%s", listID)}},
		},
	}

	ch := store.Read(ctx, query, &dcb.ReadOptions{Limit: 100})

	// Consume channel
	for range ch {
	}

	duration := time.Since(start)
	recordRead(duration, true)
	totalReads.Add(1)
}

// readByListAndStatus reads events for a list with specific status
func readByListAndStatus(ctx context.Context, store dcb.DcbStore, listID, status string) {
	start := time.Now()

	query := dcb.Query{
		Items: []dcb.QueryItem{
			{Tags: []string{
				fmt.Sprintf("list:%s", listID),
				fmt.Sprintf("status:%s", status),
			}},
		},
	}

	ch := store.Read(ctx, query, &dcb.ReadOptions{Limit: 100})

	// Consume channel
	for range ch {
	}

	duration := time.Since(start)
	recordRead(duration, true)
	totalReads.Add(1)
}

// startMetricsServer starts HTTP server with metrics and pprof
func startMetricsServer(port int) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Metrics server listening on %s", addr)
	log.Printf("Prometheus metrics: http://localhost%s/metrics", addr)
	log.Printf("pprof: http://localhost%s/debug/pprof/", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Failed to start metrics server: %v", err)
	}
}

func runWriteBenchmark(store dcb.DcbStore) {
	if *batchSize < 1 {
		log.Fatalf("batch-size must be >= 1")
	}
	if *payloadSize < 0 {
		log.Fatalf("payload-size must be >= 0")
	}

	var (
		writesTotal atomic.Uint64
		errorsTotal atomic.Uint64
	)

	payload := make([]byte, *payloadSize)

	ctx := context.Background()
	cancel := func() {}
	if *duration > 0 {
		ctx, cancel = context.WithTimeout(ctx, *duration)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sigCount := 0
		for range sigChan {
			sigCount++
			if sigCount == 1 {
				log.Println("Shutting down... (press Ctrl+C again to force)")
				cancel()
				go func() {
					select {
					case <-time.After(10 * time.Second):
						log.Println("Forcing exit after shutdown timeout")
						os.Exit(1)
					case <-ctx.Done():
					}
				}()
				continue
			}
			log.Println("Forcing exit")
			os.Exit(1)
		}
	}()

	log.Printf("Starting write benchmark: concurrency=%d batch=%d payload=%dB duration=%s",
		*concurrency, *batchSize, *payloadSize, *duration)

	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			runWriteWorker(ctx, store, workerID, payload, &writesTotal, &errorsTotal)
		}(i)
	}

	start := time.Now()
	ticker := time.NewTicker(*reportEvery)
	defer ticker.Stop()

	var lastTotal uint64
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			elapsed := time.Since(start).Seconds()
			total := writesTotal.Load()
			avg := float64(total) / elapsed
			log.Printf("Write benchmark complete: total=%d events avg=%.0f events/sec errors=%d",
				total, avg, errorsTotal.Load())
			return
		case <-ticker.C:
			total := writesTotal.Load()
			delta := total - lastTotal
			lastTotal = total
			rate := float64(delta) / reportEvery.Seconds()
			elapsed := time.Since(start).Seconds()
			avg := float64(total) / elapsed
			log.Printf("Writes/sec: %.0f (avg %.0f) total=%d errors=%d",
				rate, avg, total, errorsTotal.Load())
		}
	}
}

func runWriteWorker(
	ctx context.Context,
	store dcb.DcbStore,
	workerID int,
	payload []byte,
	writesTotal *atomic.Uint64,
	errorsTotal *atomic.Uint64,
) {
	events := make([]dcb.Event, 0, *batchSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		events = events[:0]
		for i := 0; i < *batchSize; i++ {
			eventID := writeEventID.Add(1)
			events = append(events, dcb.Event{
				Type: "bench_write",
				Tags: []string{
					fmt.Sprintf("worker:%d", workerID),
					fmt.Sprintf("event:%d", eventID),
				},
				Data: payload,
			})
		}

		if err := appendEvents(ctx, store, events, nil); err != nil {
			errCount := errorsTotal.Add(1)
			if errCount <= 5 || errCount%1000 == 0 {
				log.Printf("append error (count=%d): %v", errCount, err)
			}
			continue
		}
		writesTotal.Add(uint64(len(events)))
	}
}
