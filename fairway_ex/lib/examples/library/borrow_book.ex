defmodule Library.Event.BookBorrowed do
  @moduledoc "A book was borrowed."

  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:book_id, :borrower_id]

  @impl true
  def tags(%{book_id: bid, borrower_id: rid}), do: ["book_id:#{bid}", "borrower_id:#{rid}"]

  @impl true
  def type_name, do: "Library.BookBorrowed"
end

defmodule Library.Event.BookReturned do
  @moduledoc "A borrowed book was returned."

  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:book_id, :borrower_id]

  @impl true
  def tags(%{book_id: bid, borrower_id: rid}), do: ["book_id:#{bid}", "borrower_id:#{rid}"]

  @impl true
  def type_name, do: "Library.BookReturned"
end

defmodule Library.Change.BorrowBook do
  @moduledoc """
  Pure command: borrow a book from the library.

  Two constraints are enforced via DCB:
  1. The book must not currently be borrowed (checked by looking at the most
     recent book-scoped event — if it's a `BookBorrowed`, the book is out).
  2. The borrower must not have reached the maximum of #{Library.Change.BorrowBook.max_books_per_borrower()}
     currently borrowed books.

  Both checks read different slices of the event log and each contributes
  its own `AppendCondition`, so a conflicting concurrent write to either
  slice causes a clean retry.
  """

  alias Fairway.{Query, QueryItem, ReadAppender, Event}
  alias Library.Event.{BookBorrowed, BookReturned}

  @max_books 5
  def max_books_per_borrower, do: @max_books

  def run(ra, %{book_id: book_id, borrower_id: borrower_id}) do
    with {:ok, ra} <- ensure_book_available(ra, book_id),
         {:ok, ra} <- ensure_borrower_below_limit(ra, borrower_id) do
      ReadAppender.append_events(ra, [
        Event.new(%BookBorrowed{book_id: book_id, borrower_id: borrower_id})
      ])
    end
  end

  # ── Private ──────────────────────────────────────────────────────────────────

  # Read the most recent event for this book (reverse: true, limit: 1).
  # If it's a BookBorrowed the book is still out; if BookReturned (or absent)
  # it is available.
  defp ensure_book_available(ra, book_id) do
    query =
      Query.new(
        [QueryItem.new(types: [BookBorrowed, BookReturned], tags: ["book_id:#{book_id}"])],
        %{reverse: true, limit: 1}
      )

    {ra, is_borrowed} =
      ReadAppender.fold_events(ra, query, false, fn
        %{data: %BookBorrowed{}}, _ -> {false, true}
        %{data: %BookReturned{}}, _ -> {false, false}
        _, acc -> {true, acc}
      end)

    if is_borrowed do
      {:error, :book_already_borrowed}
    else
      {:ok, ra}
    end
  end

  # Count how many books the borrower currently holds (borrowed minus returned).
  defp ensure_borrower_below_limit(ra, borrower_id) do
    query =
      Query.new([
        QueryItem.new(
          types: [BookBorrowed, BookReturned],
          tags: ["borrower_id:#{borrower_id}"]
        )
      ])

    {ra, held} =
      ReadAppender.fold_events(ra, query, MapSet.new(), fn
        %{data: %BookBorrowed{book_id: bid}}, acc -> {true, MapSet.put(acc, bid)}
        %{data: %BookReturned{book_id: bid}}, acc -> {true, MapSet.delete(acc, bid)}
        _, acc -> {true, acc}
      end)

    if MapSet.size(held) >= @max_books do
      {:error, :borrower_limit_reached}
    else
      {:ok, ra}
    end
  end
end
