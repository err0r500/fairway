package library

import (
	"context"
	"errors"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
)

// EVENTS

type BookBorrowed struct {
	BookId     string `json:"book_id"`
	BorrowerId string `json:"borrower_id"`
}

func (e BookBorrowed) Tags() []string {
	return []string{"book_id:" + e.BookId, "borrower_id:" + e.BorrowerId}
}

type BookReturned struct {
	BookId     string `json:"book_id"`
	BorrowerId string `json:"borrower_id"`
}

func (e BookReturned) Tags() []string {
	return []string{"book_id:" + e.BookId, "borrower_id:" + e.BorrowerId}
}

// COMMAND

var ErrBookAlreadyBorrowed = errors.New("book is already borrowed")
var ErrBorrowerLimitReached = errors.New("borrower has reached max 5 books")

type BorrowBook struct {
	BookId     string
	BorrowerId string
}

const maxBooksPerBorrower = 5

func (cmd BorrowBook) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	if err := cmd.ensureBookNotCurrentlyBorrowed(ctx, ev); err != nil {
		return err
	}

	if err := cmd.ensureReaderBelowMaxBorrowedBooksLimit(ctx, ev); err != nil {
		return err
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(BookBorrowed{
		BookId:     cmd.BookId,
		BorrowerId: cmd.BorrowerId,
	}))
}

func (cmd BorrowBook) ensureBookNotCurrentlyBorrowed(ctx context.Context, ev fairway.EventsReader) error {
	isBorrowed := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(BookBorrowed{}, BookReturned{}).
				Tags("book_id:"+cmd.BookId),
		).WithOptions(dcb.ReadOptions{Reverse: true, Limit: 1}),
		func(e fairway.Event) bool {
			_, isBorrowed = e.Data.(BookBorrowed)
			return false // only need first event
		}); err != nil {
		return err
	}

	if isBorrowed {
		return ErrBookAlreadyBorrowed
	}

	return nil
}

func (cmd BorrowBook) ensureReaderBelowMaxBorrowedBooksLimit(ctx context.Context, ev fairway.EventsReader) error {
	borrowerBooks := make(map[string]bool)
	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(BookBorrowed{}, BookReturned{}).
				Tags("borrower_id:"+cmd.BorrowerId),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case BookBorrowed:
				borrowerBooks[data.BookId] = true
			case BookReturned:
				delete(borrowerBooks, data.BookId)
			}
			return true
		}); err != nil {
		return err
	}

	if len(borrowerBooks) >= maxBooksPerBorrower {
		return ErrBorrowerLimitReached
	}

	return nil
}
