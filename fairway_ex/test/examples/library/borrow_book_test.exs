defmodule Library.Change.BorrowBookTest do
  use ExUnit.Case, async: true

  alias Fairway.Testing.Store
  alias Fairway.CommandRunner
  alias Library.Change.BorrowBook
  alias Library.Event.{BookBorrowed, BookReturned}

  setup do
    Fairway.Registry.register_all([BookBorrowed, BookReturned])
    {:ok, store} = Store.start_link()
    runner = CommandRunner.new(store, Store)
    %{store: store, runner: runner}
  end

  test "borrowing a book succeeds when it is available", %{runner: runner} do
    assert :ok =
             CommandRunner.run_pure(runner, BorrowBook, %{
               book_id: "book-1",
               borrower_id: "alice"
             })
  end

  test "returns :book_already_borrowed when the book is out", %{runner: runner} do
    :ok = CommandRunner.run_pure(runner, BorrowBook, %{book_id: "book-1", borrower_id: "alice"})

    assert {:error, :book_already_borrowed} =
             CommandRunner.run_pure(runner, BorrowBook, %{
               book_id: "book-1",
               borrower_id: "bob"
             })
  end

  test "book can be borrowed again after being returned", %{store: store, runner: runner} do
    :ok = CommandRunner.run_pure(runner, BorrowBook, %{book_id: "book-1", borrower_id: "alice"})

    # Return the book directly via given_events (wire format: JSON envelope)
    Store.given_events(store, [
      %{
        type: BookReturned.type_name(),
        tags: BookReturned.tags(%Library.Event.BookReturned{book_id: "book-1", borrower_id: "alice"}),
        data: Jason.encode!(%{
          occurred_at: DateTime.to_iso8601(DateTime.utc_now()),
          data: %{book_id: "book-1", borrower_id: "alice"}
        })
      }
    ])

    assert :ok =
             CommandRunner.run_pure(runner, BorrowBook, %{
               book_id: "book-1",
               borrower_id: "bob"
             })
  end

  test "returns :borrower_limit_reached when borrower holds max books", %{runner: runner} do
    max = BorrowBook.max_books_per_borrower()

    for i <- 1..max do
      :ok =
        CommandRunner.run_pure(runner, BorrowBook, %{
          book_id: "book-#{i}",
          borrower_id: "alice"
        })
    end

    assert {:error, :borrower_limit_reached} =
             CommandRunner.run_pure(runner, BorrowBook, %{
               book_id: "book-#{max + 1}",
               borrower_id: "alice"
             })
  end

  test "concurrent borrows of the same book — exactly one succeeds", %{store: store} do
    runner = CommandRunner.new(store, Store, max_attempts: 5)
    borrower_count = 5

    tasks =
      for i <- 1..borrower_count do
        Task.async(fn ->
          CommandRunner.run_pure(runner, BorrowBook, %{
            book_id: "book-1",
            borrower_id: "borrower-#{i}"
          })
        end)
      end

    results = Task.await_many(tasks)

    assert Enum.count(results, &(&1 == :ok)) == 1

    assert Enum.count(results, &(&1 == {:error, :book_already_borrowed})) ==
             borrower_count - 1
  end
end
