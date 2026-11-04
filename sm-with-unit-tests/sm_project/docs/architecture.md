# Architecture: Server Monitoring

## Scope and assumptions
Seven 0–100% measurements arrive every second per server, one sample containing seven sensor values. Baseline 1,000 devices; evaluate 10,000 and 100,000 as growth scenarios. Graphs for one device, any subset of sensors, over 5m–1y. Target p95 <1s for a year-long chart, with target workload/hardware explicitly reported by benchmark.

## HLD


```
Go Simulator -> Go Ingest API -> NATS JetStream -> Go Worker -> TimescaleDB
                                                              ^
Single-file HTML + JS chart <- Go Query API ------------------|
```
Modular-monolith API (ingestion/query separated in source) plus independently runnable worker; no unnecessary services.

## Data and retention
- Raw samples: 1 second, 7 days
- 1-minute rollups: 45 days
- 1-hour rollups: 400 days
- Aggregate per sensor: min, max, sum, count; optional first/last if needed.
- Retention policies must not refresh an aggregate from already-deleted source data. Complete and validate higher resolution rollups before deleting the source.
- Historical data >= 7 days old can be accepted only using an explicit supported backfill procedure; standard late-arrival SLA: <= 48 hours.

## Data model
Raw PK (`device_id`, `time`), seven `REAL NOT NULL` columns, CHECK values 0..100. Numeric internal device ID; external searchable ID in devices. At-least-once delivery; DB conflict ignore with divergence telemetry (`first write wins`). UTC timestamps, boundaries `[from,to)`.

## Ingestion semantics
- API validates payload size, device identity, timestamps, all seven values.
- API publishes to JetStream with file-backed durable storage and acknowledges HTTP 202 only after publish ack.
- Worker batch-inserts transactionally, acknowledges messages after commit. Retry with bounded exponential backoff and dead-letter handling for poison messages.
- For host or disk destruction, single-host Compose deployment **does not guarantee** no loss; replication and backups required in production.
- Device sender retries with stable sample ID; overload yields 429/503, never silent success.

## Query strategy
- API accepts device, sensor subset, from, to, max_points (server-enforced ceiling).
- Raw for short ranges, minute rollups for intermediate ranges, hour rollups for long ranges.
- Group rows into buckets approximately `ceil(range/max_points)`, select bucket widths aligned with stored resolution. For 365d and ~1500 points, 6-hour buckets => ~1460 points per sensor.
- Show average line with optional min/max band; preserve missing buckets as gaps, not zeroes.
- Stitch fresh raw data with materialized aggregates at exact closed bucket boundaries to avoid double count or missing points.
- Separate latency metrics for DB, HTTP, browser render, and full end-to-end.

## API contracts (initial)
`POST /api/v1/metrics/batch` -> 202 after durable queue ack.
`GET /api/v1/devices?search=...&limit=...` -> paginated results.
`GET /api/v1/metrics/query?device_id=...&metrics=cpu,memory&from=...&to=...&max_points=1500` -> aligned bucket data for selected sensors; `min/max/avg/count` for each bucket; gaps represented by null.
`GET /health/live`, `/health/ready` -> health endpoints.
`GET /` -> embedded single HTML UI, searchable device picker, metric toggles, time range controls, 0..100 Y axis.

## Capacity (uncompressed row count)
| Devices | Samples/sec | Samples/day | Samples/year | Raw 7d | Minute 45d | Hour 400d |
|---:|---:|---:|---:|---:|---:|---:|
|1,000|1,000|86,400,000|31,536,000,000|604,800,000|64,800,000|9,600,000|
|10,000|10,000|864,000,000|315,360,000,000|6,048,000,000|648,000,000|96,000,000|
|100,000|100,000|8,640,000,000|3,153,600,000,000|60,480,000,000|6,480,000,000|960,000,000|

At 80–160 B effective per raw sample, 1,000 devices × 7 days ≈ 48–97 GB before accounting for all indexes, compression effects, replicas, backups and WAL. These are planning estimates, *not* benchmark numbers.

## Quality gates
- Ingestion correctness, idempotency, out-of-order and late arrivals, crash after DB commit before ACK.
- Refresh/retention boundary tests, missing data representation, multi-sensor alignment, query range limit.
- k6 concurrent ingestion/query workloads (including 1y, 7 sensors), report p50/p95/p99, resource usage, dropped events, publish lag, aggregation freshness and browser rendering.
- Faults: DB unavailable, NATS unavailable, worker crash, restart after backfill, queue capacity reached.

## Risks and evolution
- TimescaleDB query/write performance must be benchmarked before claiming 100k-device support; ClickHouse is a candidate for analytical expansion.
- Disk sizing, queue retention and DB WAL must be bounded, tested, and monitored.
- Single-host Docker Compose is a demo/developer deployment, not high availability.
- Ownership: ingestion/worker, data/rollups, query/UI, reliability/CI — explicit interfaces support parallel team work.
