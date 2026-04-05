defmodule Fairway.AutomationRegistry do
  @moduledoc """
  Registry of automation factory functions.

  Each factory is registered with `register/2` and receives `(store, store_module, deps)`
  at start time, returning a `Fairway.Automation.Config` struct.

  `start_all/4` creates and starts all registered automations, validates that no two
  share the same `queue_id`, and returns a zero-arity stop function that gracefully
  shuts them all down.

  ## Usage

      registry = Fairway.AutomationRegistry.new()

      registry =
        Fairway.AutomationRegistry.register(registry, fn store, store_module, deps ->
          %Fairway.Automation.Config{
            store:        store,
            store_module: store_module,
            queue_id:     "welcome-email",
            event_type:   MyApp.Events.UserRegistered,
            handler:      &MyApp.Automations.WelcomeEmail.to_command/1,
            deps:         deps,
          }
        end)

      {:ok, stop} = Fairway.AutomationRegistry.start_all(registry, store, Fairway.Testing.Store, deps)
      # later:
      stop.()

  ## In module `init/0` style (mirrors Go's `init()` pattern)

      defmodule MyApp.Automations do
        @registry Fairway.AutomationRegistry.new()
                  |> Fairway.AutomationRegistry.register(&WelcomeEmail.build_config/3)
                  |> Fairway.AutomationRegistry.register(&InviteReminder.build_config/3)

        def registry, do: @registry
      end
  """

  defstruct factories: []

  @type factory :: (store :: term(), store_module :: module(), deps :: term() ->
                      Fairway.Automation.Config.t())

  @type t :: %__MODULE__{factories: [factory()]}

  @doc "Create an empty registry."
  def new, do: %__MODULE__{}

  @doc """
  Register an automation factory.

  `factory` is a 3-arity function `fn(store, store_module, deps) -> %Config{}`.
  """
  def register(%__MODULE__{} = registry, factory) when is_function(factory, 3) do
    %{registry | factories: registry.factories ++ [factory]}
  end

  @doc """
  Start all registered automations.

  Validates that no two automations share the same `queue_id`.
  Returns `{:ok, stop_fn}` where `stop_fn.()` gracefully stops every automation,
  or `{:error, reason}` if any factory or start fails.
  """
  def start_all(%__MODULE__{} = registry, store, store_module, deps) do
    registry.factories
    |> Enum.reduce_while({:ok, []}, fn factory, {:ok, acc} ->
      config = factory.(store, store_module, deps)

      case Fairway.Automation.start_link(config) do
        {:ok, pid} -> {:cont, {:ok, [{config.queue_id, pid} | acc]}}
        {:error, reason} -> {:halt, {:error, reason}}
      end
    end)
    |> case do
      {:error, _} = err ->
        err

      {:ok, pairs} ->
        queue_ids = Enum.map(pairs, fn {id, _} -> id end)

        case find_duplicate(queue_ids) do
          nil ->
            pids = Enum.map(pairs, fn {_, pid} -> pid end)
            stop_fn = fn -> Enum.each(pids, &Fairway.Automation.stop/1) end
            {:ok, stop_fn}

          dup_id ->
            Enum.each(pairs, fn {_, pid} -> Fairway.Automation.stop(pid) end)
            {:error, {:duplicate_queue_id, dup_id}}
        end
    end
  end

  defp find_duplicate(list) do
    list
    |> Enum.frequencies()
    |> Enum.find(fn {_k, count} -> count > 1 end)
    |> case do
      nil -> nil
      {id, _} -> id
    end
  end
end
