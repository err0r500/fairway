defmodule Fairway.QueryItem do
  @moduledoc """
  A single clause in a query.
  - `types` — list of event module atoms (OR: match any)
  - `tags`  — list of tag strings (AND: must have all)

  At least one of types or tags must be non-empty.
  """

  defstruct types: [], tags: []

  @type t :: %__MODULE__{
          types: [module()],
          tags: [String.t()]
        }

  def new(opts \\ []) do
    %__MODULE__{
      types: Keyword.get(opts, :types, []),
      tags: Keyword.get(opts, :tags, [])
    }
  end

  def types(item, modules), do: %{item | types: modules}
  def tags(item, tag_list), do: %{item | tags: tag_list}

  @doc "Convert to the wire format map expected by the store: %{types: [String], tags: [String]}."
  def to_wire(%__MODULE__{types: type_modules, tags: tags}) do
    type_strings = Enum.map(type_modules, fn mod ->
      if function_exported?(mod, :type_name, 0) do
        mod.type_name()
      else
        inspect(mod)
      end
    end)
    %{types: type_strings, tags: tags}
  end
end

defmodule Fairway.Query do
  @moduledoc """
  A union of query items (OR semantics between items).
  """

  defstruct items: [], opts: %{}

  @type t :: %__MODULE__{
          items: [Fairway.QueryItem.t()],
          opts: map()
        }

  def new(items \\ [], opts \\ %{}) when is_list(items) do
    %__MODULE__{items: items, opts: opts}
  end

  def add_item(%__MODULE__{} = query, %Fairway.QueryItem{} = item) do
    %{query | items: query.items ++ [item]}
  end

  @doc "Convert all items to the wire format list."
  def to_wire_items(%__MODULE__{items: items}) do
    Enum.map(items, &Fairway.QueryItem.to_wire/1)
  end

  @doc "Extract read opts as a map suitable for the store."
  def to_read_opts(%__MODULE__{opts: opts}) do
    %{
      limit: Map.get(opts, :limit),
      after_position: Map.get(opts, :after_position),
      reverse: Map.get(opts, :reverse, false)
    }
  end
end
