# Load Testing with k6

This directory contains comprehensive load testing scripts using [k6](https://k6.io/) to verify system scalability, performance, and reliability under various load conditions.

## Prerequisites

Install k6:

```bash
# macOS
brew install k6

# Linux (Debian/Ubuntu)
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update
sudo apt-get install k6

# Windows
choco install k6
```

## Test Scripts Overview

### 1. Authentication Load Test (`auth_load_test.js`)
**Purpose**: Test authentication system performance under load

**What it tests**:
- User registration throughput
- Login performance
- Token refresh operations
- Protected endpoint access

**Load profile**:
- Ramp up: 10 → 50 → 100 users over 3.5 minutes
- Duration: 5 minutes total
- Concurrent operations per user: 4-5 requests

**Run**:
```bash
k6 run tests/load/auth_load_test.js
# Or with custom API URL
k6 run -e API_URL=http://localhost:8080 tests/load/auth_load_test.js
```

**Success criteria**:
- 95% of requests < 500ms
- 99% of requests < 1000ms
- Error rate < 1%
- Login success rate > 95%

---

### 2. Checkin Operations Load Test (`checkin_load_test.js`)
**Purpose**: Test core CRUD operations under sustained load

**What it tests**:
- Checkin creation throughput
- List operations with pagination
- Update and delete operations
- Category management
- Concurrent write operations

**Load profile**:
- Ramp up: 20 → 50 → 100 → 150 users over 4.5 minutes
- Peak load: 150 concurrent users
- Duration: 6 minutes total

**Run**:
```bash
k6 run tests/load/checkin_load_test.js
```

**Success criteria**:
- 95% of create operations < 500ms
- 95% of list operations < 300ms
- Error rate < 2%
- Create success rate > 95%

---

### 3. Spike Test (`spike_test.js`)
**Purpose**: Test system resilience to sudden traffic spikes

**What it tests**:
- System behavior during traffic surges
- Recovery after spike
- Error handling under stress
- API availability during spikes

**Load profile**:
- Normal: 10 users
- SPIKE: Jump to 200 users in 10 seconds
- Sustain: 200 users for 1 minute
- Recovery: Drop back to 10 users

**Run**:
```bash
k6 run tests/load/spike_test.js
```

**Success criteria**:
- System remains responsive during spike
- Recovery rate > 90%
- P95 latency < 2000ms during spike
- Error rate < 5%

**Analysis**:
- Compare `response_time_normal` vs `response_time_spike`
- System should recover quickly after spike
- No cascading failures

---

### 4. Stress Test (`stress_test.js`)
**Purpose**: Find system breaking point

**What it tests**:
- Maximum concurrent user capacity
- System degradation patterns
- Resource exhaustion points
- Breaking point identification

**Load profile**:
- Progressive load: 50 → 100 → 200 → 300 → 400 → 500 users
- Duration: 12 minutes total
- Identifies when system starts failing

**Run**:
```bash
k6 run tests/load/stress_test.js
```

**Success criteria**:
- Identify maximum sustainable load
- Document degradation point
- System should not crash

**Analysis**:
- Note `max_successful_vus` - maximum users before degradation
- Use findings to set autoscaling thresholds
- Plan capacity based on expected traffic

---

### 5. Soak Test (`soak_test.js`)
**Purpose**: Detect memory leaks and long-term stability issues

**What it tests**:
- Memory leak detection
- Performance degradation over time
- Connection pool exhaustion
- Cache effectiveness
- Token refresh longevity

**Load profile**:
- Sustained: 50 concurrent users for 3 hours
- Realistic user behavior with think times
- Token refreshes every 30 minutes

**Run**:
```bash
# This test takes 3+ hours
k6 run tests/load/soak_test.js

# Shorter version for testing (10 minutes)
k6 run tests/load/soak_test.js --stage 1m:50,10m:50,1m:0
```

**Success criteria**:
- Response times remain stable
- No increasing error rate over time
- Memory usage remains constant

**Analysis**:
- Check `response_time_progression` for degradation
- Monitor application memory usage during test
- Look for connection leaks or cache issues

---

## Running Tests

### Quick Start

```bash
# Ensure API is running
make run  # In another terminal

# Run all load tests
make load-test-all

# Run specific test
make load-test-auth
make load-test-checkin
make load-test-spike
make load-test-stress
make load-test-soak
```

### Custom Configuration

```bash
# Set custom API URL
export API_URL=http://api.example.com
k6 run tests/load/auth_load_test.js

# Run with custom duration
k6 run --stage 30s:10,1m:50,30s:0 tests/load/auth_load_test.js

# Run with specific VUs
k6 run --vus 100 --duration 2m tests/load/checkin_load_test.js

# Save results to file
k6 run --out json=results.json tests/load/auth_load_test.js

# Send results to InfluxDB
k6 run --out influxdb=http://localhost:8086/k6 tests/load/auth_load_test.js
```

## Understanding Results

### Key Metrics

- **http_req_duration**: Response time for HTTP requests
  - `avg`: Average response time
  - `p(95)`: 95th percentile - 95% of requests faster than this
  - `p(99)`: 99th percentile - 99% of requests faster than this

- **http_req_failed**: Percentage of failed requests
  - Should be < 1% under normal load
  - Up to 5% acceptable during spike tests

- **vus**: Virtual users (concurrent users)
  - `vus`: Current number of virtual users
  - `vus_max`: Maximum virtual users during test

- **iterations**: Number of complete test iterations

### Thresholds

Tests will FAIL if thresholds are not met:
- ✅ Green: All thresholds passed
- ❌ Red: One or more thresholds failed

### Custom Metrics

Each test defines custom metrics for specific analysis:
- `login_success_rate`: Percentage of successful logins
- `checkin_create_duration`: Time to create a checkin
- `spike_recovery_rate`: Recovery rate after traffic spike
- `memory_leak_indicator`: Trend showing potential memory leaks

## Best Practices

### Before Running Tests

1. **Ensure clean state**: Reset database or use test environment
2. **Warm up**: Run a quick test first to warm caches
3. **Monitor resources**: Use monitoring tools to observe system behavior
4. **Baseline**: Run tests against known-good build first

### During Tests

1. **Monitor**: Watch application logs, metrics, and resource usage
2. **Document**: Note any errors or unusual behavior
3. **Observe**: Check database connections, cache hit rates, memory usage

### After Tests

1. **Analyze results**: Review all metrics and custom measurements
2. **Compare**: Compare with previous test runs
3. **Optimize**: Identify and fix bottlenecks
4. **Regression**: Add to CI/CD to prevent performance regressions

## Performance Targets

Based on requirements and infrastructure:

| Metric | Target | Acceptable | Critical |
|--------|--------|------------|----------|
| P95 Response Time (Read) | < 200ms | < 500ms | < 1000ms |
| P95 Response Time (Write) | < 300ms | < 800ms | < 1500ms |
| Error Rate | < 0.1% | < 1% | < 5% |
| Concurrent Users | 100+ | 50+ | 20+ |
| Throughput | 500 req/s | 200 req/s | 50 req/s |

## Continuous Integration

### GitHub Actions Example

```yaml
name: Load Tests

on:
  schedule:
    - cron: '0 2 * * 0'  # Weekly on Sunday at 2 AM
  workflow_dispatch:      # Manual trigger

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Start API
        run: |
          docker-compose up -d
          sleep 10

      - name: Install k6
        run: |
          sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
          echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" | sudo tee /etc/apt/sources.list.d/k6.list
          sudo apt-get update
          sudo apt-get install k6

      - name: Run Auth Load Test
        run: k6 run tests/load/auth_load_test.js

      - name: Run Checkin Load Test
        run: k6 run tests/load/checkin_load_test.js

      - name: Upload Results
        uses: actions/upload-artifact@v3
        with:
          name: load-test-results
          path: '*.json'
```

## Troubleshooting

### Common Issues

**High error rates**:
- Check database connection pool size
- Verify Redis is properly configured
- Check for rate limiting triggers
- Review application logs for errors

**Slow response times**:
- Check database query performance
- Verify indexes are properly created
- Monitor cache hit rates
- Review N+1 query patterns

**Memory leaks**:
- Run soak test for extended period
- Monitor application memory usage
- Check for unclosed database connections
- Review goroutine leaks (Go specific)

**Connection issues**:
- Increase connection pool limits
- Check file descriptor limits
- Verify network bandwidth
- Review timeout configurations

## Resources

- [k6 Documentation](https://k6.io/docs/)
- [k6 Best Practices](https://k6.io/docs/testing-guides/test-types/)
- [Performance Testing Patterns](https://k6.io/docs/testing-guides/automated-performance-testing/)
- [k6 Cloud](https://k6.io/cloud/) - For distributed load testing

## Support

For issues or questions:
1. Check application logs
2. Review k6 output for specific errors
3. Consult monitoring dashboards
4. File an issue with test results attached
