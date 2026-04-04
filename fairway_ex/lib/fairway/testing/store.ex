defmodule Fairway.Testing.Store do
  @moduledoc """
  In-memory ETS-backed store for tests. Implements `Fairway.Store`.
  No FDB or Rust required — start one per test with `start_link/1`.

  ## Usage in tests

      setup do
        {:ok, store} = Fairway.Testing.Store.start_link()
        %{store: store}
      end

      test "creates a list", %{store: store} do
        runner = Fairway.CommandRunner.new(store, Fairway.Testing.Store)
        assert :ok = Fairway.CommandRunner.run_pure(runner, CreateList, %{list_id: "x", name: "Test"})
      end

  ## DCB condition check

  The ETS store implements the same condition semantics as the FDB store:
  - For each condition, check if any event matching `query_items` exists after `after_position`.
  - If any condition is violated → `{:error, :condition_failed}`.
  - All checks and the write happen atomically using an ETS-backed GenServer (serialized).
  """

  use GenServer
  @behaviour Fairway.Store

  # ── Public API ──────────────────────────────────────────────────────────────

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts)
  end

  @impl Fairway.Store
  def read_events(store, query_items, opts) do
    GenServer.call(store, {:read_events, query_items, opts})
  end

  @impl Fairway.Store
  def append_events(store, events, conditions) do
    GenServer.call(store, {:append_events, events, conditions})
  end

  @impl Fairway.Store
  def read_all_events(store) do
    GenServer.call(store, :read_all_events)
  end

  # ── Convenience: seed events directly (for test setup / Given step) ─────────

  @doc "Append events unconditionally (no DCB check). Used in test setup."
  def given_events(store, events) do
    GenServer.call(store, {:given_events, events})
  end

  # ── GenServer implementation ─────────────────────────────────────────────────

  @impl GenServer
  def init(_opts) do
    # State: list of stored_events in insertion order, each:
    # %{position: binary, type: String, tags: [String], data: binary}
    # Position is a 12-byte counter (big-endian u64 zero-padded to 12 bytes).
    {:ok, %{events: [], counter: 0}}
  end

  @impl GenServer
  def handle_call({:read_events, query_items, opts}, _from, state) do
    events = filter_events(state.events, query_items, opts)
    stored = Enum.map(events, fn e -> {e.position, e.type, e.tags, e.data} end)
    {:reply, {:ok, stored}, state}
  end

  @impl GenServer
  def handle_call(:read_all_events, _from, state) do
    stored = Enum.map(state.events, fn e -> {e.position, e.type, e.tags, e.data} end)
    {:reply, {:ok, stored}, state}
  end

  @impl GenServer
  def handle_call({:append_events, events, conditions}, _from, state) do
    # Check all conditions (serialized — no concurrent access in tests)
    case check_conditions(state.events, conditions) do
      :ok ->
        {new_state, _positions} = do_append(state, events)
        {:reply, :ok, new_state}

      {:error, :condition_failed} = err ->
        {:reply, err, state}
    end
  end

  @impl GenServer
  def handle_call({:given_events, events}, _from, state) do
    {new_state, _} = do_append(state, events)
    {:reply, :ok, new_state}
  end

  # ── Private: filtering ───────────────────────────────────────────────────────

  defp filter_events(events, query_items, opts) do
    after_pos = Map.get(opts, :after_position)
    limit = Map.get(opts, :limit)
    reverse = Map.get(opts, :reverse, false)

    filtered =
      events
      |> Enum.filter(fn e ->
        (after_pos == nil or e.position > after_pos) and
          matches_any_item(e, query_items)
      end)
      |> deduplicate_by_position()
      |> maybe_reverse(reverse)

    case limit do
      nil -> filtered
      n -> Enum.take(filtered, n)
    end
  end

  defp matches_any_item(event, query_items) do
    Enum.any?(query_items, fn item ->
      matches_item(event, item)
    end)
  end

  defp matches_item(event, %{types: types, tags: tags}) do
    type_match = types == [] or event.type in types
    tag_match = Enum.all?(tags, fn tag -> tag in event.tags end)
    type_match and tag_match
  end

  defp deduplicate_by_position(events) do
    events
    |> Enum.uniq_by(fn e -> e.position end)
  end

  defp maybe_reverse(events, true), do: Enum.reverse(events)
  defp maybe_reverse(events, false), do: events

  # ── Private: DCB condition check ─────────────────────────────────────────────

  defp check_conditions(events, conditions) do
    Enum.reduce_while(conditions, :ok, fn condition, :ok ->
      if condition_exists?(events, condition) do
        {:halt, {:error, :condition_failed}}
      else
        {:cont, :ok}
      end
    end)
  end

  defp condition_exists?(events, %{query_items: items, after_position: after_pos}) do
    Enum.any?(events, fn e ->
      (after_pos == nil or e.position > after_pos) and
        matches_any_item(e, items)
    end)
  end

  # ── Private: append ───────────────────────────────────────────────────────────

  defp do_append(state, events) do
    {new_events, new_counter, positions} =
      Enum.reduce(events, {state.events, state.counter, []}, fn wire_event, {evs, counter, pos_acc} ->
        position = counter_to_position(counter)
        stored = %{
          position: position,
          type: wire_event.type,
          tags: Map.get(wire_event, :tags, []),
          data: wire_event.data
        }
        {evs ++ [stored], counter + 1, pos_acc ++ [position]}
      end)

    new_state = %{state | events: new_events, counter: new_counter}
    {new_state, positions}
  end

  # Encode counter as a 12-byte big-endian position (compatible with versionstamp comparison)
  defp counter_to_position(n) do
    <<n::big-unsigned-integer-size(96)>>
  end
end
