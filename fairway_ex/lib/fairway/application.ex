defmodule Fairway.Application do
  use Application

  @impl true
  def start(_type, _args) do
    children = [
      Fairway.Registry
    ]

    Supervisor.start_link(children, strategy: :one_for_one, name: Fairway.Supervisor)
  end
end
