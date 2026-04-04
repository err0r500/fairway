defmodule Fairway.Fdb.Nif do
  @moduledoc """
  Rustler NIF wrapper for the Fairway FDB store.

  All functions run on Tokio async threads (DirtyIo schedule) so they never
  block the BEAM scheduler.

  These functions are called by `Fairway.Fdb.Store` and should not be called
  directly from application code.
  """

  use Rustler, otp_app: :fairway, crate: :fairway_fdb

  @doc "Open an FDB database. Returns {:ok, db_ref} or {:error, reason}."
  def open_db(_cluster_file_path), do: :erlang.nif_error(:nif_not_loaded)

  @doc """
  Read events matching query_items within a single FDB ReadTransaction.

  - `query_items`: list of `%{types: [String], tags: [String]}` maps
  - `opts`: `%{limit: integer | nil, after_position: binary | nil, reverse: boolean}`
  - Returns `{:ok, [{position_bin, type, tags, data_bin}]}` or `{:error, reason}`
  """
  def read_events(_db_ref, _namespace, _query_items, _opts), do: :erlang.nif_error(:nif_not_loaded)

  @doc """
  Append events with DCB conditions in a single FDB Transaction.

  - `events`: list of `%{type: String, tags: [String], data: binary}`
  - `conditions`: list of `%{query_items: [...], after_position: binary | nil}`
  - Returns `:ok | {:error, :condition_failed} | {:error, reason}`
  """
  def append_events(_db_ref, _namespace, _events, _conditions), do: :erlang.nif_error(:nif_not_loaded)

  @doc "Read all events from the primary events subspace in versionstamp order."
  def read_all_events(_db_ref, _namespace), do: :erlang.nif_error(:nif_not_loaded)
end
