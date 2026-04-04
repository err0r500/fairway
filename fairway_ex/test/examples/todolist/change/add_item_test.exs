defmodule Todolist.Change.AddItemTest do
  use ExUnit.Case, async: true

  alias Fairway.Testing.Store
  alias Fairway.CommandRunner
  alias Todolist.Change.{CreateList, AddItem}
  alias Todolist.Event.{ListCreated, ItemCreated}

  setup do
    Fairway.Registry.register_all([ListCreated, ItemCreated])
    {:ok, store} = Store.start_link()
    runner = CommandRunner.new(store, Store)

    # Seed a list for commands that require one to exist
    :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Shopping"})

    %{store: store, runner: runner}
  end

  test "adds an item to an existing list", %{runner: runner} do
    assert :ok =
             CommandRunner.run_pure(runner, AddItem, %{
               list_id: "l1",
               item_id: "i1",
               text: "Milk"
             })
  end

  test "returns :already_exists for a duplicate item", %{runner: runner} do
    :ok =
      CommandRunner.run_pure(runner, AddItem, %{
        list_id: "l1",
        item_id: "i1",
        text: "Milk"
      })

    assert {:error, :already_exists} =
             CommandRunner.run_pure(runner, AddItem, %{
               list_id: "l1",
               item_id: "i1",
               text: "Whole milk"
             })
  end

  test "concurrent duplicate adds — exactly one succeeds", %{store: store} do
    runner = CommandRunner.new(store, Store, max_attempts: 5)

    tasks =
      for _ <- 1..5 do
        Task.async(fn ->
          CommandRunner.run_pure(runner, AddItem, %{
            list_id: "l1",
            item_id: "i1",
            text: "Milk"
          })
        end)
      end

    results = Task.await_many(tasks)

    assert Enum.count(results, &(&1 == :ok)) == 1
    assert Enum.count(results, &(&1 == {:error, :already_exists})) == 4
  end

  test "items in different lists are independent", %{runner: runner} do
    :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l2", name: "Work"})

    assert :ok =
             CommandRunner.run_pure(runner, AddItem, %{
               list_id: "l1",
               item_id: "i1",
               text: "Milk"
             })

    # Same item_id, different list — should succeed
    assert :ok =
             CommandRunner.run_pure(runner, AddItem, %{
               list_id: "l2",
               item_id: "i1",
               text: "Buy laptop"
             })
  end
end
