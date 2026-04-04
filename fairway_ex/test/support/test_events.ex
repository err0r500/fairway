defmodule Fairway.Test.Events.ListCreated do
  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:list_id, :name]

  @impl true
  def tags(%{list_id: id}), do: ["list:#{id}"]

  @impl true
  def type_name, do: "ListCreated"
end

defmodule Fairway.Test.Events.ItemAdded do
  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:list_id, :item_id, :name]

  @impl true
  def tags(%{list_id: lid, item_id: iid}), do: ["list:#{lid}", "item:#{iid}"]

  @impl true
  def type_name, do: "ItemAdded"
end

defmodule Fairway.Test.Events.UserRegistered do
  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:user_id, :email]

  @impl true
  def tags(%{user_id: id}), do: ["user:#{id}"]

  @impl true
  def type_name, do: "UserRegistered"
end
