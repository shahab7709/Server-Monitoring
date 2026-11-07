# ADR-0018: Use user-visible SLOs and actionable alerts

Status: Proposed production objectives (2026-11-04). Current single-host deployment and one 2k measurement do not establish monthly availability or replicated durability.

## Objectives and measurement

| SLI | Proposed target and scope |
|---|---|
| Ingest/query availability | 99.9% over rolling 30 days; valid supported requests succeed within their deadline. Count overload 429/5xx as bad; exclude malformed/auth-invalid client requests. Report ingest and query separately. Error budget: 43.2 minutes per 30 days. |
| Annual chart | Browser end-to-end p95 <1s for one device, seven sensors, actual year-long hourly history, ten concurrent users; API p95 <500ms separately. Report hardware and client workload. |
| Ingest latency | p95 <250ms to durable publish ACK for batches up to 400 at admitted load. HTTP 202 is not a DB commit. Larger supported batches need their own latency baseline. |
| Freshness | Live sample generation to chart visibility p95 <10s, p99 <30s; exclude historical backfill from this live SLI and track backfill separately. Require sane clock synchronization. |
| Data protection | Every 202 must reach committed storage or a durable observable replay path/DLQ; zero silent acknowledged loss. A 99% offered-throughput capacity threshold does not authorize 1% loss. |
| Recovery | Target RPO=0 for one-host failure only after replicated broker/DB durability is configured and exercised; disaster restore target RPO<=15min, RTO<=1h using verified PITR/WAL backups. |

Replication requires independent failure domains, appropriate DB synchronous commit and explicit broker persistence/fsync/ACK semantics. Three replicas alone do not prove durability under every power-loss case. Test the stated fault model; current NATS one replica and Compose volumes cannot deliver these protection goals. Forecasts and brief benchmark passes are not availability evidence.

## Page the on-call

- Availability burn rate >14.4x on both 5min and 1h windows, or >6x on both 30min and 6h windows. A complete external serving outage has a separate immediate check after two minutes.
- No successful DB commits for >2min while valid input is being accepted; or oldest accepted-but-uncommitted enqueue age >60s for 5min with a growing backlog. Include consumer identity and restart/replay runbook.
- Queue or filesystem exhaustion projected within 30min at observed growth; actual publish rejection, unsafe disk pressure or persistent inability to accept valid traffic. Monitor retained stream bytes as well as Pending: ACKed messages currently consume capacity.
- Confirmed missing/corrupt acknowledged data, or an exhausted MaxDeliver message without a durable recovery owner/DLQ. Isolated recoverable DLQ entries need a ticket; blocked live traffic or unprotected accepted data needs a page.
- Loss of required durability protection or broken WAL/archive chain beyond the recovery objective; failover unable to restore service. Correlate downstream symptoms to one incident.

## Ticket or dashboard during business hours

High CPU with healthy user SLOs; individual slow queries; short backlog bursts; successful retries; cost/retention trends; one failed backup while an independently verified restore chain remains within RPO. Event timestamps from historical backfill are not queue-wait timestamps: enqueue-to-processing age drives backlog alerts. Do not page for absence of traffic when no traffic is expected.

Every page needs a named service owner and backup, severity, dashboard, runbook, safe mitigation and resolution condition. Group dependent alerts, use hysteresis, exercise maintenance suppression, and review false pages weekly. The reliability owner implements monitors; ingestion/data/query owners own their runbooks. Required first runbooks cover overload, disk/queue full, DB outage, poison/max-deliver, failover and restore. Customer sensor-threshold alerting is a separate product feature and is out of this phase's scope.

Related: [ADR-0005](0005-performance.md), [ADR-0012](0012-worker-delivery-retry-and-dead-letter.md), [ADR-0015](0015-capacity-and-cost.md), [ADR-0016](0016-growth-plan.md).
