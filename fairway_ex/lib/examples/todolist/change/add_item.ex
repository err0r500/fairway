defmodule Todolist.Change.AddItem do
  @moduledoc """
  Pure command: add an item to an existing todo list.

  Checks whether the item already exists (by `item_id` + `list_id`) via DCB.
  Returns `{:error, :already_exists}` if the item was already added.
  """

  alias Fairway.{Query, QueryItem, ReadAppender, Event}
  alias Todolist.Event.ItemCreated

  def run(ra, %{list_id: list_id, item_id: item_id, text: text}) do
    query = Query.new([
      QueryItem.new(
        types: [ItemCreated],
        tags: ["list_id:#{list_id}", "item_id:#{item_id}"]
      )
    ])

    {ra, exists} =
      ReadAppender.fold_events(ra, query, false, fn
        %{data: %ItemCreated{}}, _ -> {false, true}
        _, acc -> {true, acc}
      end)

    if exists do
      {:error, :already_exists}
    else
      ReadAppender.append_events(ra, [
        Event.new(%ItemCreated{list_id: list_id, item_id: item_id, text: text})
      ])
    end
  end
end
