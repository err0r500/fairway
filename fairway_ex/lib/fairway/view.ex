defmodule Fairway.View do
  @moduledoc """
  Read-only event fold for building views (projections).

  Views call `read_events/5` to fold over events matching a query and build
  an arbitrary accumulator (a map, a list, a struct, etc.).

  Example:

      def show_list(store, store_module, list_id) do
        query = Fairway.Query.new([
          Fairway.QueryItem.new(
            types: [MyApp.Events.ListCreated, MyApp.Events.ItemAdded],
            tags: ["list:" <> list_id]
          )
        ])

        Fairway.View.read_events(store, store_module, query, %{name: nil, items: []}, fn
          %{data: %MyApp.Events.ListCreated{name: n}}, acc -> %{acc | name: n}
          %{data: %MyApp.Events.ItemAdded{item: i}}, acc -> Map.update!(acc, :items, &[i | &1])
          _, acc -> acc
        end)
      end
  """

  @doc """
  Fold events from the store matching `query` into `initial_acc`.

  `fun` is called as `fun.(event, acc) -> new_acc`.
  To stop early, return `{:halt, acc}` from `fun` (standard Enum.reduce_while style).

  Returns `{:ok, final_acc} | {:error, reason}`.
  """
  def read_events(store, store_module, %Fairway.Query{} = query, initial_acc, fun)
      when is_function(fun, 2) do
    wire_items = Fairway.Query.to_wire_items(query)
    read_opts = Fairway.Query.to_read_opts(query)

    case store_module.read_events(store, wire_items, read_opts) do
      {:ok, stored_events} ->
        result =
          Enum.reduce_while(stored_events, initial_acc, fn stored_event, acc ->
            case Fairway.Event.from_store_event(stored_event) do
              {:ok, _pos, event} ->
                case fun.(event, acc) do
                  {:halt, new_acc} -> {:halt, new_acc}
                  new_acc -> {:cont, new_acc}
                end

              {:error, {:unknown_type, _}} ->
                {:cont, acc}

              {:error, _} ->
                {:cont, acc}
            end
          end)

        {:ok, result}

      {:error, _} = err ->
        err
    end
  end
end
