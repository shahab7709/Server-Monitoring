# ADR-0014: Introduce a sensor catalog and explicit feature/schema evolution
Status: Proposed (2026-11-04).

## Context
The current model declares exactly seven sensors (`cpu`, `memory`, `disk_space`, `disk_busy`, `net_in`, `net_out`, `fan`). They are required 0..100 values in the JSON payload and raw TimescaleDB table; minute/hour rollup schemas, SQL column generation, query selection, simulator and UI rely on those names. The product may later need alerts, derived metrics or additional sensor types. Treating the current columns as arbitrary user-defined YAML entries would break database and API guarantees.

## Decision
Create a versioned, compile-time **sensor catalog** in the domain with canonical name, unit, allowed range, human-readable label and supported query/aggregation behavior. Make ingestion validation, sensor selection, simulator metadata and UI labels consult it where appropriate, but retain the current seven fixed fields and persisted columns in v1. Preserve backward-compatible JSON and the 0..100 range and chart axis required by `docs/feature-spec-v2.md`. YAML may configure **operational presentation** (for example, enabled views or default selections) but cannot define arbitrary SQL identifiers or change accepted sensor schemas at runtime.

New application capabilities (such as alerting) are added as statically linked modules under `internal/modules/<feature>`, each exposing use cases and owning minimal ports. Integration with HTTP, messaging and notifications happens through adapters and is composed explicitly in bootstrap; no Go runtime plugin system, dynamic code loading or universal module registry. An alerts module, if approved later, can consume validated/committed events or issue queries, but its delivery and ordering guarantees must be specified separately; ingestion success must not accidentally become contingent on external alert delivery.

Adding an eighth persisted metric requires a separate design/change proposal: versioned wire schema, compatibility with old producers, migrations for raw and both rollup tables, backfill/NULL handling, query/window selection, retention, simulator, UI and performance tests. If truly arbitrary heterogeneous metrics become a requirement, evaluate a separate narrow/eav-style schema or dedicated measurements table with independent ADR and benchmark against the current wide schema.

## Alternatives
- Dynamic map of sensors persisted as JSONB: flexible but changes validation, indexing and rollup/query complexity.
- Generic EAV schema immediately: more extensible but sacrifices current optimized fixed-schema workload without evidence.
- Runtime plugin system: complexity, security and deployment issues for little current benefit.

## Consequences
Catalog reduces duplicated sensor metadata and supports safer feature changes while intentionally limiting sensor extensibility. Changing metadata is not the same as adding a stored metric; explicit migrations remain necessary. New modules remain part of the modular monolith and are unit tested independently.

## Migration and compatibility
1. Capture baseline sensor validation, SQL/rollup and query/UI tests for all seven sensors.
2. Introduce catalog with immutable order and `ParseSensorSet` compatibility.
3. Migrate callers incrementally; do not let config generate SQL identifier names.
4. Implement one small optional module using service + ports + adapter conventions to validate extensibility.
5. Require a new ADR/spec update before extending the stored sensor schema.

## Acceptance criteria
- Existing seven-sensor JSON requests, batch limits, query results and Canvas graph remain unchanged.
- SQL column mapping is generated only from a reviewed allowlist, never untrusted config/request text.
- New feature modules can be unit-tested without external infrastructure.
- Schema-expansion requirements are documented before new sensors enter production.

## Related decisions
ADR-0004, ADR-0007, ADR-0008, ADR-0009, ADR-0010, ADR-0013.
