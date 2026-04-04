defmodule Fairway.CommandRunnerTest do
  use ExUnit.Case, async: true

  alias Fairway.Testing.Store
  alias Fairway.Test.Events.ListCreated
  alias Fairway.Test.Commands.{CreateList, AddItem}

  setup do
    Fairway.Registry.register_all([ListCreated, Fairway.Test.Events.ItemAdded])
    {:ok, store} = Store.start_link()
    runner = Fairway.CommandRunner.new(store, Store)
    %{store: store, runner: runner}
  end

  test "run_pure succeeds for a new list", %{runner: runner} do
    assert :ok = Fairway.CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "My List"})
  end

  test "run_pure fails with :already_exists if list exists", %{runner: runner} do
    :ok = Fairway.CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "My List"})
    assert {:error, :already_exists} =
      Fairway.CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Duplicate"})
  end

  test "run_pure retries on condition_failed and eventually succeeds with concurrent writes", %{store: store} do
    # Simulate: first attempt sees condition_failed (because a concurrent write happened),
    # second attempt reads new state and succeeds with a different outcome.
    # We test this by having two runners racing for the same list.
    runner1 = Fairway.CommandRunner.new(store, Store, max_attempts: 4)
    runner2 = Fairway.CommandRunner.new(store, Store, max_attempts: 4)

    # Both try to create "l1" concurrently — exactly one should succeed, one fails
    task1 = Task.async(fn -> Fairway.CommandRunner.run_pure(runner1, CreateList, %{list_id: "l1", name: "A"}) end)
    task2 = Task.async(fn -> Fairway.CommandRunner.run_pure(runner2, CreateList, %{list_id: "l1", name: "B"}) end)

    results = [Task.await(task1), Task.await(task2)]
    assert :ok in results
    assert {:error, :already_exists} in results
  end

  test "AddItem fails when list does not exist", %{runner: runner} do
    assert {:error, :list_not_found} =
      Fairway.CommandRunner.run_pure(runner, AddItem, %{list_id: "missing", item_id: "i1", name: "X"})
  end

  test "AddItem succeeds after CreateList", %{runner: runner} do
    :ok = Fairway.CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "My List"})
    assert :ok = Fairway.CommandRunner.run_pure(runner, AddItem, %{list_id: "l1", item_id: "i1", name: "Item 1"})
  end

  test "view reflects appended events", %{store: store, runner: runner} do
    :ok = Fairway.CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Shopping"})
    :ok = Fairway.CommandRunner.run_pure(runner, AddItem, %{list_id: "l1", item_id: "i1", name: "Milk"})
    :ok = Fairway.CommandRunner.run_pure(runner, AddItem, %{list_id: "l1", item_id: "i2", name: "Eggs"})

    query = Fairway.Query.new([
      Fairway.QueryItem.new(
        types: [ListCreated, Fairway.Test.Events.ItemAdded],
        tags: ["list:l1"]
      )
    ])

    {:ok, result} = Fairway.View.read_events(store, Store, query, %{name: nil, items: []}, fn
      %{data: %ListCreated{name: n}}, acc -> %{acc | name: n}
      %{data: %Fairway.Test.Events.ItemAdded{name: n}}, acc -> Map.update!(acc, :items, &[n | &1])
      _, acc -> acc
    end)

    assert result.name == "Shopping"
    assert length(result.items) == 2
    assert "Milk" in result.items
    assert "Eggs" in result.items
  end
end
