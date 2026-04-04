defmodule Fairway.Fdb.Store do
  @moduledoc """
  `Fairway.Store` implementation backed by FoundationDB via `Fairway.Fdb.Nif`.

  ## Usage

      {:ok, db} = Fairway.Fdb.Store.open("/etc/foundationdb/fdb.cluster")
      runner = Fairway.CommandRunner.new(db, Fairway.Fdb.Store, namespace: "my_app")

  The `store` term passed to `Fairway.Store` callbacks is `%{db: db_ref, namespace: namespace}`.
  """

  @behaviour Fairway.Store

  @doc "Open an FDB database and return a store term."
  def open(cluster_file_path \\ nil, opts \\ []) do
    path = cluster_file_path || System.get_env("FDB_CLUSTER_FILE", "/etc/foundationdb/fdb.cluster")
    namespace = Keyword.get(opts, :namespace, "fairway")

    case Fairway.Fdb.Nif.open_db(path) do
      {:ok, db_ref} -> {:ok, %{db: db_ref, namespace: namespace}}
      {:error, _} = err -> err
    end
  end

  @impl true
  def read_events(%{db: db, namespace: ns}, query_items, opts) do
    Fairway.Fdb.Nif.read_events(db, ns, query_items, opts)
  end

  @impl true
  def append_events(%{db: db, namespace: ns}, events, conditions) do
    Fairway.Fdb.Nif.append_events(db, ns, events, conditions)
  end

  @impl true
  def read_all_events(%{db: db, namespace: ns}) do
    Fairway.Fdb.Nif.read_all_events(db, ns)
  end
end
