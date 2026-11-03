# ADR-0001: TimescaleDB for initial time-series storage
Status: Accepted for initial implementation; benchmark-gated.

## Context
Need one-second samples, idempotent writes, time-window reads and pre-aggregated one-year visualization. Early scale 1,000 devices and a small team.
## Decision
Use TimescaleDB hypertables and continuous aggregates, based on PostgreSQL. Select raw/1m/1h retention and enforce uniqueness on (device_id,time).
## Alternatives
ClickHouse (better fit for large analytical volumes, but operational/dedup semantics require careful handling), plain PostgreSQL (requires more manual rollup lifecycle), Prometheus (different data/ingestion contract).
## Consequences
Simpler SQL ecosystem and uniqueness; storage and ingest throughput must be benchmarked. Continuous-aggregate refresh/retention must be configured carefully.
## Revisit
If measured ingest lag, query p95, disk cost or operational burden violates SLO under target concurrency.
