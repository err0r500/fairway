defmodule Realworldapp.Event.UserWelcomeEmailSent do
  @moduledoc "A welcome email was successfully sent to a user."

  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:user_id]

  @impl true
  def tags(%{user_id: id}), do: ["user_id:#{id}"]

  @impl true
  def type_name, do: "Realworldapp.UserWelcomeEmailSent"
end
