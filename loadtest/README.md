# Load tests

```bash
docker run --rm --network server-monitoring_default -v "$PWD/loadtest:/scripts:ro" -e API_URL=http://api:8080 -e VUS=10 -e DURATION=1m grafana/k6 run /scripts/ingest.js
docker run --rm --network server-monitoring_default -v "$PWD/loadtest:/scripts:ro" -e API_URL=http://api:8080 -e VUS=10 -e DURATION=1m grafana/k6 run /scripts/query.js
```

Configure `VUS`, `DURATION` and `QUERY_DEVICE`. These tests measure HTTP latency and errors, not browser end-to-end latency. A meaningful 365-day SLO test requires 365 days of populated hourly aggregates and a separately measured browser render p95. No measurements are claimed.
