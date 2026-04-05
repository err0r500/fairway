defmodule Fairway.Automation.Config do
  @moduledoc """
  Configuration for a single `Fairway.Automation`.

  ## Required fields

  - `:store` — store handle (e.g. pid returned by `Fairway.Testing.Store.start_link/0`)
  - `:store_module` — module implementing `Fairway.Store` (e.g. `Fairway.Testing.Store`)
  - `:queue_id` — stable unique string identifying this automation's queue
  - `:event_type` — event module (must implement `Fairway.EventData`) whose events trigger this automation
  - `:handler` — `fn(%Fairway.Event{}) -> {command_module, args} | :skip`
  - `:deps` — dependencies injected into the command (passed as third arg to `command.run/3`)

  ## Optional fields (with defaults)

  - `:num_workers` — concurrent jobs (default: `2`)
  - `:poll_interval_ms` — how often the watcher polls for new events (default: `100`)
  - `:max_attempts` — attempts before a job goes to DLQ (default: `3`)
  - `:retry_base_ms` — base retry delay in ms; scaled by `5^(attempt-1)` (default: `60_000`)
  """

  @enforce_keys [:store, :store_module, :queue_id, :event_type, :handler, :deps]
  defstruct [
    :store,
    :store_module,
    :queue_id,
    :event_type,
    :handler,
    :deps,
    num_workers: 2,
    poll_interval_ms: 100,
    max_attempts: 3,
    retry_base_ms: 60_000
  ]

  @type t :: %__MODULE__{
          store: term(),
          store_module: module(),
          queue_id: String.t(),
          event_type: module(),
          handler: (Fairway.Event.t() -> {module(), map()} | :skip),
          deps: term(),
          num_workers: pos_integer(),
          poll_interval_ms: pos_integer(),
          max_attempts: pos_integer(),
          retry_base_ms: pos_integer()
        }
end

defmodule Fairway.Automation.DLQEntry do
  @moduledoc "A job that exhausted all retry attempts."

  defstruct [:position, :attempts, :error, :failed_at]

  @type t :: %__MODULE__{
          position: binary(),
          attempts: non_neg_integer(),
          error: String.t(),
          failed_at: DateTime.t()
        }
end

defmodule Fairway.Automation do
  @moduledoc """
  Background processor that watches for events of a given type and runs a
  command for each one.

  ## How it works

  1. **Watcher** — on each `poll_interval_ms` tick the automation reads events of
     `event_type` from the store that appeared after the last-seen cursor position.
     Each new event is pushed onto the internal job queue.

  2. **Dispatch** — up to `num_workers` jobs are processed concurrently. Each job
     is handled in a separate spawned process.

  3. **Command execution** — for each event the `handler` function is called:
     - Returns `{command_module, args}` → the command is run via
       `Fairway.CommandRunner.run_with_effect/3` with the configured `deps`.
     - Returns `:skip` → the job is acknowledged and discarded.

  4. **Retry** — on failure, the job is rescheduled with exponential backoff
     (`retry_base_ms × 5^(attempt-1)`): 1 min → 5 min → 25 min by default.
     After `max_attempts` failures the job is moved to the dead-letter queue.

  ## Dead-letter queue (DLQ)

      {:ok, entries} = Fairway.Automation.list_dlq(pid)
      :ok = Fairway.Automation.replay_dlq(pid, entry.position)
      :ok = Fairway.Automation.purge_dlq(pid, ~U[2025-01-01 00:00:00Z])

  ## Starting

      {:ok, pid} = Fairway.Automation.start_link(%Fairway.Automation.Config{
        store:            store,
        store_module:     Fairway.Testing.Store,
        queue_id:         "welcome-email",
        event_type:       MyApp.Events.UserRegistered,
        handler:          fn %{data: %UserRegistered{id: id, email: email}} ->
                            {MyApp.Commands.SendWelcomeEmail, %{user_id: id, email: email}}
                          end,
        deps:             %{mailer: MyApp.Mailer},
        num_workers:      4,
        poll_interval_ms: 200,
      })
  """

  use GenServer

  alias Fairway.{CommandRunner, Event, Registry}

  # ── Internal structs ────────────────────────────────────────────────────────

  defmodule Job do
    @moduledoc false
    defstruct [:position, :event, attempts: 0]
  end

  # ── State ───────────────────────────────────────────────────────────────────

  defstruct [
    :config,
    :runner,
    last_position: nil,
    pending: :queue.new(),
    inflight: %{},
    worker_count: 0,
    dlq: []
  ]

  # ── Public API ───────────────────────────────────────────────────────────────

  @doc "Start an automation process. `config` is a `Fairway.Automation.Config` struct or keyword list."
  def start_link(%Fairway.Automation.Config{} = config) do
    GenServer.start_link(__MODULE__, config)
  end

  def start_link(opts) when is_list(opts) do
    GenServer.start_link(__MODULE__, struct!(Fairway.Automation.Config, opts))
  end

  @doc "Stop the automation gracefully."
  def stop(pid), do: GenServer.stop(pid, :normal)

  @doc "Return all dead-letter queue entries (oldest first)."
  def list_dlq(pid), do: GenServer.call(pid, :list_dlq)

  @doc "Move a DLQ entry back onto the pending queue for reprocessing."
  def replay_dlq(pid, position), do: GenServer.call(pid, {:replay_dlq, position})

  @doc "Remove all DLQ entries with `failed_at` before `before_dt`."
  def purge_dlq(pid, %DateTime{} = before_dt),
    do: GenServer.call(pid, {:purge_dlq, before_dt})

  # ── GenServer callbacks ───────────────────────────────────────────────────

  @impl GenServer
  def init(%Fairway.Automation.Config{} = config) do
    # Register event type for deserialization
    Registry.register(config.event_type)

    runner =
      CommandRunner.new_with_deps(
        config.store,
        config.store_module,
        config.deps,
        # Automation has its own retry; command runner runs once
        max_attempts: 1
      )

    # Kick off the first poll immediately
    schedule_poll(0)

    {:ok, %__MODULE__{config: config, runner: runner}}
  end

  # ── Poll timer ───────────────────────────────────────────────────────────────

  @impl GenServer
  def handle_info(:poll, state) do
    state =
      state
      |> poll()
      |> dispatch()

    schedule_poll(state.config.poll_interval_ms)
    {:noreply, state}
  end

  # ── Worker completion (success or handled error) ──────────────────────────

  @impl GenServer
  def handle_info({:job_done, pid, result}, state) do
    case Map.pop(state.inflight, pid) do
      {nil, _} ->
        {:noreply, state}

      {job, inflight} ->
        state = %{state | inflight: inflight, worker_count: state.worker_count - 1}

        state =
          case result do
            :ok -> state
            {:error, reason} -> handle_failure(state, job, reason)
          end

        {:noreply, dispatch(state)}
    end
  end

  # ── Worker crash (unhandled exception / exit) ─────────────────────────────

  @impl GenServer
  def handle_info({:DOWN, _ref, :process, pid, reason}, state) when reason != :normal do
    case Map.pop(state.inflight, pid) do
      {nil, _} ->
        {:noreply, state}

      {job, inflight} ->
        state = %{state | inflight: inflight, worker_count: state.worker_count - 1}
        state = handle_failure(state, job, {:crash, reason})
        {:noreply, dispatch(state)}
    end
  end

  # Normal :DOWN — job_done was already handled
  def handle_info({:DOWN, _ref, :process, _pid, :normal}, state), do: {:noreply, state}

  # ── Retry timer ───────────────────────────────────────────────────────────

  @impl GenServer
  def handle_info({:retry_job, job}, state) do
    state = %{state | pending: :queue.in(job, state.pending)}
    {:noreply, dispatch(state)}
  end

  # ── DLQ queries ──────────────────────────────────────────────────────────

  @impl GenServer
  def handle_call(:list_dlq, _from, state) do
    {:reply, Enum.reverse(state.dlq), state}
  end

  @impl GenServer
  def handle_call({:replay_dlq, position}, _from, state) do
    case Enum.find(state.dlq, fn e -> e.position == position end) do
      nil ->
        {:reply, {:error, :not_found}, state}

      entry ->
        new_dlq = Enum.reject(state.dlq, fn e -> e.position == position end)
        # Re-enqueue with attempts reset and event to be re-fetched
        job = %Job{position: entry.position, event: nil, attempts: 0}
        new_state = %{state | dlq: new_dlq, pending: :queue.in(job, state.pending)}
        {:reply, :ok, dispatch(new_state)}
    end
  end

  @impl GenServer
  def handle_call({:purge_dlq, before_dt}, _from, state) do
    new_dlq = Enum.reject(state.dlq, fn e -> DateTime.before?(e.failed_at, before_dt) end)
    {:reply, :ok, %{state | dlq: new_dlq}}
  end

  # ── Private: polling ─────────────────────────────────────────────────────

  defp poll(%{config: config, last_position: last_pos} = state) do
    type_name = config.event_type.type_name()
    query_items = [%{types: [type_name], tags: []}]
    opts = %{after_position: last_pos, limit: 100, reverse: false}

    case config.store_module.read_events(config.store, query_items, opts) do
      {:ok, []} ->
        state

      {:ok, stored_events} ->
        {new_pending, new_last_pos} =
          Enum.reduce(stored_events, {state.pending, last_pos}, fn stored, {q, _} ->
            {pos, _, _, _} = stored

            new_q =
              case Event.from_store_event(stored) do
                {:ok, ^pos, event} -> :queue.in(%Job{position: pos, event: event}, q)
                # Unknown type or decode error: skip job, still advance cursor
                {:error, _} -> q
              end

            {new_q, pos}
          end)

        %{state | pending: new_pending, last_position: new_last_pos}

      {:error, _} ->
        state
    end
  end

  # ── Private: dispatch ─────────────────────────────────────────────────────

  # All worker slots full — wait for completions
  defp dispatch(%{worker_count: n, config: %{num_workers: max}} = state) when n >= max,
    do: state

  defp dispatch(state) do
    case :queue.out(state.pending) do
      {:empty, _} ->
        state

      {{:value, job}, rest} ->
        pid = spawn_worker(job, state)
        state = %{state | pending: rest, inflight: Map.put(state.inflight, pid, job),
                          worker_count: state.worker_count + 1}
        # Recurse: try to fill remaining slots
        dispatch(state)
    end
  end

  # ── Private: worker process ──────────────────────────────────────────────

  defp spawn_worker(job, %{config: config, runner: runner}) do
    parent = self()

    {pid, _ref} =
      spawn_monitor(fn ->
        result =
          try do
            process_job(job, config, runner)
          rescue
            e -> {:error, {:exception, Exception.message(e)}}
          catch
            :exit, reason -> {:error, {:exit, reason}}
          end

        send(parent, {:job_done, self(), result})
      end)

    pid
  end

  # Normal path: event already in job
  defp process_job(%Job{event: event} = job, config, runner) when not is_nil(event) do
    run_handler(job, event, config, runner)
  end

  # DLQ replay path: event must be re-fetched from the store
  defp process_job(%Job{position: pos, event: nil} = job, config, runner) do
    type_name = config.event_type.type_name()
    opts = %{after_position: nil, limit: nil, reverse: false}

    case config.store_module.read_events(config.store, [%{types: [type_name], tags: []}], opts) do
      {:ok, stored_events} ->
        case Enum.find(stored_events, fn {p, _, _, _} -> p == pos end) do
          nil ->
            {:error, :event_not_found}

          stored ->
            case Event.from_store_event(stored) do
              {:ok, _, event} -> run_handler(job, event, config, runner)
              {:error, reason} -> {:error, reason}
            end
        end

      {:error, reason} ->
        {:error, reason}
    end
  end

  defp run_handler(_job, event, config, runner) do
    case config.handler.(event) do
      :skip -> :ok
      {command_module, args} -> CommandRunner.run_with_effect(runner, command_module, args)
    end
  end

  # ── Private: failure handling ────────────────────────────────────────────

  defp handle_failure(state, job, reason) do
    next_attempts = job.attempts + 1

    if next_attempts >= state.config.max_attempts do
      entry = %Fairway.Automation.DLQEntry{
        position: job.position,
        attempts: next_attempts,
        error: inspect(reason),
        failed_at: DateTime.utc_now()
      }

      %{state | dlq: [entry | state.dlq]}
    else
      delay = backoff_ms(next_attempts, state.config.retry_base_ms)
      Process.send_after(self(), {:retry_job, %{job | attempts: next_attempts}}, delay)
      state
    end
  end

  # base_ms × 5^(attempt-1): attempt 1 → 1×, 2 → 5×, 3 → 25×
  defp backoff_ms(attempt, base_ms) do
    round(base_ms * :math.pow(5, attempt - 1))
  end

  defp schedule_poll(ms) do
    Process.send_after(self(), :poll, ms)
  end
end
