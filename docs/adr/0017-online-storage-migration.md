# ADR-0017: Migrate retained data using a consistent snapshot, CDC and fenced cutover

Status: Proposed (2026-11-04); migration tooling, CDC, checkpoints and adapter boundaries are not implemented yet.

## Context and contract

Replace storage while HTTP ingestion and queries remain available, preserving every acknowledged sample and all data inside the retention contract. Already expired second-level data cannot be recovered from aggregates. PostgreSQL uniqueness, atomic raw/rollup updates, first-write-wins and chart gaps must remain observable contracts even if the target has different primitives. A second independent write in an HTTP handler cannot make two stores atomic; we reject that approach.

## Preparation

Implement storage ports per ADR-0011 and versioned migrations per ADR-0013. Establish stable sample IDs, a canonical duplicate winner, a durable event/commit journal, contiguous consumer checkpoints and write-owner fencing. Current raw-key dedup expires when raw is dropped; add a dedup ledger covering the supported replay horizon or restrict replay with proven checkpoints. Test older duplicates explicitly before allowing a long migration/replay window.

Budget a journal and WAL/CDC retention longer than worst-case copy + catch-up + rollback, with independent disk alarms and archives. The current 2 GiB/48h broker configuration is insufficient. Logical slot retention must not fill the source disk. Protect source snapshots from retention deletion by pinning/exporting the data needed for copy; track subsequent retention deletes separately from unexpected loss.

## Procedure

1. Keep the source authoritative. Establish a consistent PostgreSQL snapshot and associated logical replication LSN without a gap. Copy device metadata and all three retained tiers: raw, minute, hour. Copying raw alone cannot reconstruct expired historical aggregates.
2. Stream committed changes from the snapshot boundary with durable checkpoints and idempotent application. Apply source row after-images/deletes for rollups, rather than adding deltas again or independently deriving them from the same CDC raw rows. Identify changes by source transaction/LSN, operation and primary key, so retried transfer is safe. Preserve the original first-write winner.
3. A target without multi-table transactions needs staging plus a published commit manifest/watermark; readers must not observe half-applied raw/rollup commits. Reject a target that cannot provide the required observable consistency at acceptable cost. Verify this adapter contract before declaring interchangeability.
4. Reconcile counts, deterministic checksums and sum/min/max/count per device/day and tier at a **common source watermark**. Account for approved retention deletions. Shadow queries compare bucket boundaries, gaps, UTC, sensor selection, partial buckets and latency. Canary readers move gradually; route back on mismatch or SLO regression.
5. For write cutover, choose a durable contiguous broker barrier B and stop/fence the old processing owner after committing all messages through B. Continue API acceptance into the durable queue; ingestion processing can briefly lag within its SLO. Record the corresponding source commit watermark, apply CDC through it and verify the target. Activate the target processor only for work after B, with replay dedup for already-existing sample keys. Switch readers only when the target contains the required watermark. An epoch/lease prevents an old worker restarting and becoming a second write owner.
6. Keep a seven-day rollback window **only if** the old projection remains current from the same durable canonical commit/event journal. Duplicate winner/order must remain consistent in both stores; do not rely on concurrent additive rollup writes choosing the same winner by chance. Alternatively rebuild a current old projection before rollback. Switching to a stale old DB after target-only writes would lose visible data. Reconcile again before removing old snapshots, checkpoints or infrastructure under an explicit approved decommission step.

## Failure handling and acceptance

Abort read/write cutover on checksum or semantic mismatch, unknown dedup coverage, excessive lag, journal/slot disk risk, missing checkpoint, untested restore or target capacity failure. Source serving continues; a target outage restarts from its checkpoint. Exercise crashes between copy/checkpoint, DB commit/ACK, fencing/activation and target commit/rollback replication. RPO=0 here concerns planned migration under the declared fault model with intact durable source/journal; it does not claim arbitrary simultaneous media loss is survivable.

Success means no HTTP interruption caused by cutover, no lost/duplicated retained samples or aggregates at reconciled watermarks, bounded freshness and a demonstrated rollback containing post-cutover writes. Related: [ADR-0007](0007-transactional-rollups.md), [ADR-0011](0011-application-ports-and-infrastructure-adapters.md), [ADR-0013](0013-versioned-migrations-and-schema-ownership.md).
