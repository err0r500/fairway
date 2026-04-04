defmodule Fairway.Store do
  @moduledoc """
  Behaviour for the Fairway event store.

  Two implementations are provided:
    - `Fairway.Fdb.Store`     — FoundationDB via Rustler NIF (production)
    - `Fairway.Testing.Store` — ETS-backed in-memory store (tests, no FDB required)

  ## Types

    - `position()` — 12-byte binary (FDB versionstamp: 10-byte tx version + 2-byte user version)
    - `stored_event()` — `{position, type_string, tags_list, data_bytes}`
    - `query_item()` — `%{types: [String.t()], tags: [String.t()]}`
    - `append_condition()` — `%{query_items: [query_item()], after_position: position() | nil}`
  """

  @type position :: binary()
  @type stored_event :: {position(), String.t(), [String.t()], binary()}
  @type query_item :: %{types: [String.t()], tags: [String.t()]}
  @type append_condition :: %{query_items: [query_item()], after_position: position() | nil}

  @doc """
  Read events matching the given query items within a single snapshot transaction.
  Returns a list of `stored_event` tuples in versionstamp order (or reverse if specified).
  """
  @callback read_events(
              store :: term(),
              query_items :: [query_item()],
              opts :: map()
            ) :: {:ok, [stored_event()]} | {:error, term()}

  @doc """
  Append events with optional DCB conditions inside a single atomic transaction.
  Returns `:ok` on success, `{:error, :condition_failed}` if any condition was violated,
  or `{:error, reason}` on other errors.
  """
  @callback append_events(
              store :: term(),
              events :: [%{type: String.t(), tags: [String.t()], data: binary()}],
              conditions :: [append_condition()]
            ) :: :ok | {:error, :condition_failed} | {:error, term()}

  @doc """
  Read all events from the store in versionstamp order.
  """
  @callback read_all_events(store :: term()) :: {:ok, [stored_event()]} | {:error, term()}
end
