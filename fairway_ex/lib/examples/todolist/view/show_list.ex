defmodule Todolist.View.ShowList do
  @moduledoc """
  View: project a todo list from its events.

  Reads `ListCreated` and `ItemCreated` events for a given `list_id` and
  returns a `%ShowList{}` with the list name and item count.

  Returns `{:error, :not_found}` when no `ListCreated` event exists for the id.

  ## Example

      {:ok, result} = Todolist.View.ShowList.get(store, Fairway.Testing.Store, "list-1")
      result.name        #=> "Shopping"
      result.items_count #=> 2
  """

  alias Fairway.{Query, QueryItem, View}
  alias Todolist.Event.{ListCreated, ItemCreated}

  defstruct id: nil, name: nil, items_count: 0

  @type t :: %__MODULE__{
          id: String.t() | nil,
          name: String.t() | nil,
          items_count: non_neg_integer()
        }

  @doc "Build the list view from the event store. Returns `{:ok, t()} | {:error, :not_found}`."
  def get(store, store_module, list_id) do
    query = Query.new([
      QueryItem.new(
        types: [ListCreated, ItemCreated],
        tags: ["list_id:#{list_id}"]
      )
    ])

    case View.read_events(store, store_module, query, %__MODULE__{}, fn
      %{data: %ListCreated{list_id: id, name: name}}, acc ->
        %{acc | id: id, name: name}

      %{data: %ItemCreated{}}, acc ->
        %{acc | items_count: acc.items_count + 1}

      _, acc ->
        acc
    end) do
      {:ok, %__MODULE__{id: nil}} -> {:error, :not_found}
      {:ok, result} -> {:ok, result}
      {:error, _} = err -> err
    end
  end
end
