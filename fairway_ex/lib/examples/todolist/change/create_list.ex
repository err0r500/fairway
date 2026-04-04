defmodule Todolist.Change.CreateList do
  @moduledoc """
  Pure command: create a new todo list.

  Checks for an existing list via DCB before appending. If a list with the
  same `list_id` already exists the command returns `{:error, :already_exists}`.
  On concurrent creation attempts for the same list, exactly one will succeed
  and the other will see `{:error, :already_exists}` after retry.
  """

  alias Fairway.{Query, QueryItem, ReadAppender, Event}
  alias Todolist.Event.ListCreated

  def run(ra, %{list_id: list_id, name: name}) do
    query = Query.new([
      QueryItem.new(types: [ListCreated], tags: ["list_id:#{list_id}"])
    ])

    {ra, exists} =
      ReadAppender.fold_events(ra, query, false, fn
        %{data: %ListCreated{}}, _ -> {false, true}
        _, acc -> {true, acc}
      end)

    if exists do
      {:error, :already_exists}
    else
      ReadAppender.append_events(ra, [
        Event.new(%ListCreated{list_id: list_id, name: name})
      ])
    end
  end
end
