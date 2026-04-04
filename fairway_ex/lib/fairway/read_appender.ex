defmodule Fairway.ReadAppender do
  @moduledoc """
  Immutable struct that threads through command functions, accumulating read
  records and constructing DCB append conditions at write time.

  This mirrors Go's `commandReadAppender`:
  - Each call to `fold_events/4` records `{query, highest_versionstamp}`.
  - `append_events/2` builds one `AppendCondition` per prior read record
    and calls `Store.append_events/3` atomically.

  Commands receive and return the `ReadAppender` explicitly (like Ecto.Multi):

      def run(ra, %MyCmd{list_id: id, name: name}) do
        {ra, exists} =
          Fairway.ReadAppender.fold_events(ra, query, false, fn
            %{data: %ListCreated{}}, _ -> {false, true}
            _, acc -> {true, acc}
          end)

        if exists do
          {:error, :already_exists}
        else
          Fairway.ReadAppender.append_events(ra, [Fairway.Event.new(%ListCreated{...})])
        end
      end

  The store and its module are passed in at construction time.
  """

  defstruct store: nil, store_module: nil, reads: []

  @type read_record :: {Fairway.Query.t(), binary() | nil}

  @type t :: %__MODULE__{
          store: term(),
          store_module: module(),
          reads: [read_record()]
        }

  @doc "Create a new ReadAppender backed by the given store."
  def new(store, store_module) do
    %__MODULE__{store: store, store_module: store_module}
  end

  @doc """
  Read events matching `query`, fold them into `acc` using `fun`, and record the
  highest versionstamp seen (for the subsequent append condition).

  `fun` is called as `fun.(fairway_event, acc) -> {continue :: boolean(), new_acc}`.
  Return `{false, acc}` to stop iteration early.

  Returns `{updated_ra, final_acc}`.
  """
  def fold_events(%__MODULE__{} = ra, %Fairway.Query{} = query, initial_acc, fun)
      when is_function(fun, 2) do
    wire_items = Fairway.Query.to_wire_items(query)
    read_opts = Fairway.Query.to_read_opts(query)

    case ra.store_module.read_events(ra.store, wire_items, read_opts) do
      {:ok, stored_events} ->
        {final_acc, highest_pos} =
          Enum.reduce_while(stored_events, {initial_acc, nil}, fn stored_event, {acc, _highest} ->
            {pos, _type, _tags, _data} = stored_event

            case Fairway.Event.from_store_event(stored_event) do
              {:ok, ^pos, event} ->
                {continue, new_acc} = fun.(event, acc)
                # Track highest position (versionstamps are lexicographically ordered)
                new_highest = if _highest == nil or pos > _highest, do: pos, else: _highest
                if continue do
                  {:cont, {new_acc, new_highest}}
                else
                  {:halt, {new_acc, new_highest}}
                end

              {:error, {:unknown_type, _}} ->
                # Skip unknown event types (forward compatibility)
                {:cont, {acc, _highest}}

              {:error, _reason} ->
                {:cont, {acc, _highest}}
            end
          end)

        # Record this read
        new_ra = %{ra | reads: ra.reads ++ [{query, highest_pos}]}
        {new_ra, final_acc}

      {:error, reason} ->
        # Propagate store errors as a special acc value; commands should check
        # or use fold_events! variant. Here we return the error as the acc.
        {ra, {:error, reason}}
    end
  end

  @doc """
  Append events with DCB conditions derived from all prior `fold_events` calls.
  Each read record produces one `AppendCondition`:
    - `query_items` from the query
    - `after_position` = highest versionstamp seen (nil if no events were read)

  Returns `:ok | {:error, :condition_failed} | {:error, reason}`.
  """
  def append_events(%__MODULE__{} = ra, events) when is_list(events) do
    # Serialize events to store wire format
    case serialize_events(events) do
      {:ok, wire_events} ->
        conditions = build_conditions(ra.reads)
        ra.store_module.append_events(ra.store, wire_events, conditions)

      {:error, _} = err ->
        err
    end
  end

  # ── Private helpers ──────────────────────────────────────────────────────────

  defp serialize_events(events) do
    Enum.reduce_while(events, {:ok, []}, fn event, {:ok, acc} ->
      case Fairway.Event.to_store_event(event) do
        {:ok, wire} -> {:cont, {:ok, acc ++ [wire]}}
        {:error, _} = err -> {:halt, err}
      end
    end)
  end

  defp build_conditions(reads) do
    Enum.map(reads, fn {query, highest_pos} ->
      %{
        query_items: Fairway.Query.to_wire_items(query),
        after_position: highest_pos
      }
    end)
  end
end
