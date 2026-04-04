defmodule Todolist.View.ShowListTest do
  use ExUnit.Case, async: true

  alias Fairway.Testing.Store
  alias Fairway.CommandRunner
  alias Todolist.Change.{CreateList, AddItem}
  alias Todolist.View.ShowList
  alias Todolist.Event.{ListCreated, ItemCreated}

  setup do
    Fairway.Registry.register_all([ListCreated, ItemCreated])
    {:ok, store} = Store.start_link()
    runner = CommandRunner.new(store, Store)
    %{store: store, runner: runner}
  end

  test "returns :not_found for an unknown list", %{store: store} do
    assert {:error, :not_found} = ShowList.get(store, Store, "unknown")
  end

  test "returns list metadata with 0 items", %{store: store, runner: runner} do
    :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Shopping"})

    assert {:ok, %ShowList{id: "l1", name: "Shopping", items_count: 0}} =
             ShowList.get(store, Store, "l1")
  end

  test "returns correct item count after adding items", %{store: store, runner: runner} do
    :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Shopping"})
    :ok = CommandRunner.run_pure(runner, AddItem, %{list_id: "l1", item_id: "i1", text: "Milk"})
    :ok = CommandRunner.run_pure(runner, AddItem, %{list_id: "l1", item_id: "i2", text: "Eggs"})

    assert {:ok, %ShowList{id: "l1", name: "Shopping", items_count: 2}} =
             ShowList.get(store, Store, "l1")
  end

  test "view is scoped — items from other lists are not counted", %{store: store, runner: runner} do
    :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Shopping"})
    :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l2", name: "Work"})
    :ok = CommandRunner.run_pure(runner, AddItem, %{list_id: "l1", item_id: "i1", text: "Milk"})
    :ok = CommandRunner.run_pure(runner, AddItem, %{list_id: "l2", item_id: "i1", text: "Laptop"})

    assert {:ok, %ShowList{items_count: 1}} = ShowList.get(store, Store, "l1")
    assert {:ok, %ShowList{items_count: 1}} = ShowList.get(store, Store, "l2")
  end
end
