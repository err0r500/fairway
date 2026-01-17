package dcb

import "time"

// Logger defines the logging interface for the EventStore.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// noopLogger is a no-op implementation of Logger (default).
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// Metrics defines the observability interface for the EventStore.
type Metrics interface {
	// Append metrics
	RecordAppendDuration(duration time.Duration, success bool)
	RecordAppendEvents(count int)

	// Read metrics
	RecordReadDuration(duration time.Duration, success bool)
	RecordReadEvents(count int)

	// Error metrics
	RecordError(operation string, errorType string)
}

// noopMetrics is a no-op implementation of Metrics (default).
type noopMetrics struct{}

func (noopMetrics) RecordAppendDuration(time.Duration, bool) {}
func (noopMetrics) RecordAppendEvents(int)                   {}
func (noopMetrics) RecordReadDuration(time.Duration, bool)   {}
func (noopMetrics) RecordReadEvents(int)                     {}
func (noopMetrics) RecordError(string, string)               {}
