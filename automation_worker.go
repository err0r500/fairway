package fairway

import (
	"encoding/binary"
	"fmt"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/err0r500/fairway/dcb"
)

// runWorker is the main worker loop
func (a *Automation[Deps]) runWorker() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		job, err := a.dequeue()
		if err == ErrNoJobs {
			select {
			case <-a.ctx.Done():
				return
			case <-a.pollTicker.C:
				continue
			}
		}
		if err != nil {
			select {
			case a.errCh <- fmt.Errorf("dequeue: %w", err):
			default:
			}
			continue
		}

		a.processJob(job)
	}
}

// processJob handles a single job
func (a *Automation[Deps]) processJob(job *Job) {
	// Fetch event from dcb using versionstamp
	storedEvent, err := a.fetchEvent(job.EventVS)
	if err != nil {
		a.handleJobFailure(job, fmt.Errorf("fetch event: %w", err))
		return
	}

	// Deserialize event using registry
	event, err := a.eventRegistry.deserialize(storedEvent.Event)
	if err != nil {
		a.handleJobFailure(job, fmt.Errorf("deserialize: %w", err))
		return
	}

	// Call handler to get command
	cmd := a.handler(event)
	if cmd == nil {
		// Handler returned nil, just delete the job
		if err := a.deleteJob(job); err != nil {
			select {
			case a.errCh <- fmt.Errorf("delete job: %w", err):
			default:
			}
		}
		return
	}

	// Execute command
	processErr := a.runner.RunWithEffect(a.ctx, cmd)

	if processErr != nil {
		a.handleJobFailure(job, processErr)
		return
	}

	// Success - delete the job
	if err := a.deleteJob(job); err != nil {
		select {
		case a.errCh <- fmt.Errorf("delete job after success: %w", err):
		default:
		}
	}
}

// handleJobFailure handles a failed job processing attempt
func (a *Automation[Deps]) handleJobFailure(job *Job, processErr error) {
	if err := a.retryJob(job, processErr); err != nil {
		select {
		case a.errCh <- fmt.Errorf("retry job: %w (original: %w)", err, processErr):
		default:
		}
	}
}

// fetchEvent retrieves an event from dcb by versionstamp
func (a *Automation[Deps]) fetchEvent(vs dcb.Versionstamp) (dcb.StoredEvent, error) {
	var result dcb.StoredEvent

	_, err := a.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		// Convert dcb.Versionstamp to tuple.Versionstamp
		var txVersion [10]byte
		copy(txVersion[:], vs[:10])
		userVersion := binary.BigEndian.Uint16(vs[10:12])
		tupleVs := tuple.Versionstamp{TransactionVersion: txVersion, UserVersion: userVersion}

		eventKey := a.eventsSubspace.Pack(tuple.Tuple{tupleVs})
		encodedValue := tr.Get(eventKey).MustGet()

		if encodedValue == nil {
			return nil, fmt.Errorf("event not found at versionstamp %x", vs[:])
		}

		// Decode event (type, tags, data)
		eventTuple, err := tuple.Unpack(encodedValue)
		if err != nil {
			return nil, fmt.Errorf("unpack event: %w", err)
		}

		if len(eventTuple) != 3 {
			return nil, fmt.Errorf("expected 3-tuple, got %d elements", len(eventTuple))
		}

		eventType, ok := eventTuple[0].(string)
		if !ok {
			return nil, fmt.Errorf("type field is %T, expected string", eventTuple[0])
		}

		var tags []string
		if eventTuple[1] != nil {
			tagsTuple, ok := eventTuple[1].(tuple.Tuple)
			if !ok {
				return nil, fmt.Errorf("tags field is %T, expected tuple", eventTuple[1])
			}
			tags = make([]string, len(tagsTuple))
			for i, t := range tagsTuple {
				tags[i] = t.(string)
			}
		}

		eventData, ok := eventTuple[2].([]byte)
		if !ok {
			return nil, fmt.Errorf("data field is %T, expected []byte", eventTuple[2])
		}

		result = dcb.StoredEvent{
			Event:    dcb.Event{Type: eventType, Tags: tags, Data: eventData},
			Position: vs,
		}
		return nil, nil
	})

	return result, err
}
