defmodule Todolist.Event.ItemCreated do
  @moduledoc "An item was added to a todo list."

  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:item_id, :list_id, :text]

  @impl true
  def tags(%{item_id: iid, list_id: lid}), do: ["list_id:#{lid}", "item_id:#{iid}"]

  @impl true
  def type_name, do: "Todolist.ItemCreated"
end
