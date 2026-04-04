defmodule Fairway.Fdb.Store do
  @moduledoc """
  `Fairway.Store` implementation backed by FoundationDB via `Fairway.Fdb.Nif`.

  The store term is `%{db: db_ref, namespace: namespace}`.
  All NIF calls use JSON-encoded wire format (shared with the Go cgo bindings).

  ## Usage

      {:ok, store} = Fairway.Fdb.Store.open("/etc/foundationdb/fdb.cluster", namespace: "my_app")
      runner = Fairway.CommandRunner.new(store, Fairway.Fdb.Store)
  """

  @behaviour Fairway.Store

  @doc "Open an FDB database and return a store term."
  def open(cluster_file_path \\ nil, opts \\ []) do
    path = cluster_file_path || System.get_env("FDB_CLUSTER_FILE", "/etc/foundationdb/fdb.cluster")
    namespace = Keyword.get(opts, :namespace, "fairway")

    case Fairway.Fdb.Nif.open_db(path, namespace) do
      {:ok, db_ref} -> {:ok, %{db: db_ref, namespace: namespace}}
      {:error, _} = err -> err
    end
  end

  @impl true
  def read_events(%{db: db}, query_items, opts) do
    query_json = Jason.encode!(query_items)
    opts_json = Jason.encode!(normalize_opts(opts))
    case Fairway.Fdb.Nif.read_events(db, query_json, opts_json) do
      {:ok, raw_events} ->
        {:ok, Enum.map(raw_events, &decode_stored_event/1)}
      {:error, _} = err -> err
    end
  end

  @impl true
  def append_events(%{db: db}, events, conditions) do
    events_json = Jason.encode!(Enum.map(events, &encode_event/1))
    conds_json = Jason.encode!(Enum.map(conditions, &encode_condition/1))
    case Fairway.Fdb.Nif.append_events(db, events_json, conds_json) do
      :ok -> :ok
      {:error, :condition_failed} -> {:error, :condition_failed}
      {:error, _} = err -> err
    end
  end

  @impl true
  def read_all_events(%{db: db}) do
    case Fairway.Fdb.Nif.read_all_events(db) do
      {:ok, raw_events} -> {:ok, Enum.map(raw_events, &decode_stored_event/1)}
      {:error, _} = err -> err
    end
  end

  # ── Private helpers ───────────────────────────────────────────────────────────

  # NIF returns {position_hex, type, tags, data_b64} 4-tuples.
  # Store behaviour expects {position_binary, type, tags, data_binary}.
  defp decode_stored_event({pos_hex, type, tags, data_b64}) do
    pos_bin = Base.decode16!(String.upcase(pos_hex))
    data_bin = Base.decode64!(data_b64)
    {pos_bin, type, tags, data_bin}
  end

  # Store behaviour uses {pos_binary, type, tags, data_binary}.
  # NIF wants {type, tags, data_b64}.
  defp encode_event(%{type: type, tags: tags, data: data}) do
    %{"type" => type, "tags" => tags, "data_b64" => Base.encode64(data)}
  end

  defp encode_condition(%{query_items: items, after_position: after_pos}) do
    %{
      "query_items" => items,
      "after_position" => case after_pos do
        nil -> nil
        bin -> Base.encode16(bin, case: :lower)
      end
    }
  end

  defp normalize_opts(opts) do
    %{
      "limit" => Map.get(opts, :limit),
      "after_position" => case Map.get(opts, :after_position) do
        nil -> nil
        bin -> Base.encode16(bin, case: :lower)
      end,
      "reverse" => Map.get(opts, :reverse, false)
    }
  end
end
