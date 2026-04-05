defmodule Realworldapp.Automate.SendWelcomeEmailTest do
  use ExUnit.Case, async: true

  alias Fairway.Testing.Store
  alias Fairway.{Automation, AutomationRegistry}
  alias Realworldapp.Automate.SendWelcomeEmail
  alias Realworldapp.Event.{UserRegistered, UserWelcomeEmailSent}

  # ── Setup ──────────────────────────────────────────────────────────────────

  setup do
    Fairway.Registry.register_all([UserRegistered, UserWelcomeEmailSent])
    {:ok, store} = Store.start_link()
    %{store: store}
  end

  # ── Helpers ────────────────────────────────────────────────────────────────

  # Builds deps with an in-process sent-mail accumulator (Agent).
  defp make_mailer do
    {:ok, agent} = Agent.start_link(fn -> [] end)

    send_fn = fn email, username ->
      Agent.update(agent, fn sent -> [{email, username} | sent] end)
      :ok
    end

    {%{send_welcome_email: send_fn}, fn -> Enum.reverse(Agent.get(agent, & &1)) end}
  end

  # Starts an automation with fast polling for tests.
  defp start_automation(store, deps, extra_opts \\ []) do
    opts =
      Keyword.merge([poll_interval_ms: 20, retry_base_ms: 50], extra_opts)

    config =
      SendWelcomeEmail.build_config(store, Store, deps)
      |> struct!(opts)

    Automation.start_link(config)
  end

  # Inserts a UserRegistered event directly (no command runner needed).
  defp seed_user(store, id, username, email) do
    Store.given_events(store, [
      %{
        type: UserRegistered.type_name(),
        tags: UserRegistered.tags(%UserRegistered{id: id, username: username, email: email}),
        data:
          Jason.encode!(%{
            occurred_at: DateTime.to_iso8601(DateTime.utc_now()),
            data: %{id: id, username: username, email: email, hashed_password: "x"}
          })
      }
    ])
  end

  # Poll until the store holds at least `count` events, or timeout.
  defp wait_for_event_count(store, count, timeout_ms \\ 2_000) do
    deadline = System.monotonic_time(:millisecond) + timeout_ms

    Enum.reduce_while(Stream.repeatedly(fn -> nil end), nil, fn _, _ ->
      {:ok, events} = Store.read_all_events(store)

      if length(events) >= count do
        {:halt, :ok}
      else
        remaining = deadline - System.monotonic_time(:millisecond)

        if remaining <= 0 do
          {:halt, {:error, :timeout}}
        else
          :timer.sleep(10)
          {:cont, nil}
        end
      end
    end)
  end

  # ── Core behaviour ─────────────────────────────────────────────────────────

  test "sends welcome email and records UserWelcomeEmailSent", %{store: store} do
    {deps, sent} = make_mailer()
    {:ok, pid} = start_automation(store, deps)
    on_exit(fn -> Automation.stop(pid) end)

    seed_user(store, "u1", "alice", "alice@example.com")

    assert :ok = wait_for_event_count(store, 2)
    assert sent.() == [{"alice@example.com", "alice"}]

    {:ok, all} = Store.read_all_events(store)
    types = Enum.map(all, fn {_, type, _, _} -> type end)
    assert UserWelcomeEmailSent.type_name() in types
  end

  test "does not send duplicate emails (idempotent)", %{store: store} do
    {deps, sent} = make_mailer()
    {:ok, pid} = start_automation(store, deps)
    on_exit(fn -> Automation.stop(pid) end)

    seed_user(store, "u1", "alice", "alice@example.com")
    assert :ok = wait_for_event_count(store, 2)

    # Seed the same user again (simulates a re-delivered event)
    seed_user(store, "u1", "alice", "alice@example.com")
    assert :ok = wait_for_event_count(store, 3)

    # Still only one email — the UserWelcomeEmailSent guard prevents re-sending
    assert sent.() == [{"alice@example.com", "alice"}]
  end

  test "each user gets exactly one email", %{store: store} do
    {deps, sent} = make_mailer()
    {:ok, pid} = start_automation(store, deps, num_workers: 4)
    on_exit(fn -> Automation.stop(pid) end)

    seed_user(store, "u1", "alice", "alice@example.com")
    seed_user(store, "u2", "bob", "bob@example.com")

    assert :ok = wait_for_event_count(store, 4)

    emails = sent.()
    assert length(emails) == 2
    assert {"alice@example.com", "alice"} in emails
    assert {"bob@example.com", "bob"} in emails
  end

  # ── Retry and DLQ ─────────────────────────────────────────────────────────

  test "failed jobs land in DLQ after max_attempts", %{store: store} do
    always_fail = %{send_welcome_email: fn _, _ -> {:error, :smtp_down} end}

    {:ok, pid} =
      start_automation(store, always_fail, max_attempts: 2, retry_base_ms: 20)

    on_exit(fn -> Automation.stop(pid) end)

    seed_user(store, "u1", "alice", "alice@example.com")

    # Wait long enough for 2 attempts (initial + 1 retry at ~20ms backoff)
    :timer.sleep(500)

    dlq = Automation.list_dlq(pid)
    assert length(dlq) == 1
    assert hd(dlq).attempts == 2
    assert hd(dlq).error =~ "smtp_down"
  end

  test "DLQ replay re-processes the job successfully", %{store: store} do
    {deps, sent} = make_mailer()

    # First call fails, second succeeds. max_attempts: 1 → DLQ immediately on first failure.
    attempt_ref = :counters.new(1, [])

    flaky_deps = %{
      send_welcome_email: fn email, username ->
        n = :counters.get(attempt_ref, 1)
        :counters.add(attempt_ref, 1, 1)

        if n == 0 do
          {:error, :smtp_flaky}
        else
          (deps.send_welcome_email).(email, username)
        end
      end
    }

    # max_attempts: 1 → first failure goes straight to DLQ (no retry wait)
    {:ok, pid} = start_automation(store, flaky_deps, max_attempts: 1)
    on_exit(fn -> Automation.stop(pid) end)

    seed_user(store, "u1", "alice", "alice@example.com")

    # Wait for first attempt to fail and land in DLQ
    :timer.sleep(300)

    dlq = Automation.list_dlq(pid)
    assert length(dlq) == 1, "expected 1 DLQ entry, got: #{inspect(dlq)}"

    [entry] = dlq
    :ok = Automation.replay_dlq(pid, entry.position)

    # Second attempt succeeds → UserWelcomeEmailSent appended
    assert :ok = wait_for_event_count(store, 2)
    assert length(sent.()) == 1
    assert Automation.list_dlq(pid) == []
  end

  test "purge_dlq removes entries older than cutoff", %{store: store} do
    always_fail = %{send_welcome_email: fn _, _ -> {:error, :smtp_down} end}

    {:ok, pid} = start_automation(store, always_fail, max_attempts: 2, retry_base_ms: 20)
    on_exit(fn -> Automation.stop(pid) end)

    seed_user(store, "u1", "alice", "alice@example.com")
    :timer.sleep(400)

    assert length(Automation.list_dlq(pid)) == 1

    # Purge everything up to now+1s
    future = DateTime.add(DateTime.utc_now(), 1, :second)
    :ok = Automation.purge_dlq(pid, future)

    assert Automation.list_dlq(pid) == []
  end

  # ── Registry ───────────────────────────────────────────────────────────────

  test "AutomationRegistry starts automations and stop_fn shuts them down", %{store: store} do
    {deps, sent} = make_mailer()

    registry =
      AutomationRegistry.new()
      |> AutomationRegistry.register(fn s, sm, d ->
        SendWelcomeEmail.build_config(s, sm, d) |> struct!(poll_interval_ms: 20)
      end)

    {:ok, stop} = AutomationRegistry.start_all(registry, store, Store, deps)

    seed_user(store, "u1", "alice", "alice@example.com")
    assert :ok = wait_for_event_count(store, 2)
    assert length(sent.()) == 1

    stop.()
  end

  test "AutomationRegistry rejects duplicate queue_ids", %{store: store} do
    always_fail = %{send_welcome_email: fn _, _ -> {:error, :noop} end}

    registry =
      AutomationRegistry.new()
      |> AutomationRegistry.register(&SendWelcomeEmail.build_config/3)
      |> AutomationRegistry.register(&SendWelcomeEmail.build_config/3)

    assert {:error, {:duplicate_queue_id, "welcome-email"}} =
             AutomationRegistry.start_all(registry, store, Store, always_fail)
  end
end
