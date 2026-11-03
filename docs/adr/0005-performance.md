# ADR-0005: Define the one-year chart performance SLO
Status: Accepted as a measurable target, not validated.

## Context
"Load under one second" is ambiguous without an endpoint, hardware and concurrency.
## Decision
Report end-to-end p95 for 1-year, 7-sensor chart with 1,000 baseline devices, concurrent user count and hardware documented. Additionally measure p50/p95/p99 DB, API and browser separately. Enforce a bounded maximum number of chart buckets and avoid historical raw scans.
## Alternatives
Only SQL latency (not user-facing), mean latency alone (hides long tail).
## Consequences
Requires representative historical backfill and browser performance automation.
## Revisit
If client device mix or expected concurrency changes.
