# Unit test implementation and verification — feature-spec-v2

Date: 2026-11-04

## Added tests

| File | Coverage linked to specification |
| --- | --- |
| `internal/model/model_test.go` | F01 TC01–TC03 (unit validation and JSON decoding checks only; **not** HTTP/storage assertions), F05 sensor-list contract, F01 batch limits and error messages. |
| `internal/storage/storage_test.go` | F03 TC10, TC11 (pure rollup arithmetic), TC12 (empty-aggregate helper), SQL generation checks for both rollup resolutions, seven min/max/sum columns, and conflict merging. These do **not** replace database integration tests. |
| `cmd/simulator/main_test.go` | F08 TC33 (fixed timestamp/seed reproducibility), validity/range/spike checks supporting AC22, configuration parsing. Does **not** prove TC30/TC31 backfill/live behavior end-to-end. |
| `cmd/api/main_test.go` | Structured error response and request logging middleware behavior. Does **not** replace HTTP endpoint tests with running dependencies. |

A small implementation change in `internal/storage/storage.go` routes weighted average construction through `statsFromAggregate`, which returns `nil` for an empty aggregate instead of dividing by zero.

## Execution

- `GOPROXY=off go test ./internal/model` — **PASS**.
- `GO111MODULE=off GOPATH=<temporary GOPATH pointing at repo> go test ./cmd/simulator` — **PASS**.
- `go vet ./internal/model` and `GO111MODULE=off GOPATH=<temporary GOPATH pointing at repo> go vet ./cmd/simulator` — **PASS**.
- `gofmt -l` on modified Go files — **clean**.
- `go test ./...` — **BLOCKED**, not a test failure: required modules `github.com/nats-io/nats.go` and `github.com/jackc/pgx/v5` are absent from the local Go module cache and outbound DNS/network access fails. Therefore `internal/storage` and `cmd/api` unit tests have been written but **not executed** in this environment.
- Docker/Compose integration, UI E2E, performance and recovery tests — **NOT RUN**; runtime infrastructure is unavailable here.

## Commands to run in a network-enabled developer/CI environment

```sh
go mod download
go test -count=1 ./...
go vet ./...
test -z "$(gofmt -l ./internal/model ./internal/storage ./cmd/api ./cmd/simulator)"
# With Docker Compose available:
docker compose up --build -d
docker compose --profile test run --build --rm smoke
```

## Remaining coverage required before Definition of Done

The spec enumerates 52 test cases across unit, integration, API, UI, E2E, recovery, smoke, load, stress and review. Only the unit-level subset above has been added. Database correctness for deduplication, retention, late arrivals, bucket boundaries and gap behavior must still be verified against TimescaleDB; API behavior against the JetStream/worker stack; browser race/state behavior in browser automation; and load/soak and recovery SLOs in a documented reference environment. The project should **not** be marked as fully compliant with the spec based on these unit tests.
