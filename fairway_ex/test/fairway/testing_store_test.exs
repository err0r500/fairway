defmodule Fairway.TestingStoreTest do
  use ExUnit.Case, async: true

  alias Fairway.Testing.Store
  alias Fairway.Test.Events.{ListCreated, ItemAdded}

  setup do
    Fairway.Registry.register_all([ListCreated, ItemAdded])
    {:ok, store} = Store.start_link()
    %{store: store}
  end

  test "read_all_events returns empty list for a fresh store", %{store: store} do
    assert {:ok, []} = Store.read_all_events(store)
  end

  test "append_events and read_all_events roundtrip", %{store: store} do
    event = Fairway.Event.new(%ListCreated{list_id: "l1", name: "My List"})
    {:ok, wire} = Fairway.Event.to_store_event(event)

    assert :ok = Store.append_events(store, [wire], [])
    assert {:ok, [stored]} = Store.read_all_events(store)

    {_pos, type, tags, _data} = stored
    assert type == "ListCreated"
    assert "list:l1" in tags
  end

  test "positions are monotonically increasing", %{store: store} do
    e1 = Fairway.Event.new(%ListCreated{list_id: "a", name: "A"})
    e2 = Fairway.Event.new(%ListCreated{list_id: "b", name: "B"})
    {:ok, w1} = Fairway.Event.to_store_event(e1)
    {:ok, w2} = Fairway.Event.to_store_event(e2)

    :ok = Store.append_events(store, [w1, w2], [])
    {:ok, [{pos1, _, _, _}, {pos2, _, _, _}]} = Store.read_all_events(store)
    assert pos1 < pos2
  end

  test "read_events filters by type", %{store: store} do
    {:ok, w1} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "l1", name: "L"}))
    {:ok, w2} = Fairway.Event.to_store_event(Fairway.Event.new(%ItemAdded{list_id: "l1", item_id: "i1", name: "I"}))
    :ok = Store.append_events(store, [w1, w2], [])

    {:ok, results} = Store.read_events(store, [%{types: ["ListCreated"], tags: []}], %{})
    assert length(results) == 1
    {_, type, _, _} = hd(results)
    assert type == "ListCreated"
  end

  test "read_events filters by tag", %{store: store} do
    {:ok, w1} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "l1", name: "L1"}))
    {:ok, w2} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "l2", name: "L2"}))
    :ok = Store.append_events(store, [w1, w2], [])

    {:ok, results} = Store.read_events(store, [%{types: [], tags: ["list:l1"]}], %{})
    assert length(results) == 1
    {_, _, tags, _} = hd(results)
    assert "list:l1" in tags
  end

  test "read_events with after_position excludes earlier events", %{store: store} do
    {:ok, w1} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "a", name: "A"}))
    {:ok, w2} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "b", name: "B"}))
    :ok = Store.append_events(store, [w1], [])
    {:ok, [{pos1, _, _, _}]} = Store.read_all_events(store)
    :ok = Store.append_events(store, [w2], [])

    {:ok, results} = Store.read_events(store, [%{types: ["ListCreated"], tags: []}], %{after_position: pos1})
    assert length(results) == 1
    {_, _, tags, _} = hd(results)
    assert "list:b" in tags
  end

  test "DCB condition_failed when conflicting event exists", %{store: store} do
    {:ok, w1} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "l1", name: "L"}))
    :ok = Store.append_events(store, [w1], [])

    # Try to append with condition: "no ListCreated for list:l1 after nil"
    {:ok, w2} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "l1", name: "L2"}))
    condition = %{
      query_items: [%{types: ["ListCreated"], tags: ["list:l1"]}],
      after_position: nil
    }
    assert {:error, :condition_failed} = Store.append_events(store, [w2], [condition])
  end

  test "DCB condition passes when after_position is after the conflicting event", %{store: store} do
    {:ok, w1} = Fairway.Event.to_store_event(Fairway.Event.new(%ListCreated{list_id: "l1", name: "L"}))
    :ok = Store.append_events(store, [w1], [])
    {:ok, [{pos1, _, _, _}]} = Store.read_all_events(store)

    # Condition: no ListCreated for list:l1 AFTER pos1 (the existing event is at pos1, not after)
    {:ok, w2} = Fairway.Event.to_store_event(Fairway.Event.new(%ItemAdded{list_id: "l1", item_id: "i1", name: "Item"}))
    condition = %{
      query_items: [%{types: ["ListCreated"], tags: ["list:l1"]}],
      after_position: pos1
    }
    assert :ok = Store.append_events(store, [w2], [condition])
  end
end
