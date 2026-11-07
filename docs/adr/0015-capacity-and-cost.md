# ADR-0015: Size retention windows and expose an explicit annual cost model

Status: Proposed planning baseline (2026-11-04); prices and row sizes are assumptions, not measured capacity or a vendor quote.

## Decision and rationale

Budget for one seven-sensor row/device/second. Keep the implemented 7-day raw, 45-day minute and 400-day hourly windows. One-year visualization does not require one-year raw retention. Preserve min/max/sum/count; do not price a row for each sensor. Use decimal GB/TB for costs, distinct from broker GiB limits.

At 10,000 devices:

| Quantity | Rows | Assumed bytes/row including indexes | Storage |
|---|---:|---:|---:|
| Annual raw arrivals | 315,360,000,000 | 200 | 63.072 TB if all retained |
| Raw retained 7 days | 6,048,000,000 | 200 | 1,209.60 GB |
| Minute retained 45 days | 648,000,000 | 320 | 207.36 GB |
| Hour retained 400 days | 96,000,000 | 320 | 30.72 GB |
| Mature primary windows | — | — | 1,447.68 GB |

Row sizes are conservative working assumptions for the wide schema and text IDs. No compression savings are credited: compression is not configured in the implementation. The 400-day mature window modestly exceeds the first year's actual hourly occupancy. Measure relation sizes including Timescale chunks/indexes and WAL before procurement; update this ADR if assumptions change.

For illustration, provision two DB copies with 25% additional capacity relative to stored data: `1,447.68 × 2 × 1.25 = 3,619.20 GB`. At the observed approximately 175 broker bytes/sample, six hours of 10k/s outage needs approximately 37.8 GB per broker replica; round to 50 GB per node, three nodes. This requires removal of consumed messages (for example WorkQueuePolicy after confirming consumer/replay semantics). Current 2 GiB LimitsPolicy retains ACKed messages and is unsuitable: projected fill time from empty at 10k/s is approximately 20.5 minutes, not a measured 10k result. Six-hour queue capacity does not provide 48-hour migration replay; provision that journal separately.

Assumed SSD price $0.10/GB-month and one logical backup copy at $0.023/GB-month give:

`annual storage = (3,619.20 + 150) × 0.10 × 12 + 1,447.68 × 0.023 × 12 = $4,922.60`.

Example compute: two DB VMs at $250/month, three NATS VMs at $30/month and two API/worker VMs at $50/month: $8,280/year. Infrastructure subtotal is approximately $13,203/year. Four maintenance hours/week at $40/hour add $8,320/year, totaling approximately $21,523/year. VM rates exclude disks already counted above. Rates are illustrative and may not buy the IOPS/CPU needed for 10k; vendor sizing must pass benchmark gates. These are full-year fixed-capacity budgets, not first-year usage bills. Extra PITR/WAL versions, backup generations, monitoring, IOPS, network, taxes and migration overlap are additional.

## Alternatives and consequences

Retain all annual raw: supports second-level historical reconstruction, but needs 63 TB of raw alone before replicas/backups. Compress or move old raw to object storage: potentially cheaper, but adds lifecycle, replay and restore costs; requires another decision and measurements. The chosen window loses second-level history after seven days and preserves annual aggregates. Revisit for contractual raw archival, variable sensors, changed retention, measured byte sizes or provider prices.

Related: [handoff](../design-handoff-fa.md), [ADR-0001](0001-timescaledb.md), [ADR-0007](0007-transactional-rollups.md), [measured results](../load-test-report-fa-2026-11-04.md).
