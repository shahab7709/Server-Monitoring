# ADR-0002: Durable at-least-once ingestion using NATS JetStream
Status: Accepted for initial implementation.

## Context
Data must survive worker/DB outages after being acknowledged. Direct write couples API to DB availability.
## Decision
Go HTTP ingress publishes to file-backed JetStream; publish acknowledgment precedes HTTP 202. Worker ACKs only after successful DB commit. Device retries stable samples; DB PK gives idempotency.
## Alternatives
Direct DB writes + local disk WAL (fewer services, more custom reliability), Kafka (more scale/operational complexity).
## Consequences
Queue requires sizing, monitoring, durability settings and backpressure. Single-host Compose cannot withstand total-host loss.
## Revisit
If queue overhead dominates at benchmark scale or stronger cross-host durability is required.
