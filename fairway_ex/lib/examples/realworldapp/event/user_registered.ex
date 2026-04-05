defmodule Realworldapp.Event.UserRegistered do
  @moduledoc "A new user completed registration."

  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:id, :username, :email, :hashed_password]

  @impl true
  def tags(%{id: id, username: username, email: email}) do
    ["user_id:#{id}", "username:#{username}", "email:#{email}"]
  end

  @impl true
  def type_name, do: "Realworldapp.UserRegistered"
end
