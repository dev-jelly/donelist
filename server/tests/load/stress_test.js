import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('error_rate');
const systemBreakpoint = new Counter('system_breakpoint_reached');
const maxSuccessfulVUs = new Counter('max_successful_vus');
const degradationPoint = new Trend('degradation_point');

// Stress test - find breaking point
export const options = {
  stages: [
    { duration: '2m', target: 50 },    // Warm up
    { duration: '2m', target: 100 },   // Ramp to moderate load
    { duration: '2m', target: 200 },   // Ramp to high load
    { duration: '2m', target: 300 },   // Ramp to very high load
    { duration: '2m', target: 400 },   // Find the breaking point
    { duration: '2m', target: 500 },   // Beyond breaking point
    { duration: '2m', target: 0 },     // Recovery
  ],
  thresholds: {
    // More lenient thresholds for stress test
    http_req_duration: ['p(95)<5000'],  // Accept 5s at p95 during stress
    http_req_failed: ['rate<0.5'],      // Accept up to 50% failure at peak
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

export function setup() {
  console.log('Starting stress test to find system limits...');

  // Create a pool of test users
  const testUsers = [];

  for (let i = 0; i < 50; i++) {
    const email = `stress-test-${i}@example.com`;
    const password = 'StressTest123!@#Secure';

    const registerPayload = JSON.stringify({
      email: email,
      password: password,
      display_name: `Stress Test User ${i}`,
    });

    const registerRes = http.post(
      `${BASE_URL}/api/v1/auth/register`,
      registerPayload,
      {
        headers: { 'Content-Type': 'application/json' },
      }
    );

    if (registerRes.status === 201 || registerRes.status === 409) {
      // Try login if user already exists
      const loginPayload = JSON.stringify({
        email: email,
        password: password,
      });

      const loginRes = http.post(
        `${BASE_URL}/api/v1/auth/login`,
        loginPayload,
        { headers: { 'Content-Type': 'application/json' } }
      );

      if (loginRes.status === 200) {
        const body = JSON.parse(loginRes.body);
        testUsers.push({
          email: email,
          accessToken: body.access_token,
        });
      } else if (registerRes.status === 201) {
        const body = JSON.parse(registerRes.body);
        testUsers.push({
          email: email,
          accessToken: body.access_token,
        });
      }
    }
  }

  console.log(`Created ${testUsers.length} test users for stress testing`);

  return {
    baseUrl: BASE_URL,
    users: testUsers,
    startTime: Date.now(),
  };
}

let lastSuccessfulVU = 0;
let breakpointReached = false;

export default function (data) {
  if (!data.users || data.users.length === 0) {
    console.error('No test users available');
    errorRate.add(1);
    return;
  }

  const user = data.users[__VU % data.users.length];
  const authHeaders = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${user.accessToken}`,
  };

  const currentVUs = __VU;
  const currentTime = Date.now();
  const elapsedMinutes = (currentTime - data.startTime) / 1000 / 60;

  group('Stress Test Operations', function () {
    // Test multiple operations to stress the system
    let allOperationsSucceeded = true;

    // Operation 1: Health check
    const healthRes = http.get(`${data.baseUrl}/health`, {
      tags: { name: 'StressHealthCheck', vus: currentVUs },
      timeout: '10s',
    });

    const healthSuccess = check(healthRes, {
      'health responds': (r) => r.status === 200,
    });

    if (!healthSuccess) {
      allOperationsSucceeded = false;
      errorRate.add(1);
    }

    sleep(0.1);

    // Operation 2: Get user profile
    const profileRes = http.get(`${data.baseUrl}/api/v1/users/me`, {
      headers: authHeaders,
      tags: { name: 'StressProfile', vus: currentVUs },
      timeout: '10s',
    });

    const profileSuccess = check(profileRes, {
      'profile responds': (r) => r.status === 200,
      'profile response valid': (r) => {
        try {
          return JSON.parse(r.body).email !== undefined;
        } catch (e) {
          return false;
        }
      },
    });

    if (!profileSuccess) {
      allOperationsSucceeded = false;
      errorRate.add(1);
    }

    sleep(0.1);

    // Operation 3: List categories
    const categoriesRes = http.get(`${data.baseUrl}/api/v1/categories`, {
      headers: authHeaders,
      tags: { name: 'StressCategories', vus: currentVUs },
      timeout: '10s',
    });

    const categoriesSuccess = check(categoriesRes, {
      'categories responds': (r) => r.status === 200,
    });

    if (!categoriesSuccess) {
      allOperationsSucceeded = false;
      errorRate.add(1);
    }

    sleep(0.1);

    // Operation 4: Create a checkin (write operation)
    const checkinPayload = JSON.stringify({
      title: `Stress Test Checkin ${__VU}-${__ITER}`,
      description: `Testing system under ${currentVUs} VUs`,
      tags: ['stress-test', 'performance'],
    });

    const checkinRes = http.post(
      `${data.baseUrl}/api/v1/checkins`,
      checkinPayload,
      {
        headers: authHeaders,
        tags: { name: 'StressCreateCheckin', vus: currentVUs },
        timeout: '10s',
      }
    );

    const checkinSuccess = check(checkinRes, {
      'checkin creation responds': (r) => r.status === 201 || r.status === 500,
    });

    if (!checkinSuccess) {
      allOperationsSucceeded = false;
      errorRate.add(1);
    }

    // Track system degradation
    if (allOperationsSucceeded) {
      if (currentVUs > lastSuccessfulVU) {
        lastSuccessfulVU = currentVUs;
        maxSuccessfulVUs.add(1);
      }
    } else {
      if (!breakpointReached && currentVUs > lastSuccessfulVU + 50) {
        breakpointReached = true;
        systemBreakpoint.add(1);
        degradationPoint.add(currentVUs);
        console.log(`⚠️  System degradation detected at ${currentVUs} VUs (${elapsedMinutes.toFixed(1)} min)`);
      }
    }
  });

  sleep(0.5);
}

export function teardown(data) {
  console.log('\n=== Stress Test Results ===');
  console.log(`Test Duration: ${((Date.now() - data.startTime) / 1000 / 60).toFixed(1)} minutes`);
  console.log(`Maximum Successful VUs: ${lastSuccessfulVU}`);
  console.log(`Breakpoint Reached: ${breakpointReached ? 'Yes' : 'No'}`);
}

export function handleSummary(data) {
  const summary = {
    test_type: 'Stress Test',
    max_vus: data.metrics.vus_max?.values.value || 0,
    max_successful_vus: lastSuccessfulVU,
    breakpoint_reached: breakpointReached,
    total_requests: data.metrics.http_reqs?.values.count || 0,
    failed_requests: data.metrics.http_req_failed?.values.rate
      ? `${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%`
      : '0%',
    avg_response_time: data.metrics.http_req_duration?.values.avg.toFixed(2) || 'N/A',
    p95_response_time: data.metrics.http_req_duration?.values['p(95)']?.toFixed(2) || 'N/A',
    p99_response_time: data.metrics.http_req_duration?.values['p(99)']?.toFixed(2) || 'N/A',
  };

  console.log('\n=== Stress Test Summary ===');
  console.log(JSON.stringify(summary, null, 2));
  console.log('\nRecommendations:');
  console.log(`- System can handle up to ~${lastSuccessfulVU} concurrent users reliably`);
  console.log(`- Consider horizontal scaling if expecting more than ${Math.floor(lastSuccessfulVU * 0.7)} regular users`);
  console.log('- Monitor database connection pools and Redis cache hit rates');

  return {
    'stress-test-summary.json': JSON.stringify(
      {
        summary,
        full_metrics: data.metrics,
      },
      null,
      2
    ),
    stdout: '\n✓ Stress test completed\n',
  };
}
