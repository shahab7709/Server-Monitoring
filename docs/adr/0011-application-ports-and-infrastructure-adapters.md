# ADR-0011: Define use-case-owned ports with NATS and TimescaleDB adapters
Status: Proposed (2026-11-04).

## Context
The API directly calls `queue.Publish` and `storage.Store` methods; the worker calls `storage.InsertBatch`. The API and worker import concrete technology packages, making service-level unit tests depend on infrastructure and making alternative integrations invasive.

## Decision
Define minimal interfaces next to the **module that needs them**. The initial ports are:

- Ingestion publishing: `PublishBatch(ctx, validatedBatch)` with a documented contract: success means durable JetStream publication acknowledged, not just a local buffer write.
- Query: `QueryMetrics(ctx, request) (result, error)` using domain query/result DTOs and existing bucketing semantics.
- Devices: `ListDevices(ctx, filter) (page, error)` preserving cursor and limit behavior.
- Worker persistence: `PersistBatch(ctx, validatedBatch) error` where success means the raw and minute/hour rollups committed atomically; duplicates are no-ops.

A concrete adapter can implement more than one interface, but modules do not depend on a catch-all `Store`. Place NATS publisher/consumer and TimescaleDB repositories under `internal/adapters`, retaining the same production drivers at first. Keep transaction ownership inside the TimescaleDB write adapter (one transaction for raw insert and both aggregate upserts per ADR-0007). Treat the NATS consumer as an **inbound adapter** and its publisher as an **outbound adapter**. Broker ACK/NAK is transport behavior, orchestrated by the worker adapter using processing outcomes (see ADR-0012).

Use explicit application errors (`invalid input`, `not found`, `unavailable`, `internal`) for mapping to HTTP or retry policies. Wrap infrastructure errors without leaking connection strings or SQL to clients. Fakes/in-memory adapters are test-only unless their durability limitations are explicitly accepted. Implement adapters for other brokers/databases only in response to a demonstrated use case; any candidate must meet the port's semantic requirements, not just compile.

## Alternatives
- One broad `Storage` interface: simple initially, but makes every consumer depend on unrelated capabilities.
- Interfaces declared centrally in a shared `ports` package: encourages accidental coupling and huge contracts.
- Generic repository/ORM abstraction: cannot represent required Timescale-specific query and transactional rollup behavior cleanly.

## Consequences
Makes modules independently testable and helps introduce new integrations. Mapping across adapter boundaries adds some code. Durable delivery and transactional consistency intentionally remain technology-specific guarantees, documented and exercised with integration tests.

## Migration and compatibility
1. Add interfaces in modules, with contract tests for acknowledgment and persistence semantics.
2. Wrap existing NATS publisher, avoiding changes to JetStream stream/subject or HTTP `202` response.
3. Wrap existing DB query/device operations with adapter DTO conversion, keeping raw/minute/hour selection unchanged.
4. Extract batch persistence as one atomic adapter method; retain idempotent first-write-wins semantics.
5. Inject interfaces via bootstrap; remove direct driver imports from application services.

## Acceptance criteria
- Services have unit tests using fakes for success and failure paths.
- Integration tests verify ACK after commit, replays/deduplication, late-arrival rollups, query gaps and selected-sensor ordering.
- No adapter implementation is claimed interchangeable without meeting documented invariants.
- Existing public API and deployment topology are unchanged.

## Related decisions
ADR-0001, ADR-0002, ADR-0003, ADR-0007, ADR-0009, ADR-0012.
