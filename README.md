# Server Monitoring Platform — Go / TimescaleDB / NATS JetStream

An implementation of the architecture package delivered with the challenge. All seven sensors are sampled per device each second; historical samples are backfilled by the simulator and raw/minute/hour data is persisted transactionally.

## Run (single command)

Requires Docker Engine and Docker Compose v2:

```bash
docker compose up --build
```

Open **http://localhost:8080**. It starts TimescaleDB, durable NATS JetStream, an API/UI, a database worker, and a simulator. Persistent volumes preserve database and queue state between restarts. Initial backfill runs in the background and can take time; devices appear after their first successful worker commit. UI updates without an external CDN.

Settings (environment variables):

```bash
DEVICE_COUNT=5 HISTORY_DAYS=3 SEED=42 docker compose up --build
```

A full one-year demonstration requires `HISTORY_DAYS=365`; this generates **31,536,000 samples per device** and can require significant disk and ingestion time. The default is two devices and three days to make evaluation tractable. At higher device counts, ingestion and storage must be sized and benchmarked.

## Verification

Unit tests (Go installed):

```bash
go test ./...
```

Smoke tests against running stack:

```bash
docker compose --profile test run --build --rm smoke
```

The smoke command checks HTTP readiness, the HTML dashboard, validation, publish acknowledgment, duplicate/idempotent storage, discovery, raw query, and bounded one-year query. It polls with a timeout rather than sleeping a fixed delay. Historical aggregation correctness and browser performance require further integration/E2E tests before production release.

Load tests use k6 from a container and reside in [`loadtest/`](loadtest/README.md). k6 HTTP p95 is **not** proof of browser end-to-end p95. No measured SLO claim is made here.

## HTTP endpoints

`POST /api/v1/metrics/batch` — sends `{ "samples": [{"device_id":"server-00000","time":"2026-11-02T12:00:00Z","cpu":10,"memory":20,"disk_space":30,"disk_busy":40,"net_in":50,"net_out":60,"fan":70}] }` and returns `202` only after a JetStream durable acknowledgment. Max 1000 samples / 512 KiB. Sends require all seven values between 0 and 100.

`GET /api/v1/devices?search=server&limit=25&cursor=...` — paginated device lookup by ID/name.

`GET /api/v1/metrics/query?device_id=server-00000&metrics=cpu,memory&from=2026-11-02T11:00:00Z&to=2026-11-02T12:00:00Z&max_points=1500` — selected sensor stats (`avg,min,max,count`) grouped and returned in chronological order; absent buckets have `metrics:null`. Max time range 365 days, max 1500 points, UTC `[from,to)`. `GET /health/live` and `/health/ready` for liveness and DB/queue readiness.

## Data correctness & performance

1. Validated batch is durably published, then acknowledged to sender (HTTP 202); producer retries are safe.
2. JetStream pull consumer receives batches and writes **only newly inserted samples** (`ON CONFLICT DO NOTHING RETURNING`) into minute and hour rollups, all in one SQL transaction. Acknowledgment happens after commit. Duplicate ingestion or worker replay does not double-count.
3. Raw 7d, minute 45d, hour 400d TimescaleDB retention policies; independently persisted rollups survive raw deletion. Backfilled history updates aggregates even if raw is older than the nominal raw retention window; such raw data may be removed by retention.
4. Short windows use raw; medium windows minute aggregates; >45d hour aggregates. SQL combines count-weighted sums and min/max into at most 1500 display buckets. A year of data is read from hourly rollups, **not** 31.5M raw rows per server.
5. The single-file HTML contains its own chart renderer, allows device search, sensor multi-select, preset/custom periods, shared 0..100 scale, explicit gaps, loading/errors and cancellation of stale requests.

## Architecture and team handoff

Architecture, capacity planning, assumptions and ADRs are in [`docs/`](docs/architecture.md). Implementation-specific ADRs for transactional rollup updates and offline Canvas charting override the original provisional aggregate/frontend implementation choices. Split ownership by API/contracts, worker/ingest, DB/retention, and UI/test/performance; change contracts and add ADRs before cross-cutting migrations.

**Limitations:** single-host Compose is not HA; durable means persisted locally to JetStream volume, not replicated cross-host. The worker does not yet provide a separately configured dead-letter stream; after max delivery, failures need operational intervention. No authentication/TLS, migration version framework, production dashboards or cross-zone backups. Under this environment, Docker was unavailable and external Go dependencies could not be downloaded, so full compile/integration/performance verification remains pending. Do not treat this as a verified production rollout.
