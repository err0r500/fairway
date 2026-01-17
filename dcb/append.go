package dcb

import (
	"context"
	"errors"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

// Append atomically appends events with optional condition checking
// Returns error if condition fails or any other error occurs
func (s fdbStore) Append(ctx context.Context, events []Event, condition *AppendCondition) error {
	return s.appendInternal(ctx, events, condition, nil)
}

// appendInternal is the internal implementation of Append with an optional test hook
// afterQueryHook is called after queryExists with the result (for testing only)
//
// Note: The FDB Go binding does not support context cancellation during transactions.
// This function performs best-effort checks before and during the transaction, but
// if ctx is cancelled during transaction commit, the transaction may still succeed.
func (s fdbStore) appendInternal(ctx context.Context, events []Event, condition *AppendCondition, afterQueryHook func(exists bool)) error {
	// Check context before starting
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(events) == 0 {
		s.metrics.RecordError("append", "empty_events")
		return ErrEmptyEvents
	}

	start := time.Now()

	// Validate events
	for _, event := range events {
		if event.Type == "" {
			s.logger.Error("event validation", errors.New("event with empty string type provided"))
			return errors.New("event must have a type")
		}
		// Ensure no duplicate tags
		tagSet := make(map[string]bool)
		for _, tag := range event.Tags {
			if tagSet[tag] {
				s.logger.Error("event validation", errors.New("event has duplicate tags"))
			}
			tagSet[tag] = true
		}
	}

	// Execute append in transaction
	_, err := s.db.Transact(func(tr fdb.Transaction) (any, error) {
		// Best-effort check for context cancellation
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Check append condition if specified
		if condition != nil {
			exists, err := s.queryExists(tr, condition.Query, condition.After)
			if err != nil {
				return nil, err
			}

			// Test hook: allow tests to pause here to force conflict scenarios
			if afterQueryHook != nil {
				afterQueryHook(exists)
			}

			if exists {
				return nil, ErrAppendConditionFailed
			}
		}

		// Append each event
		for i, event := range events {
			if err := s.appendSingle(tr, event, uint16(i)); err != nil {
				return nil, err
			}
		}

		// Transaction commits when Transact returns nil
		return nil, nil
	})

	duration := time.Since(start)
	success := err == nil

	s.metrics.RecordAppendDuration(duration, success)
	if success {
		s.metrics.RecordAppendEvents(len(events))
		s.logger.Info("append completed", "event_count", len(events), "duration", duration)
	} else {
		s.logger.Error("append failed", err, "event_count", len(events), "duration", duration)
	}

	return err
}

// appendSingle writes a single event with all its indexes
func (s fdbStore) appendSingle(tr fdb.Transaction, event Event, batchIndex uint16) error {
	// Create incomplete versionstamp
	vs := tuple.IncompleteVersionstamp(batchIndex)

	// 1. Write primary event storage (encode type, tags, and data together)
	// Convert []string tags to tuple.Tuple for encoding
	tagsTuple := make(tuple.Tuple, len(event.Tags))
	for i, tag := range event.Tags {
		tagsTuple[i] = tag
	}
	eventValue := tuple.Tuple{event.Type, tagsTuple, event.Data}.Pack()
	eventKey, err := s.events.PackWithVersionstamp(tuple.Tuple{vs})
	if err != nil {
		return err
	}
	tr.SetVersionstampedKey(eventKey, eventValue)

	// 2. Write to type index
	typeKey, err := s.byType.Sub(event.Type).PackWithVersionstamp(tuple.Tuple{vs})
	if err != nil {
		return err
	}
	tr.SetVersionstampedKey(typeKey, nil)

	// 3. Write to tag tree (all subsets with alphabetical ordering)
	// Only write tag indexes if event has tags
	subsets := generateAllSubsets(event.Tags)
	for _, subset := range subsets {
		tagPath := make(tuple.Tuple, 0, len(subset)+3)
		for _, tag := range subset {
			tagPath = append(tagPath, tag)
		}
		tagPath = append(tagPath, eventsInTagSubspace, event.Type, vs)

		tagKey, err := s.byTag.PackWithVersionstamp(tagPath)
		if err != nil {
			return err
		}
		tr.SetVersionstampedKey(tagKey, nil)
	}

	return nil
}

// queryExists checks if any events match the query
func (s fdbStore) queryExists(tr fdb.ReadTransaction, query Query, after *Versionstamp) (bool, error) {
	for _, item := range query.Items {
		exists, err := s.queryItemExists(tr, item, after)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil // OR semantics: any item match means exists
		}
	}
	return false, nil
}

// queryItemExists checks if any events match a single query item
func (s fdbStore) queryItemExists(tr fdb.ReadTransaction, item QueryItem, after *Versionstamp) (bool, error) {
	ranges, err := s.buildQueryRanges(tr, item, after)
	if err != nil {
		return false, err
	}

	// Check if any range has at least one event
	for _, r := range ranges {
		iter := tr.GetRange(r, fdb.RangeOptions{Limit: 1}).Iterator()
		if iter.Advance() {
			return true, nil
		}
	}

	return false, nil
}
