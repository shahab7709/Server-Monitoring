# ADR-0009: Organize the modular monolith using ports and adapters
Status: Proposed (2026-11-04).

## Context
ADR-0004 selects a Go modular monolith with an independently runnable worker. Today `cmd/api/main.go` combines HTTP routing, validation, NATS publication and storage access; `cmd/worker/main.go` combines consuming, processing and ACK/NAK decisions. The existing `internal/model`, `internal/queue` and `internal/storage` packages are useful starting points but the dependency direction is not enforced. Feature changes can require edits to executable entry points and infrastructure-heavy tests.

## Decision
Keep the **same deployment topology**: API, worker and simulator binaries; NATS JetStream; TimescaleDB; embedded offline HTML UI. Organize code around domain and use cases, with infrastructure adapters on the outside:

- `internal/domain/{metric,device}`: entities, value objects and invariant checks; no HTTP, NATS, SQL, YAML or environment imports.
- `internal/modules/{ingestion,query,devices,rollup}`: business use cases and *small consumer-owned interfaces* (ports). Interfaces are introduced only for real boundaries, not for each type.
- `internal/adapters/inbound/{http,nats}`: HTTP handlers and message delivery mechanics.
- `internal/adapters/outbound/{nats,timescaledb}`: publish and persist implementations.
- `internal/bootstrap`: constructs concrete adapters, injects them into services, manages shutdown/lifecycle.
- `cmd/{api,worker,simulator}`: thin executable entry points.

Dependencies point inward: adapters -> modules -> domain. The bootstrap package is the sole wiring layer allowed to import all sides. An inbound NATS consumer translates deliveries into an application call and owns transport-specific ACK operations; application services do not import `nats.go`. Application-facing interfaces use domain-level types and context, not pgx types. Keep the existing HTTP response schemas and endpoint names unchanged during migration.

## Alternatives
- Retain package-by-technology layering: less work initially, but business cases remain coupled to storage and transport.
- Split into microservices now: more infrastructure and transactional complexity without demonstrated need.
- Build a generic runtime plugin framework: unnecessary for statically linked Go deployments.

## Consequences
Benefits: independent unit tests, explicit boundaries, smaller feature changes and swappable integrations. Costs: more packages and adapter mapping. Avoid circular imports and interface proliferation. Adapter substitution is conditional on satisfying the required semantics (especially durable publish and transactional writes), not merely on matching function signatures.

## Migration and compatibility
1. Characterize existing API responses, batch validation, queries, rollups and worker retry behavior with tests.
2. Extract pure domain code without altering JSON contracts.
3. Extract one ingestion service and its publisher port; wire the existing NATS implementation.
4. Move query/device workflows and persistence calls behind narrow ports.
5. Extract inbound HTTP and worker adapters. Keep Docker Compose topology and UI path unchanged.
6. Remove old `internal/queue` / `internal/storage` entry points only after all callers move.

## Acceptance criteria
- `go test ./...` passes (with dependencies available); use case unit tests run without NATS or PostgreSQL.
- HTTP regression and Compose smoke/integration tests pass unchanged.
- No module/domain package imports HTTP, pgx or NATS drivers.
- The worker still ACKs only after a committed write; `202` still requires a JetStream publish acknowledgment.

## Related decisions
ADR-0002 (JetStream), ADR-0004 (modular monolith), ADR-0007 (transactional rollups), ADR-0011 (ports and adapters), ADR-0012 (reliability).
