import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// Custom metrics
const loginSuccessRate = new Rate('login_success_rate');
const loginDuration = new Trend('login_duration');
const registerSuccessRate = new Rate('register_success_rate');
const registerDuration = new Trend('register_duration');
const refreshSuccessRate = new Rate('refresh_success_rate');
const failedLogins = new Counter('failed_logins');

// Test configuration
export const options = {
  stages: [
    { duration: '30s', target: 10 },  // Ramp up to 10 users
    { duration: '1m', target: 50 },   // Ramp up to 50 users
    { duration: '2m', target: 100 },  // Ramp up to 100 users
    { duration: '1m', target: 100 },  // Stay at 100 users
    { duration: '30s', target: 0 },   // Ramp down to 0 users
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'], // 95% of requests under 500ms
    http_req_failed: ['rate<0.01'],                 // Less than 1% failure rate
    login_success_rate: ['rate>0.95'],              // 95% login success
    register_success_rate: ['rate>0.95'],           // 95% register success
    login_duration: ['p(95)<300'],                  // 95% login under 300ms
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

export function setup() {
  console.log(`Starting load test against ${BASE_URL}`);

  // Health check
  const healthRes = http.get(`${BASE_URL}/health`);
  check(healthRes, {
    'health check successful': (r) => r.status === 200,
  });

  return { baseUrl: BASE_URL };
}

export default function (data) {
  const testUserEmail = `loadtest-${__VU}-${__ITER}@example.com`;
  const testPassword = 'LoadTest123!@#SecurePassword';

  group('Authentication Flow', function () {
    // Test user registration
    group('User Registration', function () {
      const registerPayload = JSON.stringify({
        email: testUserEmail,
        password: testPassword,
        display_name: `Load Test User ${__VU}`,
      });

      const registerParams = {
        headers: {
          'Content-Type': 'application/json',
        },
        tags: { name: 'Register' },
      };

      const registerStart = Date.now();
      const registerRes = http.post(
        `${data.baseUrl}/api/v1/auth/register`,
        registerPayload,
        registerParams
      );
      const registerEnd = Date.now();

      const registerSuccess = check(registerRes, {
        'register status is 201': (r) => r.status === 201,
        'register returns access_token': (r) => {
          try {
            const body = JSON.parse(r.body);
            return body.access_token !== undefined;
          } catch (e) {
            return false;
          }
        },
        'register returns refresh_token': (r) => {
          try {
            const body = JSON.parse(r.body);
            return body.refresh_token !== undefined;
          } catch (e) {
            return false;
          }
        },
      });

      registerSuccessRate.add(registerSuccess);
      registerDuration.add(registerEnd - registerStart);

      if (!registerSuccess) {
        console.error(`Registration failed for ${testUserEmail}: ${registerRes.status} - ${registerRes.body}`);
        return;
      }

      const registerBody = JSON.parse(registerRes.body);
      const accessToken = registerBody.access_token;
      const refreshToken = registerBody.refresh_token;

      sleep(0.5);

      // Test user login
      group('User Login', function () {
        const loginPayload = JSON.stringify({
          email: testUserEmail,
          password: testPassword,
        });

        const loginParams = {
          headers: {
            'Content-Type': 'application/json',
          },
          tags: { name: 'Login' },
        };

        const loginStart = Date.now();
        const loginRes = http.post(
          `${data.baseUrl}/api/v1/auth/login`,
          loginPayload,
          loginParams
        );
        const loginEnd = Date.now();

        const loginSuccess = check(loginRes, {
          'login status is 200': (r) => r.status === 200,
          'login returns tokens': (r) => {
            try {
              const body = JSON.parse(r.body);
              return body.access_token !== undefined && body.refresh_token !== undefined;
            } catch (e) {
              return false;
            }
          },
          'login response time < 500ms': (r) => (loginEnd - loginStart) < 500,
        });

        loginSuccessRate.add(loginSuccess);
        loginDuration.add(loginEnd - loginStart);

        if (!loginSuccess) {
          failedLogins.add(1);
          console.error(`Login failed for ${testUserEmail}: ${loginRes.status}`);
        }
      });

      sleep(0.3);

      // Test token refresh
      group('Token Refresh', function () {
        const refreshPayload = JSON.stringify({
          refresh_token: refreshToken,
        });

        const refreshParams = {
          headers: {
            'Content-Type': 'application/json',
          },
          tags: { name: 'RefreshToken' },
        };

        const refreshRes = http.post(
          `${data.baseUrl}/api/v1/auth/refresh`,
          refreshPayload,
          refreshParams
        );

        const refreshSuccess = check(refreshRes, {
          'refresh status is 200': (r) => r.status === 200,
          'refresh returns new tokens': (r) => {
            try {
              const body = JSON.parse(r.body);
              return body.access_token !== undefined;
            } catch (e) {
              return false;
            }
          },
        });

        refreshSuccessRate.add(refreshSuccess);
      });

      sleep(0.2);

      // Test accessing protected endpoint
      group('Protected Endpoint Access', function () {
        const meParams = {
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${accessToken}`,
          },
          tags: { name: 'GetProfile' },
        };

        const meRes = http.get(`${data.baseUrl}/api/v1/users/me`, meParams);

        check(meRes, {
          'get profile status is 200': (r) => r.status === 200,
          'get profile returns email': (r) => {
            try {
              const body = JSON.parse(r.body);
              return body.email === testUserEmail;
            } catch (e) {
              return false;
            }
          },
        });
      });
    });
  });

  sleep(1);
}

export function teardown(data) {
  console.log('Load test completed');
  console.log(`Test ran against: ${data.baseUrl}`);
}

export function handleSummary(data) {
  return {
    'summary.json': JSON.stringify(data),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
  };
}

function textSummary(data, options) {
  const indent = options.indent || '';
  const enableColors = options.enableColors || false;

  let summary = '\n';
  summary += `${indent}✓ checks.........................: ${data.metrics.checks.values.passes}/${data.metrics.checks.values.passes + data.metrics.checks.values.fails}\n`;
  summary += `${indent}  http_req_duration..............: avg=${data.metrics.http_req_duration.values.avg.toFixed(2)}ms p(95)=${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms\n`;
  summary += `${indent}  http_req_failed................: ${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%\n`;
  summary += `${indent}  login_success_rate.............: ${(data.metrics.login_success_rate.values.rate * 100).toFixed(2)}%\n`;
  summary += `${indent}  register_success_rate..........: ${(data.metrics.register_success_rate.values.rate * 100).toFixed(2)}%\n`;
  summary += `${indent}  iterations.....................: ${data.metrics.iterations.values.count}\n`;
  summary += `${indent}  vus............................: ${data.metrics.vus.values.value} (max=${data.metrics.vus_max.values.value})\n`;

  return summary;
}
