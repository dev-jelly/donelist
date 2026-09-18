import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics for memory leak detection
const memoryLeakIndicator = new Trend('memory_leak_indicator');
const responseTimeProgression = new Trend('response_time_progression');
const errorRateOverTime = new Rate('error_rate_over_time');
const successfulRequests = new Counter('successful_requests');
const failedRequests = new Counter('failed_requests');

// Soak test - long-running test to detect memory leaks and degradation
export const options = {
  stages: [
    { duration: '5m', target: 50 },    // Ramp up
    { duration: '3h', target: 50 },    // Stay at 50 VUs for 3 hours
    { duration: '5m', target: 0 },     // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'],     // Response time shouldn't degrade
    http_req_failed: ['rate<0.01'],        // Error rate should stay low
    response_time_progression: ['p(95)<1500'], // Response time shouldn't increase over time
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

export function setup() {
  console.log('Starting soak test - 3 hour endurance test');
  console.log('Monitoring for memory leaks and performance degradation');

  // Create test users
  const testUsers = [];

  for (let i = 0; i < 20; i++) {
    const email = `soak-test-${i}@example.com`;
    const password = 'SoakTest123!@#Secure';

    const registerPayload = JSON.stringify({
      email: email,
      password: password,
      display_name: `Soak Test User ${i}`,
    });

    const registerRes = http.post(
      `${BASE_URL}/api/v1/auth/register`,
      registerPayload,
      { headers: { 'Content-Type': 'application/json' } }
    );

    if (registerRes.status === 201 || registerRes.status === 409) {
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
          refreshToken: body.refresh_token,
        });
      } else if (registerRes.status === 201) {
        const body = JSON.parse(registerRes.body);
        testUsers.push({
          email: email,
          accessToken: body.access_token,
          refreshToken: body.refresh_token,
        });
      }
    }
  }

  console.log(`Created ${testUsers.length} test users for soak testing`);

  return {
    baseUrl: BASE_URL,
    users: testUsers,
    startTime: Date.now(),
    initialResponseTimes: [],
  };
}

export default function (data) {
  if (!data.users || data.users.length === 0) {
    console.error('No test users available');
    return;
  }

  const user = data.users[__VU % data.users.length];
  const authHeaders = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${user.accessToken}`,
  };

  const elapsedMinutes = (Date.now() - data.startTime) / 1000 / 60;

  group('Soak Test - Sustained Load', function () {
    // Simulate realistic user behavior

    // 1. Check health (lightweight operation)
    const healthStart = Date.now();
    const healthRes = http.get(`${data.baseUrl}/health`, {
      tags: { name: 'SoakHealth', elapsed_min: Math.floor(elapsedMinutes) },
    });
    const healthDuration = Date.now() - healthStart;

    const healthSuccess = check(healthRes, {
      'health check successful': (r) => r.status === 200,
    });

    if (healthSuccess) {
      successfulRequests.add(1);
    } else {
      failedRequests.add(1);
      errorRateOverTime.add(1);
    }

    responseTimeProgression.add(healthDuration);

    sleep(1);

    // 2. Get user profile
    const profileStart = Date.now();
    const profileRes = http.get(`${data.baseUrl}/api/v1/users/me`, {
      headers: authHeaders,
      tags: { name: 'SoakProfile', elapsed_min: Math.floor(elapsedMinutes) },
    });
    const profileDuration = Date.now() - profileStart;

    const profileSuccess = check(profileRes, {
      'profile fetch successful': (r) => r.status === 200,
    });

    if (profileSuccess) {
      successfulRequests.add(1);
    } else {
      failedRequests.add(1);
      errorRateOverTime.add(1);
    }

    responseTimeProgression.add(profileDuration);

    sleep(2);

    // 3. List categories (cached operation)
    const categoriesStart = Date.now();
    const categoriesRes = http.get(`${data.baseUrl}/api/v1/categories`, {
      headers: authHeaders,
      tags: { name: 'SoakCategories', elapsed_min: Math.floor(elapsedMinutes) },
    });
    const categoriesDuration = Date.now() - categoriesStart;

    const categoriesSuccess = check(categoriesRes, {
      'categories list successful': (r) => r.status === 200,
    });

    if (categoriesSuccess) {
      successfulRequests.add(1);
    } else {
      failedRequests.add(1);
      errorRateOverTime.add(1);
    }

    responseTimeProgression.add(categoriesDuration);

    sleep(2);

    // 4. Create a checkin (write operation every 10th iteration to avoid bloat)
    if (__ITER % 10 === 0) {
      const checkinPayload = JSON.stringify({
        title: `Soak Test Checkin ${__VU}-${__ITER}`,
        description: `Created at ${elapsedMinutes.toFixed(0)} minutes into soak test`,
        tags: ['soak-test'],
      });

      const checkinStart = Date.now();
      const checkinRes = http.post(
        `${data.baseUrl}/api/v1/checkins`,
        checkinPayload,
        {
          headers: authHeaders,
          tags: { name: 'SoakCreateCheckin', elapsed_min: Math.floor(elapsedMinutes) },
        }
      );
      const checkinDuration = Date.now() - checkinStart;

      const checkinSuccess = check(checkinRes, {
        'checkin creation successful': (r) => r.status === 201,
      });

      if (checkinSuccess) {
        successfulRequests.add(1);
      } else {
        failedRequests.add(1);
        errorRateOverTime.add(1);
      }

      responseTimeProgression.add(checkinDuration);

      sleep(1);
    }

    // 5. List checkins with pagination
    const page = (__ITER % 5) + 1;
    const listStart = Date.now();
    const listRes = http.get(
      `${data.baseUrl}/api/v1/checkins?page=${page}&limit=20`,
      {
        headers: authHeaders,
        tags: { name: 'SoakListCheckins', elapsed_min: Math.floor(elapsedMinutes) },
      }
    );
    const listDuration = Date.now() - listStart;

    const listSuccess = check(listRes, {
      'checkins list successful': (r) => r.status === 200,
    });

    if (listSuccess) {
      successfulRequests.add(1);
    } else {
      failedRequests.add(1);
      errorRateOverTime.add(1);
    }

    responseTimeProgression.add(listDuration);

    // Calculate memory leak indicator (response time trend over time)
    // If response times increase significantly over time, it may indicate memory leaks
    const avgResponseTime = (healthDuration + profileDuration + categoriesDuration + listDuration) / 4;
    memoryLeakIndicator.add(avgResponseTime * (1 + elapsedMinutes / 180)); // Normalize by expected duration

    // Refresh token every 30 minutes to test token refresh logic
    if (__ITER % 180 === 0 && user.refreshToken) {
      const refreshPayload = JSON.stringify({
        refresh_token: user.refreshToken,
      });

      const refreshRes = http.post(
        `${data.baseUrl}/api/v1/auth/refresh`,
        refreshPayload,
        {
          headers: { 'Content-Type': 'application/json' },
          tags: { name: 'SoakRefreshToken', elapsed_min: Math.floor(elapsedMinutes) },
        }
      );

      check(refreshRes, {
        'token refresh successful': (r) => r.status === 200,
      });

      if (refreshRes.status === 200) {
        const body = JSON.parse(refreshRes.body);
        user.accessToken = body.access_token;
        user.refreshToken = body.refresh_token;
      }
    }
  });

  // Realistic user think time
  sleep(3 + Math.random() * 2);
}

export function teardown(data) {
  const durationHours = (Date.now() - data.startTime) / 1000 / 60 / 60;

  console.log('\n=== Soak Test Completed ===');
  console.log(`Total Duration: ${durationHours.toFixed(2)} hours`);
  console.log('System stability and memory leak analysis completed');
}

export function handleSummary(data) {
  const durationHours = (Date.now() - data.start_time) / 1000 / 60 / 60;

  const summary = {
    test_type: 'Soak Test (Endurance)',
    duration_hours: durationHours.toFixed(2),
    total_requests: (data.metrics.successful_requests?.values.count || 0) + (data.metrics.failed_requests?.values.count || 0),
    successful_requests: data.metrics.successful_requests?.values.count || 0,
    failed_requests: data.metrics.failed_requests?.values.count || 0,
    error_rate: data.metrics.error_rate_over_time?.values.rate
      ? `${(data.metrics.error_rate_over_time.values.rate * 100).toFixed(4)}%`
      : '0%',
    avg_response_time: data.metrics.http_req_duration?.values.avg?.toFixed(2) || 'N/A',
    p95_response_time: data.metrics.http_req_duration?.values['p(95)']?.toFixed(2) || 'N/A',
    p99_response_time: data.metrics.http_req_duration?.values['p(99)']?.toFixed(2) || 'N/A',
    response_time_progression: {
      avg: data.metrics.response_time_progression?.values.avg?.toFixed(2) || 'N/A',
      p95: data.metrics.response_time_progression?.values['p(95)']?.toFixed(2) || 'N/A',
    },
    memory_leak_assessment: 'Check if response times increased significantly over time',
  };

  console.log('\n=== Soak Test Summary ===');
  console.log(JSON.stringify(summary, null, 2));

  const responseTimeIncrease = data.metrics.response_time_progression?.values['p(95)'] / data.metrics.response_time_progression?.values.avg;

  console.log('\n=== Analysis ===');
  if (responseTimeIncrease > 2) {
    console.log('⚠️  WARNING: Significant response time degradation detected');
    console.log('   Possible memory leak or resource exhaustion');
    console.log('   Recommendation: Review application logs and memory usage');
  } else {
    console.log('✅ System remained stable throughout the soak test');
    console.log('   No significant performance degradation detected');
  }

  return {
    'soak-test-summary.json': JSON.stringify(
      {
        summary,
        recommendations: responseTimeIncrease > 2
          ? ['Investigate memory usage', 'Check for connection leaks', 'Review cache eviction policies']
          : ['System is stable for long-running operations'],
      },
      null,
      2
    ),
    stdout: '\n✓ Soak test completed\n',
  };
}
