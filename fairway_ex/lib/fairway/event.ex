defmodule Fairway.EventData do
  @moduledoc """
  Behaviour for event data structs.

  Each event type (a plain struct, not an Ecto schema) implements this behaviour to declare:
  - `tags/1` — the scoping tags for this event instance (used for DCB and index writes)
  - `type_name/0` — the stable string name used as the key in FDB (defaults to inspect(__MODULE__))

  Example:

      defmodule MyApp.Events.ListCreated do
        @behaviour Fairway.EventData
        @derive Jason.Encoder
        defstruct [:list_id, :name]

        @impl true
        def tags(%{list_id: id}), do: ["list:" <> id]

        @impl true
        def type_name, do: "ListCreated"
      end
  """

  @doc "Return the scoping tags for this event instance."
  @callback tags(struct()) :: [String.t()]

  @doc "Return the stable string type name for serialization (must be unique across the system)."
  @callback type_name() :: String.t()
end

defmodule Fairway.Event do
  @moduledoc """
  A domain event with a typed data payload.

  `occurred_at` is set to `DateTime.utc_now()` by default.
  `data` must implement `Fairway.EventData`.
  """

  @enforce_keys [:data]
  defstruct [:data, :occurred_at]

  @type t :: %__MODULE__{
          data: struct(),
          occurred_at: DateTime.t()
        }

  @doc "Create a new event with the current UTC time."
  def new(data) do
    %__MODULE__{data: data, occurred_at: DateTime.utc_now()}
  end

  @doc "Create a new event with an explicit timestamp."
  def new(data, occurred_at) do
    %__MODULE__{data: data, occurred_at: occurred_at}
  end

  @doc """
  Serialize a `Fairway.Event` into the wire format expected by the store:
  `%{type: String.t(), tags: [String.t()], data: binary()}`.

  The `data` field is JSON-encoded via Jason.
  Wraps the payload with `occurred_at` for full event reconstruction.
  """
  def to_store_event(%__MODULE__{data: data, occurred_at: occurred_at}) do
    module = data.__struct__
    type_name = module.type_name()
    tags = module.tags(data)

    envelope = %{
      occurred_at: DateTime.to_iso8601(occurred_at),
      data: data
    }

    case Jason.encode(envelope) do
      {:ok, json} ->
        {:ok, %{type: type_name, tags: tags, data: json}}
      {:error, _} = err ->
        err
    end
  end

  @doc """
  Deserialize a stored event `{position, type, tags, data_bytes}` back into a `Fairway.Event`.
  Requires the type name to be registered in `Fairway.Registry`.
  """
  def from_store_event({position, type_name, _tags, data_bytes}) do
    with {:ok, module} <- Fairway.Registry.lookup(type_name),
         {:ok, envelope} <- Jason.decode(data_bytes, keys: :atoms),
         {:ok, occurred_at, _} <- DateTime.from_iso8601(to_string(envelope.occurred_at)),
         data_struct <- struct(module, Map.drop(envelope, [:occurred_at])) do
      {:ok, position, %__MODULE__{data: data_struct, occurred_at: occurred_at}}
    else
      {:error, :not_registered} ->
        {:error, {:unknown_type, type_name}}
      {:error, reason} ->
        {:error, {:decode_failed, reason}}
    end
  end
end
