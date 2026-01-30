package fairway

import (
	"encoding/binary"
	"fmt"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/err0r500/fairway/dcb"
)

// runWatcher polls for new events and enqueues them
func (a *Automation[Deps]) runWatcher() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.pollTicker.C:
			if err := a.pollAndEnqueue(); err != nil {
				select {
				case a.errCh <- fmt.Errorf("poll and enqueue: %w", err):
				default:
				}
			}
		}
	}
}

// pollAndEnqueue reads new events from type index and enqueues them
func (a *Automation[Deps]) pollAndEnqueue() error {
	_, err := a.db.Transact(func(tr fdb.Transaction) (any, error) {
		// 1. Read cursor
		cursorValue := tr.Get(a.cursorKey).MustGet()
		var cursor *dcb.Versionstamp
		if cursorValue != nil && len(cursorValue) == 12 {
			var vs dcb.Versionstamp
			copy(vs[:], cursorValue)
			cursor = &vs
		}

		// 2. Build range for type index
		var r fdb.Range
		if cursor != nil {
			rng, err := rangeAfterVersionstamp(a.typeIndex, *cursor)
			if err != nil {
				return nil, err
			}
			r = rng
		} else {
			r = a.typeIndex
		}

		// 3. Read from type index
		kvs := tr.GetRange(r, fdb.RangeOptions{Limit: a.config.BatchSize}).GetSliceOrPanic()

		if len(kvs) == 0 {
			return nil, nil
		}

		// 4. For each event versionstamp: enqueue
		var lastVS dcb.Versionstamp
		for _, kv := range kvs {
			vs := extractVersionstampFromTypeIndex(a.typeIndex, kv.Key)
			if vs == (dcb.Versionstamp{}) {
				continue
			}

			if err := a.enqueueInTx(tr, vs); err != nil {
				return nil, err
			}
			lastVS = vs
		}

		// 5. Update cursor (same tx = atomic)
		if lastVS != (dcb.Versionstamp{}) {
			tr.Set(a.cursorKey, lastVS[:])
		}

		return nil, nil
	})
	return err
}

// rangeAfterVersionstamp creates an FDB range that starts after the given versionstamp
func rangeAfterVersionstamp(ss subspace.Subspace, after dcb.Versionstamp) (fdb.Range, error) {
	var txVersion [10]byte
	copy(txVersion[:], after[:10])
	userVersion := binary.BigEndian.Uint16(after[10:12])
	tupleVs := tuple.Versionstamp{TransactionVersion: txVersion, UserVersion: userVersion}

	// Create begin key (exclusive of 'after')
	beginKey := ss.Pack(tuple.Tuple{tupleVs})
	beginKeyExclusive := append(fdb.Key(beginKey), 0x00)

	// End key is end of subspace
	_, endKey := ss.FDBRangeKeys()

	return fdb.KeyRange{Begin: beginKeyExclusive, End: endKey}, nil
}

// extractVersionstampFromTypeIndex extracts versionstamp from type index key
// Type index key format: namespace/t/<type>/<versionstamp>
func extractVersionstampFromTypeIndex(typeIndex subspace.Subspace, key fdb.Key) dcb.Versionstamp {
	keyTuple, err := typeIndex.Unpack(key)
	if err != nil || len(keyTuple) == 0 {
		return dcb.Versionstamp{}
	}

	// Last element is the versionstamp
	tupleVs, ok := keyTuple[len(keyTuple)-1].(tuple.Versionstamp)
	if !ok {
		return dcb.Versionstamp{}
	}

	var vs dcb.Versionstamp
	copy(vs[:10], tupleVs.TransactionVersion[:])
	binary.BigEndian.PutUint16(vs[10:12], tupleVs.UserVersion)
	return vs
}
