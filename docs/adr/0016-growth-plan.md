# ADR-0016: Grow to 10k through measured gates within a four-person, twelve-week scope

Status: Proposed (2026-11-04). The existing single 2k PASS does not validate 10k or maximum sustainable capacity.

## Decision

Keep Go, TimescaleDB and JetStream initially. Assign four enduring owners with cross-review:

| Owner | Weeks 1–4 | Weeks 5–8 | Weeks 9–12 |
|---|---|---|---|
| Ingestion/worker | Accepted/generated/failed counters, bounded retries and explicit overload; classify failure/DLQ | Bounded consumer concurrency and ACK/timeout reconciliation | Load/worker-crash/ACK-loss exercises and producer backpressure |
| Data | Profile round trips, bulk device-catalog inserts, row/WAL sizing | Index/chunk/rollup tuning; migration baseline and dedup horizon | Commit correctness, retention boundaries and sustained 10k evidence |
| Query/UI | API/browser traces, representative annual history | Bound payload/render cost; query plans under ingest load | Ten-user annual chart regression and canary verification |
| Reliability | Metrics/runbooks, retention fix, infrastructure cost model | Replicated broker/DB, access control, backup/restore and deployment | Failover/restore exercises, dashboards and rollout/rollback ownership |

Week 4 gate: reproducible throughput and lag, validated dedup/rollups, measured storage/WAL, and a queue policy that cannot silently fill with consumed messages. Bulk raw insertion already exists; the per-device catalog loop is the initial round-trip candidate, not a reason to rewrite the whole store.

Week 8 gate: explain the first limiting stage using generator, API, consumer, DB and disk evidence; choose the smallest targeted change. Worker concurrency must preserve transactional dedup and avoid minute/hour lock contention. Benchmark a separate generator/browser host to distinguish client from service limits. Security, restore and replicated durability are production prerequisites. Do not split into new services merely to distribute ownership.

Week 12 gate: 10k devices at 1 sample/s and seven sensors; 2-minute warm-up plus at least 15 measured minutes, three runs near the boundary and a one-hour soak. Require committed rate >=99% of offered, achieved generation close to offered, no sustained queue growth, no unrecoverable loss, visibility p95 <10s, annual browser p95 <1s at ten users, and no sustained resource exhaustion. A 99% throughput threshold permits backlog investigation, not data loss. Include complete annual rollups, resource limits and host details in evidence. Validate failure recovery and rollout 1k → 2k → 5k → 10k with a reversible read/write routing configuration.

## Scope, alternatives and stop conditions

Defer Kubernetes, active-active multi-region, runtime sensor schemas, year-long raw storage, a product alert engine, generic plugin infrastructure and advanced dashboard UX. Implement infrastructure alerts, auth/TLS, backup and schema ownership rather than treating them as optional polish. Replacing the store is justified if week-8 measurements show the required throughput/cost cannot be met with reasonable tuning; a replacement is a separately estimated decision, not an invisible addition to twelve weeks.

Stop a load step before unsafe queue/disk growth, OOM or severe latency collapse. Pause production expansion on durability, reconciliation or SLO failures. If week-8 evidence or budget fails, reduce admitted device count or change the approved resource envelope; do not label offered load as delivered capacity.

Consequences: the team has a bounded scope and reviewable go/no-go gates; 10k delivery remains a target contingent on hardware and evidence. Related: [ADR-0012](0012-worker-delivery-retry-and-dead-letter.md), [ADR-0015](0015-capacity-and-cost.md), [ADR-0018](0018-service-objectives-and-alerting.md).
