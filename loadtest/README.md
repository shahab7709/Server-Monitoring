# Load tests

```bash
docker run --rm --network server-monitoring_default -v "$PWD/loadtest:/scripts:ro" -e API_URL=http://api:8080 -e VUS=10 -e DURATION=1m grafana/k6 run /scripts/ingest.js
docker run --rm --network server-monitoring_default -v "$PWD/loadtest:/scripts:ro" -e API_URL=http://api:8080 -e VUS=10 -e DURATION=1m grafana/k6 run /scripts/query.js
```

Configure `VUS`, `DURATION` and `QUERY_DEVICE`. These tests measure HTTP latency and errors, not browser end-to-end latency. A meaningful 365-day SLO test requires 365 days of populated hourly aggregates and a separately measured browser render p95. No measurements are claimed.

The isolated capacity runner uses existing local API/worker images and creates
separate volumes. It binds the API to `127.0.0.1:28081`. The existing deployment
is left running; record its background resource use when interpreting results.

```bash
docker compose -f loadtest/compose.benchmark.yaml up -d
CGO_ENABLED=0 go build -o /tmp/server-monitoring-simulator ./cmd/simulator
python3 loadtest/capacity_benchmark.py
# In a virtual environment containing matplotlib:
python3 loadtest/plot_capacity.py --input loadtest/results/<run-directory>
docker compose -f loadtest/compose.benchmark.yaml stop
```

The default schedule uses five minutes below 1,000 devices and 1,030 seconds
at higher steps, with 120 seconds of warm-up. The extra ten seconds at higher
steps allow at least 900 seconds between sampled measurement endpoints.
`--seconds` and `--warmup` support exploratory runs. Each run directory must be
new. Nonzero exit status means a failed or inconclusive result, not successful
capacity verification. Inspect `summary.json` and per-step evidence.

The runner stops on resource budgets, essential metric collection failures,
or the first failing capacity step. Boundary repeats, a soak run, and a separate
bounded 10k stress run must be selected after reviewing those results; they are
not inferred from a short baseline. Never delete the benchmark volumes to reset
state without explicitly approving deletion. Run-specific device prefixes avoid
deduplication between runs; queue drain is checked before and after each step.

Historical preparation publishes 8,761 actual seven-sensor hourly samples for
one device through the API and worker, and verifies 365-day rollup coverage.
This is a sparse test dataset, not a full year at 1 Hz. Browser tests use ten
Chrome pages, all seven sensors, and the existing renderer; latency ends after
two animation frames. Set `PLAYWRIGHT_MODULE` and `CHROME` for your installation.
The defaults locate Playwright inside VS Code and a system Google Chrome.
Ten additional HTTP query workers rotate 1h, 7d and 365d requests at one request
per second per worker. Warm-up requests are excluded from percentiles.

The simulator accepts `DEVICE_PREFIX` and `MAX_PUBLISH_ATTEMPTS` (default 5).
It reports cumulative generated/accepted/failed samples, failed attempts, and
tick lag in `BENCH` log records. Exhausted publications are recorded as failed;
they must never be counted as persisted samples. The sequential publisher can
become the limiting component. Compare measured accepted rate with offered
device count before attributing a failure to the worker or database.
