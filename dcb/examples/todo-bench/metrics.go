package main

import (
	"net/http"
	_ "net/http/pprof" // Auto-registers pprof endpoints
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Append metrics
	appendLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "dcb_append_duration_seconds",
		Help:    "Histogram of append operation latencies",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
	})

	appendTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dcb_append_total",
		Help: "Total number of append operations",
	}, []string{"status"}) // status = success or error

	// Read metrics
	readLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "dcb_read_duration_seconds",
		Help:    "Histogram of read operation latencies",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
	})

	readTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dcb_read_total",
		Help: "Total number of read operations",
	}, []string{"status"}) // status = success or error

	// Lock metrics
	lockWaitLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "dcb_lock_wait_duration_seconds",
		Help:    "Histogram of lock acquisition wait times",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
	})

	lockContentions = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "dcb_lock_contentions",
		Help:    "Number of contentions (poll attempts) before acquiring lock",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12), // 1 to 4096
	})

	// Scenario metrics
	scenariosCompleted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dcb_scenarios_completed_total",
		Help: "Total number of completed scenarios",
	})

	// Event metrics
	eventsAppended = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dcb_events_appended_total",
		Help: "Total number of events appended to the store",
	})

	eventsRead = promauto.NewCounter(prometheus.CounterOpts{
		Name: "dcb_events_read_total",
		Help: "Total number of events read from the store",
	})

	// Gauge for current metrics (for debugging)
	activeScenarios = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dcb_active_scenarios",
		Help: "Number of currently active scenarios",
	})
)

func init() {
	// Register metrics endpoint
	http.Handle("/metrics", promhttp.Handler())

	// pprof endpoints are automatically registered by importing _ "net/http/pprof"
	// Just importing it registers all the handlers, so we don't need to register them manually
}

// recordAppend records append operation metrics
func recordAppend(duration time.Duration, success bool) {
	appendLatency.Observe(duration.Seconds())

	status := "success"
	if !success {
		status = "error"
	}
	appendTotal.WithLabelValues(status).Inc()
}

// recordRead records read operation metrics
func recordRead(duration time.Duration, success bool) {
	readLatency.Observe(duration.Seconds())

	status := "success"
	if !success {
		status = "error"
	}
	readTotal.WithLabelValues(status).Inc()
}

// prometheusMetrics implements dcbtree.Metrics interface
type prometheusMetrics struct{}

func (prometheusMetrics) RecordAppendDuration(duration time.Duration, success bool) {
	recordAppend(duration, success)
}

func (prometheusMetrics) RecordAppendEvents(count int) {
	eventsAppended.Add(float64(count))
}

func (prometheusMetrics) RecordReadDuration(duration time.Duration, success bool) {
	recordRead(duration, success)
}

func (prometheusMetrics) RecordReadEvents(count int) {
	eventsRead.Add(float64(count))
}

func (prometheusMetrics) RecordError(operation string, errorType string) {}
