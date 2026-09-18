import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const checkinCreateRate = new Rate('checkin_create_success_rate');
const checkinCreateDuration = new Trend('checkin_create_duration');
const checkinListRate = new Rate('checkin_list_success_rate');
const checkinListDuration = new Trend('checkin_list_duration');
const checkinUpdateRate = new Rate('checkin_update_success_rate');
const categoryCreateRate = new Rate('category_create_success_rate');
const totalRequests = new Counter('total_requests');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 20 },   // Ramp up to 20 users
    { duration: '1m', target: 50 },    // Ramp up to 50 users
    { duration: '2m', target: 100 },   // Ramp up to 100 users
    { duration: '1m', target: 150 },   // Peak load at 150 users
    { duration: '1m', target: 100 },   // Scale down to 100
    { duration: '30s', target: 0 },    // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<800', 'p(99)<1500'],
    http_req_failed: ['rate<0.02'],
    checkin_create_success_rate: ['rate>0.95'],
    checkin_list_success_rate: ['rate>0.98'],
    checkin_create_duration: ['p(95)<500'],
    checkin_list_duration: ['p(95)<300'],
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

export function setup() {
  console.log(`Starting checkin load test against ${BASE_URL}`);

  // Create test users and get auth tokens
  const testUsers = [];

  for (let i = 0; i < 10; i++) {
    const email = `checkin-loadtest-${i}@example.com`;
    const password = 'LoadTest123!@#Secure';

    const registerPayload = JSON.stringify({
      email: email,
      password: password,
      display_name: `Checkin Test User ${i}`,
    });

    const registerRes = http.post(
      `${BASE_URL}/api/v1/auth/register`,
      registerPayload,
      {
        headers: { 'Content-Type': 'application/json' },
      }
    );

    if (registerRes.status === 201) {
      const body = JSON.parse(registerRes.body);
      testUsers.push({
        email: email,
        accessToken: body.access_token,
        refreshToken: body.refresh_token,
      });
    }
  }

  console.log(`Created ${testUsers.length} test users for load testing`);

  return { baseUrl: BASE_URL, users: testUsers };
}

export default function (data) {
  if (!data.users || data.users.length === 0) {
    console.error('No test users available');
    return;
  }

  // Select a random user for this iteration
  const user = data.users[__VU % data.users.length];

  const authHeaders = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${user.accessToken}`,
  };

  group('Checkin Operations', function () {
    let categoryId = null;

    // Create a category
    group('Create Category', function () {
      const categoryPayload = JSON.stringify({
        name: `Category-${__VU}-${__ITER}`,
        color: '#3498db',
        icon: 'folder',
      });

      const categoryRes = http.post(
        `${data.baseUrl}/api/v1/categories`,
        categoryPayload,
        { headers: authHeaders, tags: { name: 'CreateCategory' } }
      );

      const categorySuccess = check(categoryRes, {
        'category created': (r) => r.status === 201,
      });

      categoryCreateRate.add(categorySuccess);
      totalRequests.add(1);

      if (categorySuccess) {
        const categoryBody = JSON.parse(categoryRes.body);
        categoryId = categoryBody.id;
      }

      sleep(0.2);
    });

    // Create multiple checkins
    group('Create Checkins', function () {
      const checkinsToCreate = 5;
      const checkinIds = [];

      for (let i = 0; i < checkinsToCreate; i++) {
        const checkinPayload = JSON.stringify({
          title: `Load Test Checkin ${__VU}-${__ITER}-${i}`,
          description: `This is a load test checkin created by VU ${__VU} in iteration ${__ITER}`,
          category_id: categoryId,
          tags: ['loadtest', 'performance', `vu-${__VU}`],
          priority: i % 3 === 0 ? 'high' : 'normal',
        });

        const createStart = Date.now();
        const checkinRes = http.post(
          `${data.baseUrl}/api/v1/checkins`,
          checkinPayload,
          { headers: authHeaders, tags: { name: 'CreateCheckin' } }
        );
        const createEnd = Date.now();

        const createSuccess = check(checkinRes, {
          'checkin created successfully': (r) => r.status === 201,
          'checkin has ID': (r) => {
            try {
              return JSON.parse(r.body).id !== undefined;
            } catch (e) {
              return false;
            }
          },
          'checkin create time acceptable': () => (createEnd - createStart) < 1000,
        });

        checkinCreateRate.add(createSuccess);
        checkinCreateDuration.add(createEnd - createStart);
        totalRequests.add(1);

        if (createSuccess) {
          const checkinBody = JSON.parse(checkinRes.body);
          checkinIds.push(checkinBody.id);
        }

        sleep(0.1);
      }

      sleep(0.3);

      // List checkins with pagination
      group('List Checkins', function () {
        const listStart = Date.now();
        const listRes = http.get(
          `${data.baseUrl}/api/v1/checkins?page=1&limit=20`,
          { headers: authHeaders, tags: { name: 'ListCheckins' } }
        );
        const listEnd = Date.now();

        const listSuccess = check(listRes, {
          'checkins listed successfully': (r) => r.status === 200,
          'checkins list is array': (r) => {
            try {
              const body = JSON.parse(r.body);
              return Array.isArray(body.checkins);
            } catch (e) {
              return false;
            }
          },
          'list response time acceptable': () => (listEnd - listStart) < 500,
        });

        checkinListRate.add(listSuccess);
        checkinListDuration.add(listEnd - listStart);
        totalRequests.add(1);
      });

      sleep(0.2);

      // Update some checkins
      if (checkinIds.length > 0) {
        group('Update Checkins', function () {
          const checkinToUpdate = checkinIds[0];

          const updatePayload = JSON.stringify({
            title: `Updated Checkin ${__VU}-${__ITER}`,
            description: 'Updated during load test',
            status: 'completed',
          });

          const updateRes = http.put(
            `${data.baseUrl}/api/v1/checkins/${checkinToUpdate}`,
            updatePayload,
            { headers: authHeaders, tags: { name: 'UpdateCheckin' } }
          );

          const updateSuccess = check(updateRes, {
            'checkin updated successfully': (r) => r.status === 200,
          });

          checkinUpdateRate.add(updateSuccess);
          totalRequests.add(1);
        });

        sleep(0.2);

        // Delete a checkin
        group('Delete Checkin', function () {
          const checkinToDelete = checkinIds[checkinIds.length - 1];

          const deleteRes = http.del(
            `${data.baseUrl}/api/v1/checkins/${checkinToDelete}`,
            { headers: authHeaders, tags: { name: 'DeleteCheckin' } }
          );

          check(deleteRes, {
            'checkin deleted successfully': (r) => r.status === 200 || r.status === 204,
          });

          totalRequests.add(1);
        });
      }
    });

    // Test pagination performance
    group('Pagination Performance', function () {
      for (let page = 1; page <= 3; page++) {
        const paginationRes = http.get(
          `${data.baseUrl}/api/v1/checkins?page=${page}&limit=10`,
          { headers: authHeaders, tags: { name: `Pagination-Page${page}` } }
        );

        check(paginationRes, {
          [`pagination page ${page} successful`]: (r) => r.status === 200,
        });

        totalRequests.add(1);
        sleep(0.1);
      }
    });
  });

  sleep(1);
}

export function teardown(data) {
  console.log('Checkin load test completed');
  console.log(`Total test users: ${data.users.length}`);
}

export function handleSummary(data) {
  console.log('\n=== Checkin Load Test Summary ===');
  console.log(`Total Requests: ${data.metrics.total_requests.values.count}`);
  console.log(`HTTP Request Duration (avg): ${data.metrics.http_req_duration.values.avg.toFixed(2)}ms`);
  console.log(`HTTP Request Duration (p95): ${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms`);
  console.log(`HTTP Request Failed Rate: ${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%`);
  console.log(`Checkin Create Success Rate: ${(data.metrics.checkin_create_success_rate.values.rate * 100).toFixed(2)}%`);
  console.log(`Checkin List Success Rate: ${(data.metrics.checkin_list_success_rate.values.rate * 100).toFixed(2)}%`);
  console.log(`Iterations: ${data.metrics.iterations.values.count}`);
  console.log(`VUs (max): ${data.metrics.vus_max.values.value}`);

  return {
    'checkin-load-summary.json': JSON.stringify(data, null, 2),
    stdout: '\n✓ Checkin load test completed successfully\n',
  };
}
