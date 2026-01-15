# DCB Event Store - Todo Benchmark

Real-world performance benchmark simulating a todo list application with full observability.

## Overview => OUTDATED: for now best scenario append only (no body, no tag)

This benchmark continuously runs realistic todo list scenarios to measure DCB event store performance over time:

**Each scenario (16 events total):**
1. Create list (1 event)
2. Insert 10 items with reads before each insert (10 events, 10 reads)
3. Update 3 items with reads (3 events, 3 reads)
4. Delete 2 items with reads (2 events, 2 reads)
5. Delete list with read (1 event, 1 read)

**Total per scenario:** 16 appends, 16 reads

**Concurrency:** 50 parallel scenarios (configurable)

## Quick Start

### Using Docker Compose (Recommended)

**Prerequisites for Apple Silicon:**
Build the custom FDB image first ([source](https://forums.foundationdb.org/t/foundationdb-on-docker-mac-m2/3876/13)):

```bash
git clone git@github.com:apple/foundationdb.git
cd ./foundationdb/packaging/docker
docker build --build-arg FDB_VERSION=7.4.5 -t foundationdb/foundationdb:7.4.5-silicon --target=foundationdb .
```


**Start the full stack** (FoundationDB + Benchmark + Prometheus + Grafana):

```bash
cd examples/todo-bench
(if on apple silicon) : export FDB_CUSTOM_TAG=-silicon
docker-compose up --build
```

initialize the foundation db
```
docker exec todo-bench-fdb-1 fdbcli --exec "configure new single ssd"
```

Access:
- **Grafana Dashboard**: http://localhost:3000 (admin/admin)
- **Prometheus**: http://localhost:9090
- **Metrics Endpoint**: http://localhost:8080/metrics
- **pprof**: http://localhost:8080/debug/pprof/

The Grafana dashboard "DCB Event Store - Todo Benchmark" will be pre-loaded.

**Note:** The benchmark container shares the FDB cluster file via a Docker volume (`fdb-config`), so no manual cluster file setup is needed.
