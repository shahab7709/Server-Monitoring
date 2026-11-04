# ADR-0013: Use versioned database migrations with single-runner ownership
Status: Proposed (2026-11-04).

## Context
Both `cmd/api/main.go` and `cmd/worker/main.go` call `Store.Migrate()` on startup. Current DDL is embedded in `internal/storage/storage.go`, guarded by a PostgreSQL advisory transaction lock, and initial retention setup runs in the API. A growing schema needs an auditable order of changes, predictable rollback/recovery and separation between serving traffic and administering schema.

## Decision
Store ordered, immutable migration files under `migrations/` and run them via a dedicated `cmd/migrate` binary or explicit one-shot Compose job **before** API and worker are started. Use one migration runner with an advisory lock and a migration history table; refuse incompatible/partially applied versions and fail startup if the required schema version is not present. Choose a Go migration library/tool during implementation based on Timescale extension compatibility and support for migrations that cannot run inside one transaction; document tool-specific transactional boundaries. Use **forward-fix** as the default recovery strategy for data-bearing migrations; reversible down migrations only when safe and tested.

Preserve the data semantics established by ADR-0007: unique `(device_id,time)`, accepted-row `INSERT ... RETURNING`, atomic writes of raw samples and both rollup resolutions, and retention of each tier. Keep retention policy installation/updates as versioned or explicitly idempotent database administration steps with the same owner. Schema DDL is no longer an implicit API/worker side effect. Document required privileges so runtime components can use reduced-privilege database credentials, while the migration job has DDL rights.

For compatibility changes use expand -> backfill -> switch readers/writers -> contract. Never drop a column needed by a still-running binary. Coordinate deployment and migration version requirements explicitly; health/readiness should report schema incompatibility without exposing SQL or secrets.

## Alternatives
- Continue startup migrations in both binaries: convenient for local Compose, but poor change ownership and race diagnostics.
- Manual SQL with no version tracking: difficult to reproduce and audit.
- ORM-generated schema sync: risky for hypertables, retention jobs and aggregate tables.

## Consequences
Schema history becomes reviewable and upgradeable; deployment has an additional explicit step. Backfills and large Timescale schema changes require capacity planning and potentially online operations. Migration tool choice is deliberately deferred until the first implementation PR.

## Migration and compatibility
1. Record current schema as a checked baseline; define how existing databases are marked migrated without replaying initial DDL.
2. Introduce migration runner, schema version check and migration job in Compose.
3. Move existing `Migrate()` DDL and retention installation into versioned migrations/admin steps without recreating data.
4. Stop invoking `Migrate()` from API and worker once automated upgrade tests pass.
5. Verify fresh database install, in-place upgrade, failed migration and rollback/forward-fix documentation.

## Acceptance criteria
- Fresh Compose deployment creates schema exactly once before API and worker readiness.
- Existing populated database upgrades without dropping raw/rollup data or breaking idempotency.
- API/worker can run without schema-modifying privileges.
- A failed/incompatible migration leaves a detectable error state and blocks unsafe startup.

## Related decisions
ADR-0001, ADR-0003, ADR-0007, ADR-0009, ADR-0014.
