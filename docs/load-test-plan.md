# Load Testing and Capacity Benchmark Plan — Server Monitoring

**Status:** Ready to execute on your machine. **No benchmark results have been measured for this document.**  
**Scope:** API, NATS JetStream, Worker, TimescaleDB, Simulator, chart API, and Docker resource utilization.  
**Basis:** `docs/feature-spec-v2.md` (F07, F08, F12; TC28, TC46–TC49), `compose.yaml`, and the existing scripts under `loadtest/`.

## 1. Goals and Questions to Answer

1. What is the **maximum sustainable device count** on the tested hardware, assuming each device sends one sample per second containing seven sensors?
2. What are the rates of **HTTP acceptance** and **durable database persistence**? Does the queue backlog grow over time?
3. What is the chart response latency under concurrent load and with genuine historical data (p50/p95/p99)?
4. Which component limits capacity (load generator, API, NATS, Worker, database, disk, or query path), and what evidence demonstrates that limit?
5. At **10,000 devices**, which component fails or saturates first, and what changes should be prioritized?

**Capacity definition:** The largest device-count step that meets **every acceptance criterion** in Section 5 for at least 15 minutes at steady state. An HTTP `202` indicates acknowledgment of durable publication to the queue; it **does not** mean the data has been committed to the database. Do not label a short exploratory run as verified sustainable capacity.

## 2. Test Environment and Hardware Specifications

Run the benchmark on the machine hosting the Docker Compose deployment. The following details **must** appear in the final report, along with the test date and Git commit:

| Item | Command / collection method | Actual value |
|---|---|---|
| CPU model / logical cores | `lscpu` | Fill in after execution |
| Total / available RAM | `free -h` | Fill in after execution |
| Disk type (SSD/HDD) and capacity | `lsblk -o NAME,TYPE,ROTA,SIZE,MODEL`; `df -h` | Fill in after execution |
| OS / kernel | `uname -a`; `cat /etc/os-release` | Fill in after execution |
| Docker / Compose versions | `docker version`; `docker compose version` | Fill in after execution |
| Container resource limits | `docker inspect` or effective Compose configuration | Fill in after execution |
| Database / NATS versions | Image tags and volume sizes | Fill in after execution |
| Data configuration | Retention, sensor count, sampling interval, history depth, batch size | Fill in after execution |
| Source revision | `git rev-parse HEAD`; `git status --short` | Fill in after execution |

Example initial data collection on Linux:

```bash
mkdir -p loadtest/results/run-$(date -u +%Y%m%dT%H%M%SZ)
# Point RUN_DIR to the newly created directory.
RUN_DIR="$(ls -dt loadtest/results/run-* | head -1)"
lscpu > "$RUN_DIR/lscpu.txt"
free -h > "$RUN_DIR/memory.txt"
lsblk -o NAME,TYPE,ROTA,SIZE,MODEL > "$RUN_DIR/disks.txt"
df -h > "$RUN_DIR/filesystems.txt"
uname -a > "$RUN_DIR/kernel.txt"
docker version > "$RUN_DIR/docker-version.txt"
docker compose version > "$RUN_DIR/compose-version.txt"
docker compose config > "$RUN_DIR/compose-effective.yaml" # Redact secrets before sharing.
docker compose ps > "$RUN_DIR/compose-ps.txt"
git rev-parse HEAD > "$RUN_DIR/git-commit.txt"
```

**Security and reproducibility:** `compose-effective.yaml` may contain credentials; sanitize it before publication. Record database and filesystem free space, as well as cgroup limits, before and after each step. On macOS or Windows, report the CPU and memory assigned to Docker Desktop **separately** from the host hardware specifications.

## 3. Preparation and Load Isolation

- Back up existing database data and volumes. The test can generate a significant amount of data. **Do not run `docker compose down -v`** unless the environment is dedicated and deletion has been explicitly approved.
- Ensure that **only one** load-generating Simulator instance is running. Stop the default Compose `simulator` service before starting step-specific simulator instances, so they do not overlap:

  ```bash
  docker compose ps
  docker compose stop simulator
  docker compose up -d db nats api worker
  curl -fsS http://localhost:8080/ >/dev/null
  ```

  `curl /` is only a preliminary connectivity check. Confirm API, database, and queue health with the project's smoke tests. Before stopping an existing simulator, verify that doing so is permitted for the running deployment.
- Record any active backfill, retention, or other resource-heavy scheduled jobs. Avoid uncontrolled background load during capacity measurements.
- Use dedicated device IDs / data keys for each run, or run against a separate database. Otherwise, deduplication on `(device_id, time)`, pre-existing data, and residual queue catch-up can distort the observed database write rate.
- Prepare a **verified, real 365-day historical dataset** containing hourly rollups for the one-year chart test. This may require backfilling a separate test database. Assert the number of devices, time buckets, and actual temporal coverage. Setting `HISTORY_DAYS=0` does **not** create the required history.
- Monitor CPU, memory, and networking on the load generator. If the simulator saturates first, you have measured **generator capacity**, not system capacity. Move the generator to another machine or use multiple producers, then repeat the test.

## 4. Existing Tools and Required Fixes Before Making Capacity Claims

| Existing tool / path | Purpose | Required caution or change |
|---|---|---|
| `cmd/simulator/main.go` | Generates one sample per device per second; batches of up to 400 | Batch publishing is sequential and retries can continue indefinitely. Measure achieved load separately from offered load. At 10k devices, approximately 25 batches per tick are needed. |
| `loadtest/capacity_benchmark.py` | Device-count steps, DB throughput, query latency, CPU/RAM | `queue_pending()` currently returns `None`; without queue backlog measurements, stable consumption cannot be proven. |
| `loadtest/plot_capacity.py` | Generates four PNG charts from actual CSV data | Plot **measured data only**; exclude warm-up and invalid intervals from step summaries. |
| `loadtest/ingest.js` | k6 HTTP ingestion workload | A supplement to the simulator, not a replacement for the device-count scenario. |
| `loadtest/query.js` | k6 chart query load | A 10-VU, one-year scenario needs genuine 365-day history and a separate measurement of browser rendering. |

### Required runner improvements (preferably a small separate PR)

1. Implement `queue_pending()` using the NATS monitoring API / JetStream consumer information. Record `num_pending`, `num_ack_pending`, and their rates of change every five seconds. The NATS monitoring port `8222` is available inside the Compose network but is not published on the host; collect metrics securely from inside the network.
2. Compute the successful database write rate from a **verifiable counter**. PostgreSQL `pg_stat_user_tables.n_tup_ins` is cumulative, is subject to statistics resets and insertion behavior, and by itself does not prove every sample was stored. Use run-specific device IDs/time windows and exact `COUNT(*)` differences, or validated successful-commit metrics with end-to-end lag; divide by the exact elapsed interval.
3. Compute p95/p99 for **each step after warm-up**, using requests generated within that step. The current runner collects query latency samples during warm-up and produces CSV p95 values calculated from a growing sample set.
4. After each step, stop the previous simulator and let the queue drain or return to a documented baseline. Otherwise, database writes during the next step may belong to a previous step's devices.
5. Store time-series samples, not only per-step averages. Include disk IOPS/latency, disk free space, network activity, query errors, ingress rate, and queue backlog in addition to CPU and memory.
6. Treat failures to collect essential metrics (for example from `docker stats` or `psql`) as **INCONCLUSIVE**, not success. The absence of `matplotlib` must also cause plot generation to fail visibly.
7. At 10k devices, unbounded retries can stall the simulator for a long period. Add bounded timeouts and separate counters for successful/failed publication and generator-side backlog.
8. Generate a per-step **PASS / FAIL / INCONCLUSIVE** summary from predefined thresholds and supporting measurements. Return a nonzero exit status when required criteria cannot be verified.

## 5. Metrics, Thresholds, and Definition of Maximum Capacity

| Metric | Definition / collection | Initial acceptance criterion (declare before testing) |
|---|---|---|
| Offered load | `devices × 1 sample/s` | Reference value, not proof of achieved throughput |
| Actual accepted rate | Samples acknowledged with HTTP `202` per second, **not** HTTP requests per second | Close to offered load, without accumulating generator backlog or excessive retries |
| DB committed rate | Newly committed samples per second | `>= 99%` of offered load in the steady-state window (**proposed test criterion**, not a specification requirement) |
| End-to-end lag | p95 from sample generation to query visibility | `< 10 s` as an initial proposal; identify it as an assumption in the report |
| Queue backlog slope | Change in Pending + Ack Pending messages during steady state | `<= 0` over the window, allowing transient fluctuations; report values for each step |
| Ingestion / write errors | HTTP 4xx/5xx, publish failures, retries | `< 1%` unrecoverable error rate and no data loss |
| Chart API latency | p50/p95/p99 for 1h, 7d, and 365d queries at 10 VUs | p95 `< 1000 ms` for **end-to-end one-year chart display**; API alone does not verify this condition |
| Browser chart latency | p95 from request initiation to fully rendered chart | `< 1000 ms` for a one-year dataset, seven sensors, and 10 concurrent users (specification target) |
| Resource headroom | CPU, RAM, I/O, disk, network, and container/host health | No OOM, sustained severe throttling, or saturation; establish any percentage ceiling before testing |

**PASS rule:** A step passes only if, following two minutes of warm-up, **all** persistence, queue-backlog, error-rate, API/chart latency, and resource-health conditions remain satisfied over a **15-minute** measurement period. If browser or queue metrics are unavailable, do not claim complete SLO-qualified capacity; mark the result **INCONCLUSIVE**. Repeat steps near the capacity boundary three times and report a conservative supported capacity.

## 6. Test Scenarios and Execution Order

### A — Baseline (low load)

- Start at **50** and **100** devices, with five minutes per step including two minutes of warm-up.
- Verify Compose health, sample correctness, query functionality, device coverage, and achieved generation rate.
- Treat these as baseline observations, **not** as evidence of maximum capacity.

### B — Step Load / Capacity Search

- Suggested device counts: **100 → 250 → 500 → 1000 → 2000 → 3000 → 5000 → 7500 → 10000**.
- Use five-minute steps for initial fault discovery. Starting at 1,000 devices, use **17 minutes per step**: two-minute warm-up plus 15 minutes of measurement.
- During ingestion, run **10 concurrent query virtual users (VUs)** covering 1-hour, 7-day, and 365-day windows. Only include the one-year test after verifying the historical dataset.
- After the first failure, narrow the boundary using a binary search between the last PASS and the first FAIL (for example, if 3,000 passes and 5,000 fails, test 4,000).
- Between steps, allow the queue to drain or reach a documented comparable state. Record the drain time separately.

### C — Soak / Stability

- Sustain the highest passing step for **30–60 minutes**.
- Check for memory leaks, growing backlog, throughput degradation, and latency drift.
- Confirm final capacity with repeat runs and the soak test; a single 15-minute PASS is not sufficient for a strong capacity claim.

### D — Stress Test Toward 10,000 Devices

- Attempt the 10k-device scenario even if a lower step fails, **only in a dedicated environment with a disk/queue budget and explicit stop conditions**.
- Stop when backlog grows persistently, latency spikes uncontrollably, database disk space approaches a critical threshold, OOM occurs, or committed throughput falls sharply. A destructive failure is unnecessary.
- Correlate component-specific metrics to identify the **first observed bottleneck**, rather than assuming one in advance.

## 7. Commands to Run on Your Machine

From the project root:

```bash
docker compose ps
docker compose stop simulator
docker compose up -d db nats api worker
python3 --version
python3 -m pip install matplotlib   # Prefer installation in a virtual environment.
```

Run the **existing** benchmark script for a short exploratory test. **Until the fixes in Section 4 are implemented, these results are preliminary only**:

```bash
python3 loadtest/capacity_benchmark.py \
  --devices 50,100,250,500,1000 \
  --seconds 120 --warmup 20 --sample-seconds 5 \
  --query-workers 2 --query-window-days 0.0416667 \
  --out loadtest/results/exploratory

python3 loadtest/plot_capacity.py --input loadtest/results/exploratory
```

**Important:** The current runner includes warm-up samples in its p95 calculation, does not collect Queue Pending, and `--query-workers` only controls HTTP concurrency. This command is **not** sufficient to establish final capacity or prove a one-year chart SLO.

Example independent k6 one-year query test, **only after loading the required one-year data**:

```bash
docker run --rm --network server-monitoring_default \
  -v "$PWD/loadtest:/scripts:ro" \
  -e API_URL=http://api:8080 -e VUS=10 -e DURATION=15m \
  -e QUERY_DEVICE=server-00000 \
  grafana/k6 run /scripts/query.js
```

The Compose network is often called `server-monitoring_default`; verify its actual name with `docker network ls`. The current `query.js` measures HTTP latency only. Add a separate browser-based measurement (for example, Playwright plus the Performance API) to measure end-to-end chart rendering. Record the actual query time range and validate the historical data coverage.

## 8. Required Charts and Data Sources

All plots must use **actual measured samples** and include axis labels, units, sample intervals, and a run identifier.

1. **Write Throughput:** X-axis = device count; Y-axis = samples/second. Plot three series: **offered**, **accepted by NATS**, and **persisted in the database**. Add a time-series plot for the 10k-device step to show sustained behavior and queue backlog growth.
2. **Chart Response Time:** X-axis = device count; Y-axis = milliseconds. Show API p50, p95, and p99, and **browser rendering p95 separately**. Distinguish 1-hour, 7-day, and 365-day query windows. Mark the 1,000 ms target as a reference line.
3. **Resource Usage Under Load:** Show CPU and RAM consumption separately for API, Worker, TimescaleDB, NATS, and Simulator. Generate **at least one CPU chart and one RAM chart**. Disk I/O and queue backlog plots are strongly recommended for diagnosing the bottleneck.

The existing `plot_capacity.py` generates four PNG files from `samples.csv`:

- `01_write_rate.png`
- `02_query_latency.png`
- `03_resource_usage.png`
- `04_memory_usage.png`

These provide a starting point, **but** currently lack a confirmed NATS-accepted series, backlog observations, and browser rendering p95. Extend the instrumentation before treating the images as complete evidence. Never label example, projected, or synthetic values as **measured**.

## 9. Bottleneck Identification Guide

| Observed symptom | Likely explanation | First diagnostic / change to evaluate |
|---|---|---|
| Offered > Accepted; simulator CPU high | Generator bottleneck or sequential HTTP batch publishing | Separate producer host; bounded parallel publishing; batch publication timing |
| Accepted high; Persisted low; queue backlog continually growing | Worker or database cannot keep up | Consumer lag, transaction timing, locks, database I/O |
| Worker CPU high; DB underutilized | Processing/serialization overhead or single-worker limitations | Profiling, batching, concurrency with ordering and idempotency guarantees |
| DB CPU, I/O, or lock waits high | Transactional write amplification from raw inserts + two rollups, indexes | `EXPLAIN`, `pg_stat_statements`, batch insertion, index and rollup pipeline analysis |
| JetStream memory/disk pressure and backpressure | Undelivered messages or queue storage limit | Increase consumer throughput; review queue capacity and retention, not only memory |
| Chart p95 high while ingestion remains stable | Query plan/DB contention or UI rendering | `EXPLAIN ANALYZE`, query concurrency, browser traces; isolate API from rendering |
| Host CPU saturated or throttling | Host capacity or cgroup quota | Record host utilization; adjust limits/resources or separate roles |

**Code-based hypotheses:** The current simulator publishes batches sequentially, while the Worker writes samples together with raw and two rollup updates. These are plausible bottlenecks, **not measured conclusions**. Identify the real bottleneck by correlating achieved production, acceptance, commit rate, lag, profiles, and resource metrics on your machine.

## 10. Analysis for 10,000 Devices

Assuming one sample per device per second:

- Required ingest rate: **10,000 samples/second**, or **864 million samples/day**. Each sample has seven sensor values.
- With a maximum batch size of **400 samples**, at least **25 batches/second** are required. This excludes JSON parsing, network overhead, indexing, WAL writes, and the two rollup updates.
- Retaining seven days of raw data at that rate represents substantial storage and WAL pressure. Measure actual bytes/sample, compression behavior, and free storage before setting capacity limits.

**Recommended investigation and optimization order (conditional on measurements):**

1. **Verify that the simulator can actually offer 10k samples/second.** If it cannot, add controlled parallel producers or multiple independent generators. Do not attribute generator failure to the database.
2. **If queue backlog rises:** Profile database commit time and Worker throughput. Experiment with database batching, bulk statements, consumer count, and `MaxAckPending`. Increasing concurrency without examining transaction conflicts and deduplication is risky.
3. **If the database saturates:** Measure the cost of raw ingestion plus transactional rollups. Evaluate index design, Timescale chunk sizing and compression, WAL/disk I/O, bulk inserts, and—only if justified—an asynchronous rollup pipeline with explicit correctness and replay guarantees.
4. **If charts slow down:** Keep queries on time-bucketed rollups and within the 1,500-bucket limit. Inspect query plans and cache behavior. Consider separating ingest and read workloads if measurement supports it.
5. **If disk I/O or available space is the limit:** Consider faster/larger SSD storage, revised retention/compression, and monitoring of WAL and JetStream disk use.

**Evidence-based statement for the report:** “At 10,000 devices, the required offered rate is 10,000 samples/second. We identify the first failing component only after measuring actual generation, accepted and committed rates, queue growth, chart p95, and CPU/RAM/I/O.” Until the benchmark is executed, do **not** name any particular component as the definite first failure.

## 11. Final Report Template (Complete After Testing)

### Run details

- UTC date: `_____`; Git commit: `_____`; Docker / Compose versions: `_____`
- CPU: `_____`; RAM: `_____`; Disk: `_____`; OS: `_____`
- Historical query dataset: `_____` days; Devices: `_____`; Concurrent query VUs: `_____`
- Acceptance thresholds: `_____`; Warm-up duration: `_____`; Measurement duration: `_____`

### Results

| Devices | Offered samples/s | Accepted/s | DB committed/s | Queue slope | Query p95 ms | Browser p95 ms | Peak DB/Worker CPU | Error % | PASS/FAIL |
|---:|---:|---:|---:|---:|---:|---:|---|---:|---|
| 100 | — | — | — | — | — | — | — | — | — |
| 500 | — | — | — | — | — | — | — | — | — |
| 1000 | — | — | — | — | — | — | — | — | — |
| 2000 | — | — | — | — | — | — | — | — | — |
| 5000 | — | — | — | — | — | — | — | — | — |
| 10000 | — | — | — | — | — | — | — | — | — |

### Evidence and conclusions

- Highest sustained capacity on the recorded hardware: `_____` devices (subject to the defined SLO).
- First failing step: `_____` devices; Failed criterion: `_____`.
- Evidence-backed bottleneck: `_____`; Supporting charts/logs: `_____`.
- Repeated / confirmation tests: `_____`.
- P0/P1 changes and expected impact (**hypotheses until A/B-tested**): `_____`.
- Required evidence files: `01_write_rate.png`, `02_query_latency.png`, `03_resource_usage.png`, and `04_memory_usage.png`, plus `samples.csv` and the hardware inventory.

## 12. Definition of Done

- [ ] Host hardware characteristics and Docker resource limits are recorded.
- [ ] No extra producers, repeated test data, or carried-over queue backlog distort the steps.
- [ ] Offered, accepted, and committed rates are measured independently.
- [ ] Queue depth/lag and disk capacity are tracked.
- [ ] The one-year chart test runs with actual 365-day data and 10 concurrent VUs.
- [ ] Browser **chart rendering** p95 (not only API latency) is measured.
- [ ] Each step has a predefined threshold and a PASS / FAIL / INCONCLUSIVE outcome.
- [ ] At least the three required graph categories are generated from real CSV measurements.
- [ ] The last PASS and first FAIL are repeated; a soak test is completed.
- [ ] The 10k-device assessment identifies the first **observed** point of saturation using data.
- [ ] No existing Docker volume is accidentally deleted or corrupted during the test.

---

**Reminder:** This is an **execution plan**, not a completed benchmark report. The proposed thresholds in Section 5 are test contract assumptions, **not measured findings or claims about the capacity of your hardware**.
