// k6 profile for a cached read path (PRD §2 G4: 7,000 RPS peak, p99 < 200ms).
//
// Run:  task load                                   (default: ramp to TARGET_RPS)
//       k6 run -e TARGET_RPS=1000 -e READ_PATH=/api/v1/things/42 test/load/read_path.js
//
// The template ships no business routes, so READ_PATH defaults to /health.
// Point it at your module's hottest GET once one exists. The reference
// profile registered one user in setup() and had every VU read the SAME
// key — a hot key is exactly where singleflight and the cache decorator
// have to earn their place (R9); docs/examples.md keeps that shape.
import http from 'k6/http';
import { check } from 'k6';
import { Trend } from 'k6/metrics';

const BASE = __ENV.BASE || 'http://localhost:8080';
const READ_PATH = __ENV.READ_PATH || '/health';
const TARGET_RPS = parseInt(__ENV.TARGET_RPS || '2000', 10);
const DURATION = __ENV.DURATION || '30s';

const readLatency = new Trend('read_latency', true);

export const options = {
  scenarios: {
    cached_reads: {
      executor: 'ramping-arrival-rate',
      startRate: Math.max(1, Math.floor(TARGET_RPS / 10)),
      timeUnit: '1s',
      preAllocatedVUs: 200,
      maxVUs: 2000,
      stages: [
        { target: Math.floor(TARGET_RPS / 2), duration: '10s' }, // warm the cache
        { target: TARGET_RPS, duration: '10s' },                 // ramp to peak
        { target: TARGET_RPS, duration: DURATION },              // hold
      ],
    },
  },
  thresholds: {
    // G4: p99 < 200ms on the cached read path.
    'http_req_duration{expected_response:true}': ['p(99)<200'],
    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  const res = http.get(`${BASE}${READ_PATH}`, {
    tags: { name: `GET ${READ_PATH}` },
  });
  readLatency.add(res.timings.duration);
  check(res, {
    'status is 200': (r) => r.status === 200,
  });
}
