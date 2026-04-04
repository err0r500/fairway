defmodule Todolist.Event.ListCreated do
  @moduledoc "A todo list was created."

  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:list_id, :name]

  @impl true
  def tags(%{list_id: id}), do: ["list_id:#{id}"]

  @impl true
  def type_name, do: "Todolist.ListCreated"
end
