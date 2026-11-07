# ADR-0007: Transactionally updated rollups rather than Timescale continuous aggregates
Status: Accepted for this implementation (supersedes implementation detail in ADR-0001/0003).

## Context
Raw may be deleted after 7 days, but 365-day historical ingestion must be supported. Scheduled continuous-aggregate refresh windows can miss historical late arrivals, while refreshing from already-expired source risks losing old aggregates.

## Decision
In one transaction, `INSERT ... ON CONFLICT DO NOTHING RETURNING` obtains newly accepted source rows; aggregate those rows into minute and hour min/max/sum/count for seven sensors with conflict-updates. Both resolutions and raw are committed atomically. Retention policies for raw/minute/hour are independent. Replays do not affect aggregates.

## Trade-offs
A heavier write path and potential row lock hot spots on per-device/hour buckets, especially for large historical backfills. Optimizes consistency and explainability over maximum ingest throughput. Consider staging/COPY, bounded aggregation batches, asynchronous refresh, and/or analytical stores after benchmark evidence. First write wins for conflicting duplicate samples.

## Idempotency horizon
The implementation deduplicates against the raw primary key. Once raw retention deletes that key, replaying the expired sample can insert it again and increment surviving rollups. The replay guarantee therefore requires retained dedup keys or a controlled replay boundary; it is not an indefinite event ledger. A longer-horizon ledger/checkpoint scheme is a prerequisite for the migration described in [ADR-0017](0017-online-storage-migration.md).
