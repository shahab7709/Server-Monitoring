# ADR-0003: Multi-resolution rollups for long-range charts
Status: Accepted.

## Context
One sensor sampled every second produces ~31.5 million points in a year; browsers need ~1k–2k visible buckets.
## Decision
Raw 7d, one-minute 45d, one-hour 400d. Store min/max/sum/count for all seven sensors; aggregate again into ~max_points aligned buckets per query. Keep gaps explicit and recent raw stitched to completed materializations.
## Alternatives
Scan raw on demand (slow); simple point-sampling (hides spikes); only averages (loses maxima); indefinite raw retention (storage cost).
## Consequences
Cannot reconstruct original second-level history beyond raw retention. Refresh lag and late data handling require testing.
## Revisit
When spike fidelity requirements or customer access patterns change.
