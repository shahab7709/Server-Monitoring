# Measured load-test results: capacity-20261104

A maximum sustainable capacity requires repeated qualified steps and a soak run. This file reports individual observations only.

[Declared workload and stop budgets](contract.json). [Verified historical coverage](history-coverage.txt).

| Devices | Measured s | Accepted/s | Committed/s | Queue slope msg/s | Year API p95 ms | Browser p95 ms | Outcome |
|---:|---:|---:|---:|---:|---:|---:|---|
| 50 | 175.56 | 50.13 | 49.84 | 0.00 | 161.43 | 242.50 | INCONCLUSIVE |
| 100 | 175.15 | 99.91 | 99.91 | 0.00 | 189.08 | 258.90 | INCONCLUSIVE |
| 250 | 177.80 | 244.66 | 244.66 | 0.00 | 746.20 | 964.80 | FAIL |
| 500 | 175.40 | 501.72 | 498.87 | 0.00 | 315.83 | 365.30 | INCONCLUSIVE |
| 1000 | 895.57 | 1000.48 | 999.36 | 0.00 | 142.60 | 214.70 | INCONCLUSIVE |

## 00-50: INCONCLUSIVE

[Full summary](00-50/summary.json).

Missing: Less than 15 minutes measured; Query-API end-to-end visibility lag and complete resource health verification unavailable.

After drain: 15000 stored samples; 15000 logged accepted samples; 50 devices.

## 01-100: INCONCLUSIVE

[Full summary](01-100/summary.json).

Missing: Less than 15 minutes measured; Query-API end-to-end visibility lag and complete resource health verification unavailable.

After drain: 30000 stored samples; 30000 logged accepted samples; 100 devices.

## 02-250: FAIL

[Full summary](02-250/summary.json).

Failures: Committed rate below 99% offered; Accepted rate below 99% offered.

Missing: Less than 15 minutes measured; Query-API end-to-end visibility lag and complete resource health verification unavailable.

After drain: 74500 stored samples; 74500 logged accepted samples; 250 devices.

## 03-500: INCONCLUSIVE

[Full summary](03-500/summary.json).

Missing: Less than 15 minutes measured; Query-API end-to-end visibility lag and complete resource health verification unavailable.

After drain: 150000 stored samples; 150000 logged accepted samples; 500 devices.

## 04-1000: INCONCLUSIVE

[Full summary](04-1000/summary.json).

Missing: Less than 15 minutes measured; Query-API end-to-end visibility lag and complete resource health verification unavailable.

After drain: 1020000 stored samples; 1020000 logged accepted samples; 1000 devices.
