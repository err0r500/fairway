<p align="center">
  <img src="./doc/fairway.png">
</p>

## ⚠️ Experimental (Work in Progress)

This project is **experimental** and under **heavy work**. It’s **not published yet**.

I'm progressively migrating code from the private repo of a previous attempt

✅ Feel free to try it locally:
- clone the repo
- toy with the example 🙂

# Fairway
Golang framework for building micromodule systems using eventsourcing with [Dynamic Consistency Boundaries](https://dcb.events), and [FoundationDB](https://www.foundationdb.org).

## What Fairway provides

### Decoupled domain modeling
- Each command defines only the minimal model it needs to make its decision
- No shared domain model across commands - each command is independent

### Fine-grained consistency & concurrency
- Optimistic concurrency limited to the data a command actually reads
- No contention on unrelated data - commands only conflict when they read the same events

### Independent command evolution
- Commands share no code, communication, or underlying streams
- No hidden coupling - each command is a separate slice of functionality
- Zero merge conflicts via code generation and self-registration

### Schema-free evolution
- No data migrations when commands or views evolve (including no stream refactoring)
- Event sourcing: new versions simply reinterpret existing events differently
- Past facts remain unchanged, interpretations can evolve

### Single datastore for everything
- Leverages FoundationDB for all needs: queues, persistent read models, events
- Easy enforcement of unique constraints across the entire system
- No operational complexity from managing multiple databases

---

> -- Is it a good idea ?
>
> -- I'm not sure, yet.
>
> -- Is it worth exploring ?
>
> -- Absolutely, yes !

---

## Micromodules: UNIX philosophy for backends
System behavior emerges from tiny, independent modules that compose through a shared event log.

**Principles:**
- Does one thing well (changes small part of system or displays small info slice)
- Disposable (replaceable anytime, no breakage, no migration)
- Minimal state (_only_ what's needed for its specific task)
- Consistency boundaries limited to just the data needed for command decisions
- No inter-module communication (compose via shared log)

**Patterns** (from [Event Modeling](https://eventmodeling.org/)):

Each micromodule implements exactly one pattern:

- **Command** - State change from trigger to system modification
- **View** - Connects events to representation
- **Automation** - Uses View to trigger a Command without user intervention

## State and future plan
- **[dcb/](./dcb/)** : DCB-compliant eventsourcing interface backed by FoundationDB
- **High-level modules** (ongoing) : coming from a previous attempt using another db
- **CLI** (planned) : Code generation tools
