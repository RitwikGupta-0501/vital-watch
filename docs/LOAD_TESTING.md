# Load Testing & Capacity Benchmarks

## Executive Summary

To ensure high availability and stability, the Vital Watch backend was subjected to a rigorous capacity benchmarking suite using **Grafana k6**, **Prometheus**, and **Docker**. 

We utilized a binary search methodology to identify the exact breaking points of the system across two distinct workload profiles:
1. **Raw Infrastructure Baseline**: Validating the maximum throughput of the Go HTTP router and PostgreSQL connection pool.
2. **Full User Journey**: Validating the real-world capacity involving heavy cryptographic operations (bcrypt password hashing), JWT session management, and complex database transactions.

The results demonstrated exceptional performance, with the baseline effortlessly handling **2,000+ concurrent users** at `<4ms` latency, and the computationally-heavy authentication flow handling **142 concurrent users** under `500ms` latency (bottlenecked intentionally by bcrypt security scaling).

---

## Testing Architecture

The load testing stack is fully containerized and integrated with our observability infrastructure:
- **Load Generator**: `grafana/k6` executed via Docker.
- **Metrics Storage**: k6 is configured to remote-write native histograms directly to our local **Prometheus** instance (`http://localhost:9090/api/v1/write`).
- **Visualization**: Metrics are queryable in **Grafana** (`http://localhost:3000`) for real-time visualization of `http_req_duration` and `http_req_failed`.
- **Search Strategy**: A custom Python script (`scripts/loadtest/binary_search_capacity.py`) automates k6 execution, applying a binary search algorithm to narrow down the maximum Virtual User (VU) capacity based on strict pass/fail thresholds.

---

## 1. Raw Infrastructure Baseline Benchmark

The baseline test isolates the network layer, Go router, and database connection pool from complex business logic by heavily querying the `/api/healthz` endpoint.

**Test Configuration:**
- **Target Endpoint**: `GET /api/healthz`
- **Load Profile**: 15s Ramp-up, 30s Sustain, 15s Ramp-down
- **Success Criteria**: 95th Percentile (p95) Latency < 200ms AND Error Rate < 1%
- **Rate Limiting**: Bypassed for testing purposes.

### Binary Search Execution Log (Excerpt)

| Virtual Users | Result | p95 Latency | Error Rate |
|---------------|--------|-------------|------------|
| 1025          | ✅ PASS | 1.77ms      | 0.00%      |
| 1513          | ✅ PASS | 3.44ms      | 0.00%      |
| 1757          | ✅ PASS | 2.80ms      | 0.00%      |
| 1940          | ✅ PASS | 3.51ms      | 0.00%      |
| 2000          | ✅ PASS | 3.07ms      | 0.00%      |

**Result**: The test successfully scaled to the predefined absolute maximum of **2,000 Concurrent Virtual Users**. Even at maximum capacity, the API handled the load with a phenomenal p95 latency of `~3.07ms` and a `0.00%` error rate.

---

## 2. Full User Journey Benchmark

The user journey test simulates realistic traffic by executing a sequence of heavy business logic operations.

**Test Configuration:**
- **Target Endpoints**: `POST /api/register`, `POST /api/login`, `GET /api/profile`, `GET /api/doctors`
- **Load Profile**: 15s Ramp-up, 30s Sustain, 15s Ramp-down
- **Success Criteria**: 95th Percentile (p95) Latency < 500ms AND Error Rate < 5%
- **Rate Limiting**: Bypassed for testing purposes (normally capped at 10 req/min for auth).

### Binary Search Execution Log

| Virtual Users | Result | p95 Latency | Error Rate |
|---------------|--------|-------------|------------|
| 100           | ✅ PASS | 117.24ms    | 0.00%      |
| 125           | ✅ PASS | 202.63ms    | 0.00%      |
| 137           | ✅ PASS | 277.77ms    | 0.00%      |
| 140           | ✅ PASS | 456.81ms    | 0.00%      |
| 141           | ✅ PASS | 445.50ms    | 0.00%      |
| 142           | ✅ PASS | 487.05ms    | 0.00%      |
| 143           | ❌ FAIL | 888.18ms    | 0.00%      |
| 150           | ❌ FAIL | 555.28ms    | 0.00%      |

**Result**: The system comfortably handled **142 concurrent users** constantly creating accounts, logging in, and querying the database before the p95 latency crossed the 500ms threshold.

---

## Conclusion & Analysis

1. **Connection Pool Resiliency**: The Go `pgxpool` efficiently multiplexed 2,000 concurrent web requests onto a much smaller set of physical PostgreSQL connections without exhaustion or significant latency spikes.
2. **Cryptographic Bottlenecks**: The drop from 2,000 VUs to 142 VUs during the User Journey test is the direct result of `bcrypt` password hashing used in `/register` and `/login`. This is a strict security requirement (NIST SP 800-63B / HIPAA) to mitigate brute-force attacks by intentionally maximizing CPU time.
3. **Defense in Depth**: In a production environment, the `authLimiter` strictly restricts authentication endpoints to 10 requests per minute per IP. This ensures that the CPU bottleneck identified (142 VUs) can never be maliciously exploited by a single actor to cause a Denial of Service (DoS).
