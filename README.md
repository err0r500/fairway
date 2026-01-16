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
