defmodule Realworldapp.Automate.SendWelcomeEmail do
  @moduledoc """
  Automation: send a welcome email when a `UserRegistered` event is observed.

  Triggered by: `Realworldapp.Event.UserRegistered`
  Command:      `Realworldapp.Automate.SendWelcomeEmail.Command`
  Queue ID:     `"welcome-email"`

  The command is idempotent — it checks for an existing `UserWelcomeEmailSent`
  event before calling the mailer, so retries are safe.

  ## Deps

      %{mailer: module_implementing_Mailer_behaviour}

  ## Registration

      registry =
        Fairway.AutomationRegistry.new()
        |> Fairway.AutomationRegistry.register(&Realworldapp.Automate.SendWelcomeEmail.build_config/3)

  ## Mailer behaviour

      @callback send_welcome_email(email :: String.t(), username :: String.t()) ::
                  :ok | {:error, term()}
  """

  alias Fairway.{Query, QueryItem, ReadAppender, Event}
  alias Realworldapp.Event.{UserRegistered, UserWelcomeEmailSent}

  # ── Config factory ────────────────────────────────────────────────────────

  @doc "Build the `Fairway.Automation.Config` for this automation."
  def build_config(store, store_module, deps) do
    %Fairway.Automation.Config{
      store:            store,
      store_module:     store_module,
      queue_id:         "welcome-email",
      event_type:       UserRegistered,
      handler:          &to_command/1,
      deps:             deps
    }
  end

  # ── Handler: event → command ──────────────────────────────────────────────

  @doc "Convert a `UserRegistered` event into the command args map."
  def to_command(%Fairway.Event{data: %UserRegistered{id: id, username: username, email: email}}) do
    {__MODULE__.Command, %{user_id: id, username: username, email: email}}
  end

  # ── Command ───────────────────────────────────────────────────────────────

  defmodule Command do
    @moduledoc """
    Send welcome email, then record `UserWelcomeEmailSent` (idempotent).

    `deps` must be a map with key `:send_welcome_email` — a 2-arity function
    `fn email, username -> :ok | {:error, reason}`.
    """

    def run(ra, %{user_id: user_id, username: username, email: email}, %{send_welcome_email: send_fn}) do
      query = Query.new([
        QueryItem.new(types: [UserWelcomeEmailSent], tags: ["user_id:#{user_id}"])
      ])

      {ra, already_sent} =
        ReadAppender.fold_events(ra, query, false, fn
          %{data: %UserWelcomeEmailSent{}}, _ -> {false, true}
          _, acc -> {true, acc}
        end)

      if already_sent do
        :ok
      else
        with :ok <- send_fn.(email, username) do
          ReadAppender.append_events(ra, [
            Event.new(%UserWelcomeEmailSent{user_id: user_id})
          ])
        end
      end
    end
  end
end
