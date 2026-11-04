# ADR-0010: Load validated YAML configuration with environment overrides
Status: Proposed (2026-11-04).

## Context
Startup settings are spread across `cmd/api/main.go`, `cmd/worker/main.go`, `cmd/simulator/main.go`, `internal/queue/queue.go` and `compose.yaml`. Constants include broker stream/subject, batch limits, retry settings, timeout values and retention policy parameters. This makes behavior hard to audit and tune between development and production.

## Decision
Use an explicitly typed configuration model in `internal/platform/config` and version-controlled non-secret YAML under `configs/`. Load at startup, validate strictly and fail before accepting work on malformed/inconsistent settings. Precedence (lowest to highest): compiled safe defaults -> `configs/default.yaml` -> optional environment-specific YAML -> documented environment-variable overrides -> documented CLI overrides. Configuration file location is specified with `--config` or `SM_CONFIG`; default path is documented for each deployment. No runtime hot reload in v1.

Example **proposed keys** (not a claim that the current loader supports them):

```yaml
server:
  address: ":8080"
  read_timeout: 15s
  write_timeout: 20s
message_broker:
  driver: nats
  stream: SERVER_METRICS
  subject: metrics.samples
  consumer: db-writers
  max_age: 48h
  max_bytes: 2147483648
ingestion:
  max_batch_size: 1000
  max_payload_bytes: 524288
  publish_timeout: 5s
worker:
  batch_size: 8
  processing_timeout: 90s
  ack_wait: 120s
  max_deliver: 8
storage:
  raw_retention: 168h
  minute_retention: 1080h
  hourly_retention: 9600h
query:
  max_points: 1500
  max_range: 8760h
logging:
  level: info
  format: json
```

Credentials (notably `DATABASE_URL` and `NATS_URL`, or more granular credential sources) come from environment variables / secret mounts and **must never be checked into YAML or logged**. Explicit overrides must be documented rather than inferred ambiguously from nested keys. Existing `DATABASE_URL`, `NATS_URL`, `API_URL`, `DEVICE_COUNT`, `HISTORY_DAYS`, `SEED` remain supported during transition. Use a YAML decoder with unknown-key rejection; parse durations as `time.Duration` via an explicit unmarshal path; validate positivity, driver support and dependencies (e.g. processing timeout < ack wait with an appropriate margin). YAML controls *operational settings*, not SQL identifiers, arbitrary domain schemas or secrets.

Preserve specification-enforced invariants: batch maximum 1,000, body limit 512 KiB, seven 0..100 sensors, 1,500-point query ceiling, max query range 365 days, and retention required for supported chart horizons. Changes to those limits require a separate spec/API decision. When using Compose, mount/read config files reliably in the container and pass secrets by environment or secret store.

## Alternatives
- Environment variables only: simple but difficult to organize and document numerous settings.
- Viper or remote config service: potentially useful later, but not needed for a small statically configured deployment.
- Hot reload: introduces partial-update and versioning hazards across components.

## Consequences
Configuration is consistent, reviewable and testable; there is a new parser and validation surface. Default config in development must not accidentally become production credentials or permissive production security settings. Print a sanitized effective configuration at startup, excluding secrets.

## Migration and compatibility
1. Enumerate current settings/defaults and capture existing startup behavior.
2. Add typed structs, a deterministic loader and table-driven tests for precedence, missing files, invalid keys, bad durations and secret redaction.
3. Introduce default/development/example files; provide production override guidance without secrets.
4. Switch one binary at a time, retaining legacy env names.
5. Update `compose.yaml`, Dockerfile and README; verify startup from a clean Compose volume.

## Acceptance criteria
- API, worker and simulator start with the documented defaults and overrides.
- Invalid configuration fails fast with actionable, non-secret errors.
- No database password or broker credential appears in tracked YAML or startup logs.
- API limits and broker durability semantics remain compatible with `docs/feature-spec-v2.md`.

## Related decisions
ADR-0002, ADR-0004, ADR-0007, ADR-0014.
