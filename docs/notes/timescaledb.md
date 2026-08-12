# TimescaleDB notes

Working reference for hypertable + columnstore decisions in keelwave. Pulled from official TigerData / Timescale docs and verified against our Timescale 2.27.x image.

## What it is

Postgres extension (not a separate DB). Tables stay regular Postgres — SQL, joins, FKs, transactions, pgx all unchanged. The extension adds:

- **Hypertables** — one logical table, auto-sharded into time-based **chunks**
- **Columnstore / compression** — old chunks compressed 10–20× (Timescale claims up to 95% storage reduction)
- **Continuous aggregates** — materialised time-bucketed views
- **Retention policies** — drop old chunks automatically
- **`time_bucket()`** + friends for time-windowed grouping

## Hypertables and chunks

A hypertable presents as one table. Inserts auto-route into the **chunk** covering that row's timestamp. Default chunk window = **7 days**.

```
ai_traces (hypertable, what you query)
├── _hyper_1_1_chunk  (Jan 1-7)
├── _hyper_1_2_chunk  (Jan 8-14)
└── ...
```

`SELECT … WHERE timestamp > now() - '1 day'` → planner reads chunk min/max and skips chunks outside range (**chunk exclusion**). Active chunk fits in RAM = fast inserts.

### Why partition column must be in PK
Postgres can't enforce uniqueness across chunks without scanning all of them. So Timescale requires the partition column inside the PK:

```sql
PRIMARY KEY (id, timestamp)   -- not just (id)
```

## Two ways to create a hypertable

### A. Modern (`WITH` clause, Timescale ≥ 2.20.0)
```sql
CREATE TABLE ai_traces (...) WITH (
    tsdb.hypertable,
    tsdb.partition_column = 'timestamp',
    tsdb.segmentby        = 'project_id'
);
```
Single statement, declarative. Auto-creates columnstore policy.

### B. Classic (function call, all versions)
```sql
CREATE TABLE ai_traces (...);
SELECT create_hypertable('ai_traces', by_range('timestamp'));
ALTER TABLE ai_traces SET (timescaledb.compress, timescaledb.compress_segmentby = 'project_id');
SELECT add_compression_policy('ai_traces', INTERVAL '7 days');
```
More verbose, returns metadata, every guide uses it.

**Keelwave uses A.** Our Timescale version supports it; bundling compression into one DDL keeps migrations tighter.

## `tsdb.*` options (full list, docs)

| Option | Default | Purpose |
|---|---|---|
| `tsdb.hypertable` | `true` | Make this a hypertable |
| `tsdb.columnstore` | `true` | Auto-create columnstore (compression) policy |
| `tsdb.partition_column` | first `TIMESTAMP[TZ]` col | Time column to chunk by |
| `tsdb.chunk_interval` | `7 days` | Window per chunk |
| `tsdb.create_default_indexes` | `true` | Auto B-tree on partition + space cols |
| `tsdb.associated_schema` | `_timescaledb_internal` | Where chunks live |
| `tsdb.associated_table_prefix` | `_hyper` | Chunk naming |
| `tsdb.orderby` | `<time> DESC` | Columnstore row order |
| `tsdb.segmentby` | auto-picked from first batch | Columnstore segment dimension |
| `tsdb.sparse_index` | auto | Indexes on compressed chunks |

## `segmentby` — the option that actually matters

Not metadata. Changes the **on-disk physical layout of compressed chunks**.

### What compression does
1. Chunk ages past policy window (e.g. 7 days)
2. Rows grouped into batches of ~1000
3. Batches grouped **by `segmentby` value**
4. Per-column encoders applied:
   - timestamps → delta-of-delta
   - floats → gorilla
   - low-cardinality strings → dictionary
5. Compressed chunk replaces uncompressed one. Still queryable.

### Why segmentby choice matters
- Queries filtering on the segmentby column **only decompress matching segments** — fast.
- Queries that ignore it = decompress the whole chunk.
- **Low cardinality = better compression ratio** (more rows per group).
- **High cardinality = worse compression + slower scans** (many tiny groups).

Official: *"The columnstore is optimized for 1000 rows per batch per `segmentby` value."*

### Keelwave's choices

| Table | segmentby | Why |
|---|---|---|
| `ai_traces` | `project_id` | All queries tenant-scoped via API key middleware |
| `api_events` | `project_id` | Same |
| `agent_runs` | `project_id` | Same |
| `infra_metrics` | `project_id, host` | Queries always pick specific host |
| `agent_steps` | `project_id, agent_run_id` | Timeline reads = one run's steps; segmenting by run ID avoids decompressing the whole chunk for the agent inspector UI |

### Bad segmentby choices (avoid)
- `request_id` / any UUID per row → millions of groups, kills compression
- A column never used in `WHERE` → no query speedup
- Leaving it unset → Timescale guesses from first batch, might guess wrong

## Why we removed `CREATE EXTENSION pgcrypto`

Original 000001 had `CREATE EXTENSION pgcrypto` for `gen_random_uuid()`. Dropped because:
- PG 18 ships native `uuidv7()` — no extension needed
- API key hashes are computed in Go (`crypto/sha256`), not in DB
- One less extension to keep in sync

## Things to know / gotchas

- **No `UPDATE` of partition column** — Postgres can't move a row across chunks. Fine for ingest (we never update `timestamp`).
- **Unique constraints** must include partition column (same reason as PK rule).
- **Don't drop the `timescaledb` extension** in a down migration — would cascade-nuke every hypertable. Extension stays installed across all migrations.
- **Chunk size tuning** — too small = millions of chunks = planner overhead. Too big = lose pruning benefit. Default 7d works for most ingest patterns.
- **`schema_migrations` table** is plain Postgres, not a hypertable. Untouched by Timescale.

## Future things to add (when we need them)
- `add_retention_policy('ai_traces', INTERVAL '90 days')` — auto-drop old chunks
- `CREATE MATERIALIZED VIEW … WITH (timescaledb.continuous)` — pre-computed rollups (p95 latency per minute, hourly cost, etc.)
- Manual compression force: `SELECT compress_chunk(...)` for backfills

## Sources

Tiger Data is the renamed entity behind TimescaleDB; the old `docs.timescale.com` URLs redirect to `tigerdata.com`. The archived `docs.timescale.com-content` GitHub repo (last touched 2021) is stale — do not cite.

- [Tiger Data — CREATE TABLE WITH tsdb.hypertable](https://www.tigerdata.com/docs/api/latest/hypertable/create_table)
- [Tiger Data — Compression overview](https://docs.tigerdata.com/use-timescale/latest/compression/)
- [Tiger Data — About compression / hypercore](https://www.tigerdata.com/docs/use-timescale/latest/compression/about-compression)
- [Tiger Data — Compression API reference](https://www.tigerdata.com/docs/api/latest/compression)
- [Tiger Data — compression_settings view](https://docs.tigerdata.com/api/latest/informational-views/compression_settings/)
- [Tiger Data — Changelog](https://www.tigerdata.com/docs/about/latest/changelog)
- [TimescaleDB CHANGELOG (GitHub)](https://github.com/timescale/timescaledb/blob/main/CHANGELOG.md) — engine release notes, still authoritative
