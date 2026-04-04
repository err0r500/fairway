defmodule Fairway.Test.Commands.CreateList do
  @moduledoc "Pure command: create a list (idempotency check via DCB)."

  alias Fairway.Test.Events.ListCreated

  def run(ra, %{list_id: list_id, name: name}) do
    query = Fairway.Query.new([
      Fairway.QueryItem.new(types: [ListCreated], tags: ["list:#{list_id}"])
    ])

    {ra, exists} =
      Fairway.ReadAppender.fold_events(ra, query, false, fn
        %{data: %ListCreated{}}, _ -> {false, true}
        _, _ -> {true, false}
      end)

    if exists do
      {:error, :already_exists}
    else
      Fairway.ReadAppender.append_events(ra, [
        Fairway.Event.new(%ListCreated{list_id: list_id, name: name})
      ])
    end
  end
end

defmodule Fairway.Test.Commands.AddItem do
  @moduledoc "Pure command: add an item to a list (requires list to exist)."

  alias Fairway.Test.Events.{ListCreated, ItemAdded}

  def run(ra, %{list_id: list_id, item_id: item_id, name: name}) do
    query = Fairway.Query.new([
      Fairway.QueryItem.new(types: [ListCreated], tags: ["list:#{list_id}"])
    ])

    {ra, list_exists} =
      Fairway.ReadAppender.fold_events(ra, query, false, fn
        %{data: %ListCreated{}}, _ -> {false, true}
        _, acc -> {true, acc}
      end)

    if not list_exists do
      {:error, :list_not_found}
    else
      Fairway.ReadAppender.append_events(ra, [
        Fairway.Event.new(%ItemAdded{list_id: list_id, item_id: item_id, name: name})
      ])
    end
  end
end
