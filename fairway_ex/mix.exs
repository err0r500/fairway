defmodule Fairway.MixProject do
  use Mix.Project

  def project do
    [
      app: :fairway,
      version: "0.1.0",
      elixir: "~> 1.16",
      start_permanent: Mix.env() == :prod,
      deps: deps(),
      compilers: [:rustler] ++ Mix.compilers(),
      rustler_crates: rustler_crates()
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {Fairway.Application, []}
    ]
  end

  defp deps do
    [
      {:rustler, "~> 0.32"},
      {:jason, "~> 1.4"}
    ]
  end

  defp rustler_crates do
    [
      fairway_fdb: [
        path: "native/fairway_fdb",
        mode: rustler_mode()
      ]
    ]
  end

  defp rustler_mode do
    if Mix.env() == :prod, do: :release, else: :debug
  end
end
