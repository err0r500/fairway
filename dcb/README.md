# DCB
Low-level access to the eventstore

It exposes a [DCB](https://dcb.events) compliant storage interface backed by [foundationdb](https://www.foundationdb.org)
```go
type DcbStore interface {
	Append(ctx context.Context, events []Event, condition *AppendCondition) error
	Read(ctx context.Context, query Query, opts *ReadOptions) iter.Seq2[StoredEvent, error]
	ReadAll(ctx context.Context) iter.Seq2[StoredEvent, error]
}
```

## Additional docs 
- [foundationdb eventstore layout](./_doc/fdb_storage.md)
- [how read iterator is kept memory efficient](./_doc/read_streaming_design.md)

## Development
For dev and in order to run tests, the build flags must include "test"

```
export GOFLAGS="-tags=test"

then :
nvim or go test -v ./...
```
