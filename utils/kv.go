package utils

import (
	"encoding/json"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

// KV provides key-value operations scoped to a subspace
type KV struct {
	tr    fdb.Transaction
	space subspace.Subspace
}

// NewKV creates a new KV helper
func NewKV(tr fdb.Transaction, space subspace.Subspace) KV {
	return KV{tr: tr, space: space}
}

func pathToTuple(p []string) tuple.Tuple {
	t := make(tuple.Tuple, len(p))
	for i, s := range p {
		t[i] = s
	}
	return t
}

// SetPath sets an empty marker at the given path
func (k KV) SetPath(p []string) {
	k.tr.Set(k.space.Pack(pathToTuple(p)), nil)
}

// ClearPath removes the value at the given path
func (k KV) ClearPath(p []string) {
	k.tr.Clear(k.space.Pack(pathToTuple(p)))
}

// ScanPath returns all keys with the given prefix
func (k KV) ScanPath(prefix []string) [][]string {
	kvs := k.tr.GetRange(k.space.Sub(pathToTuple(prefix)...), fdb.RangeOptions{}).GetSliceOrPanic()
	results := make([][]string, 0, len(kvs))
	for _, kv := range kvs {
		keyTuple, err := k.space.Unpack(kv.Key)
		if err != nil {
			continue
		}
		path := make([]string, len(keyTuple))
		for i, elem := range keyTuple {
			if str, ok := elem.(string); ok {
				path[i] = str
			}
		}
		results = append(results, path)
	}
	return results
}

// ClearPrefix removes all keys with the given prefix
func (k KV) ClearPrefix(prefix []string) {
	k.tr.ClearRange(k.space.Sub(pathToTuple(prefix)...))
}

// SetJSON marshals v to JSON and stores it at the given path
func (k KV) SetJSON(p []string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	k.tr.Set(k.space.Pack(pathToTuple(p)), data)
	return nil
}

// GetJSON reads and unmarshals JSON from the given path
func (k KV) GetJSON(p []string, v any) error {
	data := k.tr.Get(k.space.Pack(pathToTuple(p))).MustGet()
	if data == nil {
		return nil
	}
	return json.Unmarshal(data, v)
}
