import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// Custom metrics
const spikeRecoveryRate = new Rate('spike_recovery_rate');
const responseTimeSpike = new Trend('response_time_during_spike');
const responseTimeNormal = new Trend('response_time_normal');

// Spike test configuration - sudden traffic increase
export const options = {
  stages: [
    { duration: '1m', target: 10 },    // Normal load
    { duration: '10s', target: 200 },  // SPIKE! Sudden jump to 200 users
    { duration: '1m', target: 200 },   // Stay at spike level
    { duration: '10s', target: 10 },   // Quick drop back
    { duration: '1m', target: 10 },    // Recovery period
    { duration: '10s', target: 0 },    // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'],  // Allow higher latency during spike
    http_req_failed: ['rate<0.05'],     // Allow up to 5% failure during spike
    spike_recovery_rate: ['rate>0.90'], // 90% should recover
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

let accessToken = null;

export function setup() {
  console.log('Setting up spike test...');

  // Register and login a test user
  const email = 'spike-test@example.com';
  const password = 'SpikeTest123!@#Secure';

  const registerPayload = JSON.stringify({
    email: email,
    password: password,
    display_name: 'Spike Test User',
  });

  const registerRes = http.post(
    `${BASE_URL}/api/v1/auth/register`,
    registerPayload,
    { headers: { 'Content-Type': 'application/json' } }
  );

  if (registerRes.status === 201) {
    const body = JSON.parse(registerRes.body);
    return {
      baseUrl: BASE_URL,
      accessToken: body.access_token,
    };
  }

  // If registration fails (user exists), try login
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
    return {
      baseUrl: BASE_URL,
      accessToken: body.access_token,
    };
  }

  console.error('Failed to setup test user');
  return { baseUrl: BASE_URL, accessToken: null };
}

export default function (data) {
  const authHeaders = data.accessToken
    ? {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${data.accessToken}`,
      }
    : { 'Content-Type': 'application/json' };

  const isSpikePeriod = __VU > 50; // VUs above 50 are part of the spike

  group('Spike Test - API Resilience', function () {
    // Test health endpoint
    const healthStart = Date.now();
    const healthRes = http.get(`${data.baseUrl}/health`, {
      tags: { name: 'HealthCheck', spike: isSpikePeriod },
    });
    const healthEnd = Date.now();

    const healthSuccess = check(healthRes, {
      'health check responds': (r) => r.status === 200,
      'health check fast': () => (healthEnd - healthStart) < 1000,
    });

    if (isSpikePeriod) {
      responseTimeSpike.add(healthEnd - healthStart);
    } else {
      responseTimeNormal.add(healthEnd - healthStart);
    }

    spikeRecoveryRate.add(healthSuccess);

    sleep(0.1);

    // Test API endpoints
    if (data.accessToken) {
      const apiStart = Date.now();
      const profileRes = http.get(`${data.baseUrl}/api/v1/users/me`, {
        headers: authHeaders,
        tags: { name: 'GetProfile', spike: isSpikePeriod },
      });
      const apiEnd = Date.now();

      const apiSuccess = check(profileRes, {
        'profile endpoint responds': (r) => r.status === 200 || r.status === 401,
        'profile response reasonable': () => (apiEnd - apiStart) < 2000,
      });

      spikeRecoveryRate.add(apiSuccess);

      if (isSpikePeriod) {
        responseTimeSpike.add(apiEnd - apiStart);
      } else {
        responseTimeNormal.add(apiEnd - apiStart);
      }
    }

    sleep(0.2);

    // Create some load on list endpoints
    const listStart = Date.now();
    const listRes = http.get(`${data.baseUrl}/api/v1/categories`, {
      headers: authHeaders,
      tags: { name: 'ListCategories', spike: isSpikePeriod },
    });
    const listEnd = Date.now();

    check(listRes, {
      'list endpoint responds': (r) => r.status === 200 || r.status === 401,
    });

    if (isSpikePeriod) {
      responseTimeSpike.add(listEnd - listStart);
    } else {
      responseTimeNormal.add(listEnd - listStart);
    }
  });

  sleep(0.5);
}

export function teardown(data) {
  console.log('\n=== Spike Test Summary ===');
  console.log('Spike test verifies system resilience to sudden traffic increases');
  console.log('Expected: System should handle spike gracefully and recover quickly');
}

export function handleSummary(data) {
  const summary = {
    'Avg Response Time (Normal)': data.metrics.response_time_normal?.values.avg || 'N/A',
    'Avg Response Time (Spike)': data.metrics.response_time_spike?.values.avg || 'N/A',
    'P95 Response Time (Normal)': data.metrics.response_time_normal?.values['p(95)'] || 'N/A',
    'P95 Response Time (Spike)': data.metrics.response_time_spike?.values['p(95)'] || 'N/A',
    'Recovery Rate': data.metrics.spike_recovery_rate?.values.rate
      ? `${(data.metrics.spike_recovery_rate.values.rate * 100).toFixed(2)}%`
      : 'N/A',
    'Failed Requests': data.metrics.http_req_failed?.values.rate
      ? `${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%`
      : 'N/A',
  };

  console.log('\n=== Performance Comparison ===');
  console.log(JSON.stringify(summary, null, 2));

  return {
    'spike-test-summary.json': JSON.stringify(data, null, 2),
    stdout: '\n✓ Spike test completed\n',
  };
}
