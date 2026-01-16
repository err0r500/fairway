package fairway_test

import (
	"context"
	"fmt"
	"log"
	"testing"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/google/uuid"
)

type TodoListCreated struct {
	ListId string
}

type TodoListDeleted struct {
	ListId string
}

type TodoItemAdded struct {
	ListId string
	ItemId string
	Text   string
}

type AddItem struct {
	ListId string
	ItemId string
	Text   string
}

func (cmd AddItem) Run(ctx context.Context, ra fairway.EventReadAppender) error {
	var count int

	handler := fairway.Query(
		fairway.QueryItem{}.Types(TodoListCreated{}, TodoListDeleted{}).Tags("list:" + cmd.ListId),
	).Handle(func(te fairway.TaggedEvent, err error) bool {
		if err != nil {
			return false
		}

		switch e := te.Event.(type) {
		case TodoListCreated:
			log.Println("received TodoListCreated", e.ListId)
			count += 1
		case TodoListDeleted:
			log.Println("received TodoListDeleted", e.ListId)
			count += 1
		}
		return true
	})

	if err := ra.ReadEvents(ctx, handler); err != nil {
		return err
	}

	log.Println("count: ", count)

	return ra.AppendEvents(ctx, fairway.NewEvent(
		TodoItemAdded{
			ListId: cmd.ListId,
			ItemId: cmd.ItemId,
			Text:   cmd.Text,
		},
		fmt.Sprintf("list:%s", cmd.ListId),
		fmt.Sprintf("item:%s", cmd.ItemId),
	))
}

func TestHello(t *testing.T) {
	store := SetupTestStore(t)

	e := fairway.NewEvent(TodoListCreated{ListId: "listid"}, "list:listid")
	if err := fairway.NewReadAppender(store).AppendEvents(t.Context(), e, e); err != nil {
		t.Fatal("initial append error", err)
	}

	if err := fairway.NewCommandRunner(store).Run(
		t.Context(),
		AddItem{ListId: "listid", ItemId: "itemId", Text: "text"},
	); err != nil {
		t.Fatal(err)
	}
}

// commandDeps holds dependencies for commands with side effects
type commandDeps struct {
	Logger *log.Logger
}

// ArchiveList is a command with side effects using injected dependencies
type ArchiveList struct {
	ListId string
}

func (cmd ArchiveList) Run(ctx context.Context, ra fairway.EventReadAppender, deps commandDeps) error {
	var count int

	handler := fairway.Query(
		fairway.QueryItem{}.Types(TodoListCreated{}, TodoItemAdded{}).Tags("list:" + cmd.ListId),
	).Handle(func(te fairway.TaggedEvent, err error) bool {
		if err != nil {
			return false
		}
		count++
		return true
	})

	if err := ra.ReadEvents(ctx, handler); err != nil {
		return err
	}

	// Use injected dependency
	deps.Logger.Printf("Archiving list %s with %d events", cmd.ListId, count)

	return ra.AppendEvents(ctx, fairway.NewEvent(
		TodoListDeleted{ListId: cmd.ListId},
		fmt.Sprintf("list:%s", cmd.ListId),
	))
}

func TestCommandWithSideEffect(t *testing.T) {
	store := SetupTestStore(t)

	// Create initial events
	e := fairway.NewEvent(TodoListCreated{ListId: "list1"}, "list:list1")
	if err := fairway.NewReadAppender(store).AppendEvents(t.Context(), e); err != nil {
		t.Fatal("initial append error", err)
	}

	// Create runner with dependencies
	deps := commandDeps{Logger: log.Default()}
	runner := fairway.NewCommandWithEffectRunner(store, deps)

	// Execute command with side effects
	if err := runner.RunWithEffect(t.Context(), ArchiveList{ListId: "list1"}); err != nil {
		t.Fatal(err)
	}
}

func TestCommandWithEffectRunner_CanRunPureCommands(t *testing.T) {
	store := SetupTestStore(t)

	// Create initial events
	e := fairway.NewEvent(TodoListCreated{ListId: "list2"}, "list:list2")
	if err := fairway.NewReadAppender(store).AppendEvents(t.Context(), e, e); err != nil {
		t.Fatal("initial append error", err)
	}

	// Create runner with dependencies
	deps := commandDeps{Logger: log.Default()}
	runner := fairway.NewCommandWithEffectRunner(store, deps)

	// CommandWithEffectRunner can also run pure commands
	if err := runner.RunPure(t.Context(), AddItem{ListId: "list2", ItemId: "item1", Text: "test"}); err != nil {
		t.Fatal(err)
	}
}

func SetupTestStore(t *testing.T) dcb.DcbStore {
	t.Helper()

	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()

	// Use unique namespace per test
	namespace := fmt.Sprintf("t-%d", uuid.New())
	store := dcb.NewDcbStore(db, namespace)

	// Clean up after test
	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(namespace), End: fdb.Key(namespace + "\xff")})
			return nil, nil
		})
	})

	return store
}
