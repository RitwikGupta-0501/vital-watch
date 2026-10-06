import http from 'k6/http';
import { check, sleep } from 'k6';

// The baseline script tests the /healthz endpoint. 
// This establishes the absolute maximum capacity of the Go HTTP server and
// database connection pool without complex business logic or authentication overhead.

const targetVUs = __ENV.TARGET_VUS ? parseInt(__ENV.TARGET_VUS) : 50;

export const options = {
  // Define a phased load profile
  stages: [
    { duration: '15s', target: targetVUs },  // Ramp-up
    { duration: '30s', target: targetVUs },   // Sustain
    { duration: '15s', target: 0 },   // Ramp-down
  ],
  // Thresholds define the pass/fail criteria for the test
  thresholds: {
    http_req_duration: ['p(95)<200'], // 95% of requests must complete below 200ms
    http_req_failed: ['rate<0.01'],   // Less than 1% of requests can fail
  },
};

export default function () {
  // Use host.docker.internal if running k6 in docker on Mac/Windows, 
  // or localhost if running k6 natively on the host machine.
  // We use the exposed port 8000 for the vital-watch-api container.
  const url = 'http://localhost:8000/api/healthz';

  const params = {
    headers: {
      'Content-Type': 'application/json',
    },
  };

  const res = http.get(url, params);

  // Validate the response
  check(res, {
    'status is 200': (r) => r.status === 200,
    'database is up': (r) => r.json('database') === 'up',
  });

  // Brief sleep to simulate real-user think time and prevent the test from 
  // turning into a pure network benchmark of the local loopback adapter.
  sleep(1);
}

// ---------------------------------------------------------------------------------
// How to run this test and send metrics to the local Prometheus instance:
// ---------------------------------------------------------------------------------
// K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
// K6_PROMETHEUS_RW_TREND_AS_NATIVE_HISTOGRAM=true \
// k6 run --out experimental-prometheus-rw scripts/loadtest/baseline.js
// ---------------------------------------------------------------------------------
