package fairway

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/err0r500/fairway/dcb"
)

// AutomationConfig configures automation behavior
type AutomationConfig struct {
	NumWorkers    int           // default: 1
	LeaseTTL      time.Duration // default: 30s
	GracePeriod   time.Duration // default: 60s
	MaxAttempts   int           // default: 3
	BatchSize     int           // default: 16
	PollInterval  time.Duration // default: 100ms
	RetryBaseWait time.Duration // default: 1min (base backoff wait)
}

// defaultConfig returns default automation configuration
func defaultConfig() AutomationConfig {
	return AutomationConfig{
		NumWorkers:    1,
		LeaseTTL:      30 * time.Second,
		GracePeriod:   60 * time.Second,
		MaxAttempts:   3,
		BatchSize:     16,
		PollInterval:  100 * time.Millisecond,
		RetryBaseWait: time.Minute,
	}
}

// Automation watches for events and executes handlers
type Automation[Deps any] struct {
	// Config
	queueId       string
	eventType     string
	eventRegistry eventRegistry
	handler       func(Event) CommandWithEffect[Deps]
	runner        CommandWithEffectRunner[Deps]
	config        AutomationConfig

	// FDB
	db             fdb.Database
	typeIndex      subspace.Subspace // dcb's namespace/t/eventType
	eventsSubspace subspace.Subspace // dcb's namespace/e
	queueDir       subspace.Subspace // automation namespace/queue
	cursorKey      fdb.Key           // automation namespace/cursor
	dlqDir         subspace.Subspace // automation namespace/dlq

	// Runtime
	workerID   [16]byte
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	errCh      chan error
	pollTicker *time.Ticker
}

// AutomationOption configures an Automation
type AutomationOption[Deps any] func(*Automation[Deps])

// WithNumWorkers sets the number of worker goroutines
func WithNumWorkers[Deps any](n int) AutomationOption[Deps] {
	return func(a *Automation[Deps]) {
		if n > 0 {
			a.config.NumWorkers = n
		}
	}
}

// WithLeaseTTL sets the lease timeout for jobs
func WithLeaseTTL[Deps any](d time.Duration) AutomationOption[Deps] {
	return func(a *Automation[Deps]) {
		if d > 0 {
			a.config.LeaseTTL = d
		}
	}
}

// WithGracePeriod sets the grace period for job completion
func WithGracePeriod[Deps any](d time.Duration) AutomationOption[Deps] {
	return func(a *Automation[Deps]) {
		if d > 0 {
			a.config.GracePeriod = d
		}
	}
}

// WithMaxAttempts sets the maximum retry attempts before DLQ
func WithMaxAttempts[Deps any](n int) AutomationOption[Deps] {
	return func(a *Automation[Deps]) {
		if n > 0 {
			a.config.MaxAttempts = n
		}
	}
}

// WithBatchSize sets the batch size for event polling
func WithBatchSize[Deps any](n int) AutomationOption[Deps] {
	return func(a *Automation[Deps]) {
		if n > 0 {
			a.config.BatchSize = n
		}
	}
}

// WithPollInterval sets the polling interval for events
func WithPollInterval[Deps any](d time.Duration) AutomationOption[Deps] {
	return func(a *Automation[Deps]) {
		if d > 0 {
			a.config.PollInterval = d
		}
	}
}

// WithRetryBaseWait sets the base wait time for retry backoff
func WithRetryBaseWait[Deps any](d time.Duration) AutomationOption[Deps] {
	return func(a *Automation[Deps]) {
		if d > 0 {
			a.config.RetryBaseWait = d
		}
	}
}

// NewAutomation creates a new automation instance
func NewAutomation[Deps any](
	store dcb.DcbStore,
	deps Deps,
	queueId string,
	eventTypeExample any,
	handler func(Event) CommandWithEffect[Deps],
	opts ...AutomationOption[Deps],
) (*Automation[Deps], error) {
	if handler == nil {
		return nil, errors.New("handler is required")
	}
	if store == nil {
		return nil, errors.New("store is required")
	}

	db := store.Database()
	dcbNamespace := store.Namespace()
	runner := NewCommandWithEffectRunner(store, deps)

	// Resolve event type name
	eventType := resolveEventTypeName(eventTypeExample)

	// Build subspaces
	dcbRoot := subspace.Sub(dcbNamespace)
	automationRoot := subspace.Sub(dcbNamespace + "/" + queueId)

	// Generate worker ID
	var workerID [16]byte
	if _, err := rand.Read(workerID[:]); err != nil {
		return nil, fmt.Errorf("generate worker ID: %w", err)
	}

	// Create event registry and register the event type
	registry := newEventRegistry()
	registry.types[eventType] = reflect.TypeOf(eventTypeExample)

	a := &Automation[Deps]{
		queueId:        queueId,
		eventType:      eventType,
		eventRegistry:  registry,
		handler:        handler,
		runner:         runner,
		config:         defaultConfig(),
		db:             db,
		typeIndex:      dcbRoot.Sub("t").Sub(eventType),
		eventsSubspace: dcbRoot.Sub("e"),
		queueDir:       automationRoot.Sub("queue"),
		cursorKey:      automationRoot.Pack(tuple.Tuple{"cursor"}),
		dlqDir:         automationRoot.Sub("dlq"),
		workerID:       workerID,
		errCh:          make(chan error, 100),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}

// Start begins the automation processing
func (a *Automation[Deps]) Start(ctx context.Context) error {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.pollTicker = time.NewTicker(a.config.PollInterval)

	// Start watcher goroutine
	a.wg.Add(1)
	go a.runWatcher()

	// Start worker goroutines
	for range a.config.NumWorkers {
		a.wg.Add(1)
		go a.runWorker()
	}

	return nil
}

// Stop gracefully stops the automation
func (a *Automation[Deps]) Stop() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.pollTicker != nil {
		a.pollTicker.Stop()
	}
}

// Wait blocks until all workers have finished
func (a *Automation[Deps]) Wait() error {
	a.wg.Wait()
	close(a.errCh)

	// Collect any errors
	var errs []error
	for err := range a.errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// QueueId returns the queue identifier for this automation
func (a *Automation[Deps]) QueueId() string {
	return a.queueId
}

// Errors returns the error channel for monitoring
func (a *Automation[Deps]) Errors() <-chan error {
	return a.errCh
}
