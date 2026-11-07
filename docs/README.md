# Documentation and architectural decisions

- [سند کوتاه تحویل طراحی و عملیات](design-handoff-fa.md): تصمیم‌ها، بودجهٔ سالانه، رشد چهار نفره، مهاجرت و SLO/هشدارها.
- [گزارش تست بار و گراف‌ها](load-test-report-fa-2026-11-04.md): نتایج موجود و محدودیت ادعای ظرفیت.
- [Architecture background](architecture.md): original architecture scope; implemented rollup/UI details are governed by ADR-0007/0008 and the handoff above.

Accepted implementation decisions: [storage](adr/0001-timescaledb.md), [queue](adr/0002-jetstream.md), [retention/query tiers](adr/0003-rollups.md), [deployment](adr/0004-modular-monolith.md), [performance target](adr/0005-performance.md), [transactional rollups](adr/0007-transactional-rollups.md), [offline Canvas](adr/0008-offline-canvas.md). ADR-0007/0008 supersede the earlier continuous-aggregate/chart-library implementation details in ADR-0001/0003/0006.

## Proposed architecture ADRs (0009–0014)

These files extend the existing `docs/adr/0001`–`0008` decisions. All six are **Proposed** pending team approval and implementation. They are designed to preserve the current `docs/feature-spec-v2.md` contracts.

| ADR | Topic | Suggested first PR |
|---|---|---|
| 0009 | Hexagonal modular monolith | Domain extraction / thin entry points |
| 0010 | Typed YAML configuration | Config loader + tests |
| 0011 | Application ports and infrastructure adapters | Ingestion service + NATS publisher adapter |
| 0012 | Worker retry and dead-letter | Worker processing classification and failure tests |
| 0013 | Versioned migrations | Baseline schema + migration job |
| 0014 | Sensor catalog and feature evolution | Central catalog and compatibility tests |

## Proposed delivery and operating decisions (0015–0018)

| ADR | Topic | Status |
|---|---|---|
| [0015](adr/0015-capacity-and-cost.md) | Retention sizing and annual cost assumptions | Proposed planning baseline |
| [0016](adr/0016-growth-plan.md) | Four-person, twelve-week path to 10k | Proposed; benchmark gated |
| [0017](adr/0017-online-storage-migration.md) | Snapshot/CDC, fenced cutover and current rollback projection | Proposed; tooling not implemented |
| [0018](adr/0018-service-objectives-and-alerting.md) | Production SLOs, recovery and paging policy | Proposed targets |


