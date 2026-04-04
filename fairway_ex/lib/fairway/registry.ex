defmodule Fairway.Registry do
  @moduledoc """
  ETS-backed registry mapping event type name strings → module atoms.

  Event modules register themselves at application start. The registry is used
  during deserialization to find the correct struct module for a given type name.

  Registration:

      Fairway.Registry.register(MyApp.Events.ListCreated)
      # or in bulk:
      Fairway.Registry.register_all([MyApp.Events.ListCreated, MyApp.Events.ItemAdded])

  Lookup:

      {:ok, MyApp.Events.ListCreated} = Fairway.Registry.lookup("ListCreated")
      {:error, :not_registered} = Fairway.Registry.lookup("Unknown")
  """

  use Agent

  @table :fairway_event_registry

  def start_link(_opts) do
    Agent.start_link(fn ->
      :ets.new(@table, [:named_table, :public, read_concurrency: true])
      :ok
    end, name: __MODULE__)
  end

  @doc "Register an event module. Uses `module.type_name()` as the key."
  def register(module) when is_atom(module) do
    type_name =
      if function_exported?(module, :type_name, 0) do
        module.type_name()
      else
        inspect(module)
      end
    :ets.insert(@table, {type_name, module})
    :ok
  end

  @doc "Register multiple event modules."
  def register_all(modules) when is_list(modules) do
    Enum.each(modules, &register/1)
  end

  @doc "Look up a module by its type name string."
  def lookup(type_name) do
    case :ets.lookup(@table, type_name) do
      [{^type_name, module}] -> {:ok, module}
      [] -> {:error, :not_registered}
    end
  end

  @doc "Auto-discover and register all loaded modules that implement Fairway.EventData."
  def discover_and_register do
    :code.all_loaded()
    |> Enum.map(fn {mod, _} -> mod end)
    |> Enum.filter(fn mod ->
      function_exported?(mod, :type_name, 0) and function_exported?(mod, :tags, 1)
    end)
    |> Enum.each(&register/1)
  end
end
