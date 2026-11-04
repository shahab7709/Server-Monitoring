# ADR-0012: Make worker delivery, retry and dead-letter behavior explicit
Status: Proposed (2026-11-04).

## Context
The current worker pulls from JetStream (`db-writers`), attempts a transactional database insert, ACKs with `AckSync()` on success and otherwise `NakWithDelay(5s)`. It specifies `MaxDeliver(8)` but does not implement a durable dead-letter handoff or classify permanent versus transient failures. A malformed message can be retried repeatedly; exhausted delivery attempts need an observable recovery procedure. ADR-0002 requires at-least-once delivery, and ADR-0007 requires atomic commits and idempotent replay.

## Decision
Retain JetStream pull consumption and at-least-once delivery. Structure the worker as an inbound consumer adapter plus an application batch processor. Classify outcomes:

- **Success / already processed:** transaction has committed, then ACK synchronously. ACK failure permits redelivery; the database primary key and rollup logic prevent double counting.
- **Transient error:** unavailable database, timeouts, connection failures, concurrency pressure. NAK with bounded, configured backoff; do not ACK as if successful.
- **Permanent error:** invalid or unprocessable schema/message that repeated delivery cannot repair. Route to a dedicated durable dead-letter subject/stream with original payload (or protected reference), stable message identity, error class, timestamp and delivery metadata. ACK the source **only after the DLQ publish is durably acknowledged**. If DLQ publishing fails, leave the source unacknowledged for redelivery/escalation.

Configure delivery attempt limits, timeout, ack wait and backoff consistently; `ack_wait` must exceed normal processing time or the worker must extend acknowledgment deadlines (`InProgress`) during long processing. Align actual stream/consumer creation with these settings; verify existing consumers are reconciled safely when configuration changes. Track JetStream max-delivery advisories and handle exhausted messages explicitly; do not assume `MaxDeliver` automatically creates a DLQ or ensures republishing. Alert on exhausted delivery and unhandled poison messages. Keep DLQ replay as a separate guarded operational action, never an automatic infinite loop.

The DLQ handoff is **at-least-once**, not a cross-stream atomic transaction. A crash after DLQ publish but before source ACK can duplicate DLQ entries; include a deterministic DLQ id/deduplication strategy and idempotent inspection/replay tools. Set retention/access controls for sensitive payloads. Do not change the public HTTP acceptance contract: `202` remains contingent on the original stream's durable publish ack. A single-host one-replica stream still has the limits stated in ADR-0002.

## Alternatives
- Fixed-delay retry forever: simple but allows poison-message loops and unknown queue health.
- ACK and log permanent errors: loses recoverability.
- Rely on max-deliver alone: not a complete dead-letter workflow.
- Exactly-once processing promise: not justified across broker/DB or two streams.

## Consequences
Improves fault isolation and operational observability; introduces DLQ retention, security, replay and failure-order complexity. The reliable database transaction remains the source of ingestion idempotency; queue deduplication windows are supplementary.

## Migration and compatibility
1. Add worker processing/error-classification tests and capture the current behavior.
2. Introduce retry/backoff configuration and relevant delivery metrics.
3. Provision DLQ stream/subject and max-deliver advisory handling with documented ownership.
4. Implement durable DLQ-first handoff; add duplicate-safe replay procedure.
5. Run fault-injection tests for DB outage, broker outage, malformed data, consumer restart and ACK loss.

## Acceptance criteria
- No message is ACKed before successful DB commit or durable DLQ acceptance.
- A retry after commit does not increment rollups twice.
- DLQ failure cannot silently discard data; max-delivery exhaustion is visible and actionable.
- Shutdown releases inflight work safely; retry/backoff parameters do not create immediate redelivery storms.
- Tests cover both handoff crash orderings and replay idempotency.

## Related decisions
ADR-0002, ADR-0007, ADR-0010, ADR-0011.
