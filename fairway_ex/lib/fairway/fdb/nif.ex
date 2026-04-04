defmodule Fairway.Fdb.Nif do
  @moduledoc """
  Rustler NIF wrapper for the Fairway FDB store.

  All functions run on Tokio async threads (DirtyIo schedule) so they never
  block the BEAM scheduler.

  These functions are called by `Fairway.Fdb.Store` and should not be called
  directly from application code.
  """

  use Rustler, otp_app: :fairway, crate: :fairway_fdb

  @doc """
  Open an FDB database. Returns `{:ok, db_ref}` or `{:error, reason}`.
  `cluster_file_path` is the path to the cluster file.
  `namespace` scopes all keys (e.g. `"my_app"`).
  """
  def open_db(_cluster_file_path, _namespace), do: :erlang.nif_error(:nif_not_loaded)

  @doc """
  Read events matching query_items within a single FDB ReadTransaction.

  - `query_json`: JSON-encoded array of `%{types: [String], tags: [String]}` maps
  - `opts_json`: JSON-encoded `%{limit: integer | nil, after_position: hex_string | nil, reverse: boolean}`
  - Returns `{:ok, [{position_hex, type, tags, data_b64}]}` or `{:error, reason}`
  """
  def read_events(_db_ref, _query_json, _opts_json), do: :erlang.nif_error(:nif_not_loaded)

  @doc """
  Append events with DCB conditions in a single FDB Transaction.

  - `events_json`: JSON-encoded list of `%{type: String, tags: [String], data_b64: String}`
  - `conditions_json`: JSON-encoded list of `%{query_items: [...], after_position: hex | null}`
  - Returns `:ok | {:error, :condition_failed} | {:error, reason}`
  """
  def append_events(_db_ref, _events_json, _conditions_json), do: :erlang.nif_error(:nif_not_loaded)

  @doc "Read all events from the primary events subspace in versionstamp order."
  def read_all_events(_db_ref), do: :erlang.nif_error(:nif_not_loaded)
end
