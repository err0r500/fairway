defmodule Fairway.CommandRunner do
  @moduledoc """
  Executes commands with automatic retry on DCB condition failures.

  ## Pure commands (no side effects)

  Default: up to 4 attempts (1 + 3 retries), exponential backoff starting at 10ms, capped at 500ms.

      runner = Fairway.CommandRunner.new(store, MyApp.Fdb.Store)
      Fairway.CommandRunner.run_pure(runner, MyApp.Commands.CreateList, %{list_id: "x", name: "Y"})

  ## Commands with side effects

  Default: 1 attempt (no retry — side effects may not be idempotent).

      runner = Fairway.CommandRunner.new_with_deps(store, MyApp.Fdb.Store, %{email: MyApp.Email})
      Fairway.CommandRunner.run_with_effect(runner, MyApp.Commands.SendWelcomeEmail, %{user_id: "u1"})

  ## Command behaviour

  Pure command modules must implement:

      @callback run(Fairway.ReadAppender.t(), args :: map()) ::
        :ok | {:ok, Fairway.ReadAppender.t()} | {:error, term()}

  The return value may be `:ok`, `{:ok, ra}` (ra is ignored), or `{:error, reason}`.
  `{:error, :condition_failed}` triggers a retry (up to max_attempts).

  ## Custom retry per command

  Implement `retry_opts/0` on the command module to override runner defaults:

      def retry_opts, do: [max_attempts: 1]  # disable retry for this command
  """

  defstruct store: nil, store_module: nil, deps: nil, max_attempts: 4, base_delay_ms: 10

  @type t :: %__MODULE__{
          store: term(),
          store_module: module(),
          deps: term(),
          max_attempts: pos_integer(),
          base_delay_ms: pos_integer()
        }

  def new(store, store_module, opts \\ []) do
    %__MODULE__{
      store: store,
      store_module: store_module,
      max_attempts: Keyword.get(opts, :max_attempts, 4),
      base_delay_ms: Keyword.get(opts, :base_delay_ms, 10)
    }
  end

  def new_with_deps(store, store_module, deps, opts \\ []) do
    %{new(store, store_module, opts) | deps: deps, max_attempts: Keyword.get(opts, :max_attempts, 1)}
  end

  @doc "Run a pure command (no side effects). Retries on {:error, :condition_failed}."
  def run_pure(%__MODULE__{} = runner, command_module, args) do
    max = command_max_attempts(command_module, runner.max_attempts)
    do_retry(runner, command_module, args, max, 0)
  end

  @doc "Run a command with side effects. No retry by default."
  def run_with_effect(%__MODULE__{} = runner, command_module, args) do
    max = command_max_attempts(command_module, runner.max_attempts)
    do_retry_with_effect(runner, command_module, args, max, 0)
  end

  # ── Private retry loop ────────────────────────────────────────────────────

  defp do_retry(_runner, _mod, _args, 0, _attempt) do
    {:error, :max_retries_exceeded}
  end

  defp do_retry(runner, mod, args, remaining, attempt) do
    ra = Fairway.ReadAppender.new(runner.store, runner.store_module)

    case mod.run(ra, args) do
      :ok ->
        :ok

      {:ok, _ra} ->
        :ok

      {:error, :condition_failed} when remaining > 1 ->
        backoff(attempt, runner.base_delay_ms)
        do_retry(runner, mod, args, remaining - 1, attempt + 1)

      other ->
        other
    end
  end

  defp do_retry_with_effect(_runner, _mod, _args, 0, _attempt) do
    {:error, :max_retries_exceeded}
  end

  defp do_retry_with_effect(runner, mod, args, remaining, attempt) do
    ra = Fairway.ReadAppender.new(runner.store, runner.store_module)

    case mod.run(ra, args, runner.deps) do
      :ok ->
        :ok

      {:ok, _ra} ->
        :ok

      {:error, :condition_failed} when remaining > 1 ->
        backoff(attempt, runner.base_delay_ms)
        do_retry_with_effect(runner, mod, args, remaining - 1, attempt + 1)

      other ->
        other
    end
  end

  defp backoff(attempt, base_ms) do
    delay = min(base_ms * :math.pow(2, attempt) |> round(), 500)
    :timer.sleep(delay)
  end

  defp command_max_attempts(mod, default) do
    if function_exported?(mod, :retry_opts, 0) do
      Keyword.get(mod.retry_opts(), :max_attempts, default)
    else
      default
    end
  end
end
