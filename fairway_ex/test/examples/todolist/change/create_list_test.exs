defmodule Todolist.Change.CreateListTest do
  use ExUnit.Case, async: true

  alias Fairway.Testing.Store
  alias Fairway.CommandRunner
  alias Todolist.Change.CreateList
  alias Todolist.Event.ListCreated

  setup do
    Fairway.Registry.register_all([ListCreated])
    {:ok, store} = Store.start_link()
    runner = CommandRunner.new(store, Store)
    %{store: store, runner: runner}
  end

  test "creates a new list", %{runner: runner} do
    assert :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Shopping"})
  end

  test "returns :already_exists for a duplicate list", %{runner: runner} do
    :ok = CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Shopping"})

    assert {:error, :already_exists} =
             CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Duplicate"})
  end

  test "concurrent creates — exactly one succeeds", %{store: store} do
    runner = CommandRunner.new(store, Store, max_attempts: 5)

    tasks =
      for _ <- 1..5 do
        Task.async(fn ->
          CommandRunner.run_pure(runner, CreateList, %{list_id: "l1", name: "Race"})
        end)
      end

    results = Task.await_many(tasks)

    assert Enum.count(results, &(&1 == :ok)) == 1
    assert Enum.count(results, &(&1 == {:error, :already_exists})) == 4
  end
end
