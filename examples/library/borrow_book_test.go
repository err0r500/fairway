package library_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/library"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
)

func TestBorrowBook_ConcurrentBorrowers_OnlyOneSucceeds(t *testing.T) {
	t.Parallel()

	store := given.SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)

	bookId := "book-1"
	borrowerCount := 5

	var wg sync.WaitGroup
	results := make(chan error, borrowerCount)

	for i := range borrowerCount {
		wg.Add(1)
		go func(borrowerId int) {
			defer wg.Done()
			cmd := library.BorrowBook{
				BookId:     bookId,
				BorrowerId: string(rune('A' + borrowerId)),
			}
			results <- runner.RunPure(context.Background(), cmd)
		}(i)
	}

	wg.Wait()
	close(results)

	successCount := 0
	alreadyBorrowedCount := 0
	for err := range results {
		if err == nil {
			successCount++
		} else if errors.Is(err, library.ErrBookAlreadyBorrowed) {
			alreadyBorrowedCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}

	assert.Equal(t, 1, successCount, "exactly 1 borrower should succeed")
	assert.Equal(t, borrowerCount-1, alreadyBorrowedCount, "others should get ErrBookAlreadyBorrowed")
}

func TestBorrowBook_BorrowerLimitReached(t *testing.T) {
	t.Parallel()

	store := given.SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)

	borrowerId := "borrower-1"

	// borrow 5 books
	for i := range 5 {
		err := runner.RunPure(context.Background(), library.BorrowBook{
			BookId:     fmt.Sprintf("book-%d", i),
			BorrowerId: borrowerId,
		})
		assert.NoError(t, err)
	}

	// 6th should fail
	err := runner.RunPure(context.Background(), library.BorrowBook{
		BookId:     "book-6",
		BorrowerId: borrowerId,
	})
	assert.ErrorIs(t, err, library.ErrBorrowerLimitReached)
}
