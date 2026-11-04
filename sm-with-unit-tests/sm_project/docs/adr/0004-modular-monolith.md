# ADR-0004: Go modular monolith API, separate worker
Status: Accepted.

## Context
Take-home must boot via Compose and remain maintainable for a growing team.
## Decision
Use standard Go HTTP API with clear ingestion/query/service/repository interfaces, separately launched worker and simulator; a single HTML file embedded and served by Go.
## Alternatives
Many microservices (deployment and debugging burden), single process for all including worker (less isolation).
## Consequences
Cheap operational footprint, independently testable modules, scalable worker replicas where needed.
## Revisit
When measured resource isolation or organizational boundaries justify service extraction.
