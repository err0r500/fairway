# Fairway

Golang framework for building micromodule systems using eventsourcing with [Dynamic Consistency Boundaries](https://dcb.events), and [FoundationDB](https://www.foundationdb.org).

## State and future plan
- **[dcb/](./dcb/)** : DCB-compliant eventsourcing interface backed by FoundationDB
- **High-level modules** (planned) : coming from a previous attempt using another db
- **CLI** (planned) : Code generation tools

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

---
I’ve just open-sourced a DCB-compliant event store built on top of foundationdb

Here's the link to the repo : https://github.com/err0r500/fairway
It’s obviously a bit rough around the edges  and just the first piece of a larger project (spoiler: an event-sourcing framework “for human beings”).

And from a more personal perspective, the main reason this exists : it’s my latest attempt to find a possible answer to a question I’ve been a bit obsessed with for years : how can we bring the UNIX philosophy to backend development?

If you’re curious, I’ve also documented how the DCB event store is implemented (see in the dcb/ folder on the repo).

Oh and here are the stats for appending events in the store (Mac m1, fdb & client in a docker-compose) : looks like a small production traffic may be handled
