# Donelist API - Monitoring Implementation Summary

## Overview

This document provides a complete overview of the monitoring and observability stack implemented for the Donelist API. It covers all aspects from metrics collection to incident response procedures.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Metrics Implementation](#metrics-implementation)
3. [Log Aggregation](#log-aggregation)
4. [Distributed Tracing](#distributed-tracing)
5. [Alerting and Notifications](#alerting-and-notifications)
6. [Visualization and Dashboards](#visualization-and-dashboards)
7. [SLO/SLI Tracking](#slosli-tracking)
8. [Deployment Guide](#deployment-guide)
9. [Troubleshooting](#troubleshooting)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         Donelist API Pods                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐         │
│  │   API Pod 1  │  │   API Pod 2  │  │   API Pod 3  │         │
│  │              │  │              │  │              │         │
│  │ /metrics     │  │ /metrics     │  │ /metrics     │         │
│  │ /health      │  │ /health      │  │ /health      │         │
│  │ OTLP Export  │  │ OTLP Export  │  │ OTLP Export  │         │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘         │
│         │                  │                  │                  │
└─────────┼──────────────────┼──────────────────┼─────────────────┘
          │                  │                  │
          │ Metrics          │ Metrics          │ Metrics
          │ Traces           │ Traces           │ Traces
          │ Logs             │ Logs             │ Logs
          │                  │                  │
          ▼                  ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Observability Layer                           │
│                                                                  │
│  ┌────────────────┐  ┌────────────────┐  ┌─────────────────┐  │
│  │   Prometheus   │  │      OTEL      │  │   Fluent Bit    │  │
│  │   (Metrics)    │  │   Collector    │  │     (Logs)      │  │
│  │                │  │   (Traces)     │  │                 │  │
│  │  - Scraping    │  │  - OTLP        │  │  - Log collect  │  │
│  │  - Storage     │  │  - Jaeger      │  │  - Parsing      │  │
│  │  - Evaluation  │  │  - Tempo       │  │  - Enrichment   │  │
│  └────────┬───────┘  └────────┬───────┘  └────────┬────────┘  │
│           │                   │                    │            │
└───────────┼───────────────────┼────────────────────┼───────────┘
            │                   │                    │
            │                   │                    │
            ▼                   ▼                    ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Visualization & Analysis                    │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                        Grafana                           │   │
│  │  - API Overview Dashboard                               │   │
│  │  - Database Performance Dashboard                       │   │
│  │  - Business Metrics Dashboard                           │   │
│  │  - SLO Dashboard                                        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                  │
│  ┌────────────────┐  ┌────────────────┐  ┌─────────────────┐  │
│  │ Jaeger UI      │  │ Loki/ES        │  │  Prometheus UI  │  │
│  │ (Trace View)   │  │ (Log Search)   │  │  (Metrics)      │  │
│  └────────────────┘  └────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
            │
            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Alerting & Notification                       │
│                                                                  │
│  ┌────────────────┐           ┌──────────────────────────┐     │
│  │ AlertManager   │──────────▶│  Notification Channels   │     │
│  │                │           │  - Slack                 │     │
│  │ - Grouping     │           │  - PagerDuty             │     │
│  │ - Routing      │           │  - Email                 │     │
│  │ - Inhibition   │           │  - Webhooks              │     │
│  └────────────────┘           └──────────────────────────┘     │
└─────────────────────────────────────────────────────────────────┘
```

---

## Metrics Implementation

### Task 17.2: Response Time Percentile Metrics (COMPLETE)

#### Implementation Details

**Location**: `/server/internal/metrics/metrics.go`

**Metrics Exported**:
```go
// HTTP Request Duration with custom buckets for percentile calculation
HTTPRequestDuration: promauto.NewHistogramVec(
    prometheus.HistogramOpts{
        Name: "http_request_duration_seconds",
        Help: "HTTP request latency in seconds",
        // Optimized buckets for p50, p95, p99 calculation
        Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
    },
    []string{"method", "path"},
)
```

**Percentile Queries**:
```promql
# P50 (median)
histogram_quantile(0.50,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le, path)
)

# P95
histogram_quantile(0.95,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le, path)
)

# P99
histogram_quantile(0.99,
  sum(rate(http_request_duration_seconds_bucket[5m])) by (le, path)
)
```

**Middleware Integration**:
```go
// File: internal/metrics/middleware.go
func MetricsMiddleware(m *Metrics) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        // Track in-flight requests
        m.HTTPRequestsInFlight.WithLabelValues(method).Inc()
        defer m.HTTPRequestsInFlight.WithLabelValues(method).Dec()

        c.Next()

        // Record request duration for percentile calculation
        duration := time.Since(start).Seconds()
        m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
    }
}
```

**Dashboard Integration**:
- Grafana panel showing p50, p95, p99 side by side
- Alerts on p95 > 1s and p99 > 2.5s
- Per-endpoint breakdown available

---

### Task 17.3: Custom Business Metrics (COMPLETE)

#### Implementation Details

**Location**: `/server/internal/metrics/business_metrics.go`

**Key Business Metrics**:

1. **User Engagement**:
   ```go
   // Daily/Weekly/Monthly Active Users
   UserActiveDailyGauge: promauto.NewGauge(...)
   UserActiveWeeklyGauge: promauto.NewGauge(...)
   UserActiveMonthlyGauge: promauto.NewGauge(...)

   // User signups and logins
   UserSignups: promauto.NewCounterVec(...)  // by method (oauth, email)
   UserLogins: promauto.NewCounterVec(...)   // by method, success
   ```

2. **Content Metrics**:
   ```go
   // Check-in operations with rate tracking
   CheckinCreations: promauto.NewCounterVec(...)      // per minute calculation
   CheckinUpdates: promauto.NewCounterVec(...)
   CheckinDeletions: promauto.NewCounterVec(...)

   // Check-in duration tracking
   CheckinDurationHistogram: promauto.NewHistogramVec(...)
   ```

3. **Feature Usage**:
   ```go
   // Search, timeline, teams
   SearchQueries: promauto.NewCounterVec(...)
   TimelineViews: promauto.NewCounterVec(...)
   TeamCreations: promauto.NewCounterVec(...)

   // Premium features
   PremiumUpgrades: promauto.NewCounterVec(...)
   PremiumFeatureUsage: promauto.NewCounterVec(...)
   ```

4. **System Health**:
   ```go
   // WebSocket connections
   WebSocketConnections: prometheus.Gauge(...)

   // Webhook deliveries
   WebhookDeliveries: promauto.NewCounterVec(...)
   WebhookFailures: promauto.NewCounterVec(...)

   // Sync operations
   SyncOperations: promauto.NewCounterVec(...)
   SyncConflicts: promauto.NewCounterVec(...)
   ```

**Rate Calculations**:
```promql
# Check-ins per minute
rate(donelist_checkin_creations_total[1m]) * 60

# Active users (current)
donelist_users_active_daily

# Webhook success rate
(sum(rate(donelist_webhook_deliveries_total{status="success"}[5m])) /
 sum(rate(donelist_webhook_deliveries_total[5m]))) * 100
```

**Business Dashboards**:
- **KPI Dashboard**: DAU/WAU/MAU, signup trends, engagement metrics
- **Content Dashboard**: Check-in creation rate, category usage, timeline views
- **Feature Dashboard**: Premium adoption, API key usage, webhook health

---

## Log Aggregation

### Task 17.4: Log Aggregation Pipeline (COMPLETE)

#### Implementation Details

**Technology Stack**: Fluent Bit → Loki/Elasticsearch

**Configuration**: `/server/deploy/k8s/logging-stack.yaml`

**Pipeline Architecture**:

1. **Collection Layer** (Fluent Bit DaemonSet):
   ```yaml
   [INPUT]
       Name tail
       Path /var/log/containers/donelist-api-*.log
       Parser docker, cri
       Tag kube.*
   ```

2. **Processing Layer**:
   ```yaml
   [FILTER]
       Name kubernetes
       Match kube.*
       Merge_Log On
       K8S-Logging.Parser On

   [FILTER]
       Name parser
       Match kube.*
       Key_Name log
       Parser json

   [FILTER]
       Name record_modifier
       Match kube.*
       Record app donelist
       Record environment ${ENVIRONMENT}
       Record cluster ${CLUSTER_NAME}
   ```

3. **Output Options**:

   **Option A: Loki** (Recommended for Kubernetes):
   ```yaml
   [OUTPUT]
       Name loki
       Match kube.*
       Url http://loki.monitoring.svc:3100/loki/api/v1/push
       Labels job=donelist-api,namespace=donelist
       Auto_Kubernetes_Labels On
   ```

   **Option B: Elasticsearch**:
   ```yaml
   [OUTPUT]
       Name es
       Match kube.*
       Host elasticsearch.monitoring.svc
       Port 9200
       Index donelist-api
       Logstash_Format On
       Logstash_Prefix donelist-api
   ```

**Log Structure** (Application):
```go
// Structured logging with zap
logger.Info("request processed",
    zap.String("method", method),
    zap.String("path", path),
    zap.Int("status", status),
    zap.Duration("duration", duration),
    zap.String("user_id", userID),
    zap.String("trace_id", traceID),
)
```

**Log Queries**:
```
# Loki LogQL examples

# All errors in last hour
{app="donelist", level="error"} |= "" [1h]

# Slow requests
{app="donelist"} | json | duration > 1s

# Errors by endpoint
sum by(path) (rate({app="donelist", level="error"}[5m]))

# Database errors
{app="donelist"} |= "database" |= "error"
```

**Integration with Grafana**:
- Loki datasource configured
- Log panels in dashboards
- Correlation with metrics via exemplars
- Jump from traces to logs

---

## Distributed Tracing

### Task 17.5: APM and Distributed Tracing (COMPLETE)

#### Implementation Details

**Technology Stack**: OpenTelemetry → Jaeger/Tempo

**Configuration**: `/server/deploy/k8s/opentelemetry.yaml`

**Architecture Components**:

1. **Application Instrumentation**:
   ```go
   // Initialize OpenTelemetry tracer
   import (
       "go.opentelemetry.io/otel"
       "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
       "go.opentelemetry.io/otel/sdk/trace"
   )

   // Create trace provider
   exporter, _ := otlptracegrpc.New(ctx,
       otlptracegrpc.WithEndpoint("otel-collector:4317"),
       otlptracegrpc.WithInsecure(),
   )

   tp := trace.NewTracerProvider(
       trace.WithBatcher(exporter),
       trace.WithResource(resource.NewWithAttributes(
           semconv.ServiceNameKey.String("donelist-api"),
       )),
   )
   ```

2. **Automatic Instrumentation**:
   ```go
   // HTTP server instrumentation
   import "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

   router.Use(otelgin.Middleware("donelist-api"))

   // Database instrumentation
   import "go.opentelemetry.io/contrib/instrumentation/database/sql/otelsql"

   db, _ := otelsql.Open("postgres", dsn,
       otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
   )
   ```

3. **Custom Spans**:
   ```go
   func ProcessCheckin(ctx context.Context, checkin *Checkin) error {
       tracer := otel.Tracer("donelist-api")
       ctx, span := tracer.Start(ctx, "ProcessCheckin")
       defer span.End()

       // Add attributes
       span.SetAttributes(
           attribute.String("checkin.id", checkin.ID),
           attribute.String("user.id", checkin.UserID),
       )

       // Create child spans
       ctx, dbSpan := tracer.Start(ctx, "database.saveCheckin")
       err := db.SaveCheckin(ctx, checkin)
       dbSpan.End()

       if err != nil {
           span.RecordError(err)
           span.SetStatus(codes.Error, err.Error())
           return err
       }

       return nil
   }
   ```

4. **OpenTelemetry Collector Configuration**:
   ```yaml
   receivers:
     otlp:
       protocols:
         grpc:
           endpoint: 0.0.0.0:4317
         http:
           endpoint: 0.0.0.0:4318

   processors:
     batch:
       timeout: 10s
       send_batch_size: 1024

     probabilistic_sampler:
       sampling_percentage: 10.0  # 10% sampling for high volume

   exporters:
     jaeger:
       endpoint: jaeger-collector:14250

     otlp:
       endpoint: tempo:4317

   service:
     pipelines:
       traces:
         receivers: [otlp]
         processors: [probabilistic_sampler, batch]
         exporters: [jaeger, otlp]
   ```

5. **Trace Analysis Queries**:
   ```
   # Jaeger UI queries

   # Find slow traces
   service=donelist-api minDuration=1s

   # Find errors
   service=donelist-api tags=error:true

   # Find specific operations
   service=donelist-api operation=POST /api/v1/checkins

   # Trace by user
   service=donelist-api tags=user.id:123
   ```

**Key Metrics from Traces**:
- Request flow visualization
- Service dependencies
- Latency breakdown by operation
- Error propagation
- Database query performance

**Integration Points**:
- Trace IDs in logs for correlation
- Exemplars in Prometheus metrics
- Links from metrics to traces in Grafana
- Alert context includes trace samples

---

## Alerting and Notifications

### Task 17.6: Alert Rules and Thresholds (COMPLETE)

#### Implementation Details

**Configuration**:
- Alert Rules: `/server/deploy/k8s/monitoring.yaml`
- AlertManager: `/server/deploy/k8s/alertmanager-config.yaml`

**Alert Categories**:

1. **Critical Alerts** (Page immediately):

   ```yaml
   # Critical Error Rate
   - alert: CriticalErrorRate
     expr: |
       (sum(rate(http_requests_total{status=~"5.."}[5m])) /
        sum(rate(http_requests_total[5m]))) > 0.15
     for: 2m
     labels:
       severity: critical
     annotations:
       summary: "Critical error rate detected"
       description: "Error rate is {{ $value | humanizePercentage }}"
       runbook_url: "https://docs/runbooks#critical-error-rate"

   # Database Connection Failure
   - alert: DatabaseConnectionFailure
     expr: rate(database_connection_errors_total[5m]) > 0
     for: 2m
     labels:
       severity: critical

   # Pod Crash Looping
   - alert: PodCrashLooping
     expr: rate(kube_pod_container_status_restarts_total[15m]) > 0
     for: 5m
     labels:
       severity: critical
   ```

2. **Warning Alerts** (Notify, don't page):

   ```yaml
   # High Error Rate
   - alert: HighErrorRate
     expr: |
       (sum(rate(http_requests_total{status=~"5.."}[5m])) /
        sum(rate(http_requests_total[5m]))) > 0.05
     for: 5m
     labels:
       severity: warning

   # High Response Time
   - alert: HighResponseTime
     expr: |
       histogram_quantile(0.95,
         sum(rate(http_request_duration_seconds_bucket[5m])) by (le)
       ) > 1
     for: 5m
     labels:
       severity: warning

   # High Memory Usage
   - alert: HighMemoryUsage
     expr: |
       (container_memory_working_set_bytes /
        container_spec_memory_limit_bytes) > 0.85
     for: 5m
     labels:
       severity: warning
   ```

3. **Info Alerts** (Log only):

   ```yaml
   # Low Cache Hit Rate
   - alert: LowCacheHitRate
     expr: |
       (sum(rate(cache_hits_total[15m])) /
        (sum(rate(cache_hits_total[15m])) +
         sum(rate(cache_misses_total[15m])))) < 0.7
     for: 15m
     labels:
       severity: info
   ```

**AlertManager Configuration**:

```yaml
route:
  receiver: 'default'
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h

  routes:
  # Critical alerts - immediate notification
  - match:
      severity: critical
    receiver: 'critical-alerts'
    group_wait: 5s
    repeat_interval: 4h
    continue: true

  # Warning alerts - grouped notifications
  - match:
      severity: warning
    receiver: 'warning-alerts'
    group_wait: 30s
    repeat_interval: 12h

receivers:
  # Critical alerts - Slack + PagerDuty
  - name: 'critical-alerts'
    slack_configs:
    - channel: '#donelist-critical'
      title: ':rotating_light: CRITICAL: {{ .GroupLabels.alertname }}'
      text: |
        *Alert:* {{ .GroupLabels.alertname }}
        *Summary:* {{ .CommonAnnotations.summary }}
        *Description:* {{ .CommonAnnotations.description }}
        *Runbook:* {{ .CommonAnnotations.runbook_url }}
    pagerduty_configs:
    - service_key: 'PAGERDUTY_SERVICE_KEY'

  # Warning alerts - Slack only
  - name: 'warning-alerts'
    slack_configs:
    - channel: '#donelist-alerts'
      title: ':warning: WARNING: {{ .GroupLabels.alertname }}'
```

**Inhibition Rules**:
```yaml
inhibit_rules:
  # Suppress warning if critical is firing
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname', 'cluster']

  # Suppress pod alerts if deployment failing
  - source_match:
      alertname: 'DeploymentRolloutStuck'
    target_match_re:
      alertname: 'Pod.*'
    equal: ['namespace', 'deployment']
```

**Alert Thresholds Summary**:

| Metric | Warning | Critical | Duration |
|--------|---------|----------|----------|
| Error Rate | 5% | 15% | 5m / 2m |
| Response Time (p95) | 1s | 2.5s | 5m / 2m |
| Memory Usage | 85% | 95% | 5m / 2m |
| CPU Usage | 85% | 95% | 5m / 2m |
| DB Connections | 80% limit | 95% limit | 5m / 2m |
| Cache Hit Rate | < 70% | < 50% | 15m / 5m |
| Pod Restarts | 1 / 15m | 3 / 15m | 5m / 2m |

---

## Visualization and Dashboards

### Task 17.7: Grafana Dashboards (COMPLETE)

#### Implementation Details

**Configuration**: `/server/deploy/k8s/grafana-dashboards.yaml`

**Dashboard Collection**:

1. **API Overview Dashboard**:
   - **Purpose**: High-level health and performance
   - **Panels**:
     - Request Rate (req/s) by method and status
     - Response Time (p50, p95, p99)
     - Error Rate (%) with threshold lines
     - Active Connections (WebSocket + HTTP)
     - Request/Response Size Distribution (heatmaps)
     - Top 10 Endpoints by Request Count
     - Top 10 Slowest Endpoints (p95)
   - **Refresh**: 30s
   - **Time Range**: Last 6 hours

2. **Database Performance Dashboard**:
   - **Purpose**: Database and cache monitoring
   - **Panels**:
     - Database Connection Pool Status
     - DB Query Duration (p50, p95, p99) by operation
     - DB Error Rate by Operation and Table
     - Cache Hit Rate (%)
     - Cache Operation Duration
     - Slowest Queries by Table
     - Connection Pool Metrics
   - **Refresh**: 30s
   - **Time Range**: Last 6 hours

3. **Business Metrics Dashboard**:
   - **Purpose**: Product and user metrics
   - **Panels**:
     - Daily/Weekly/Monthly Active Users (stats)
     - Check-in Creation Rate by Category
     - User Signups by Method (OAuth, Email)
     - Timeline Views with Cache Hit Rate
     - Search Queries by Type
     - Webhook Delivery Success Rate (gauge)
     - Sync Conflict Rate
     - Premium Feature Usage
   - **Refresh**: 1m
   - **Time Range**: Last 24 hours

4. **SLO Dashboard**:
   - **Purpose**: SLI/SLO tracking and error budget
   - **Panels**:
     - API Availability (SLO: 99.9%)
     - Request Success Rate (SLO: 99.5%)
     - Response Time p95 (SLO: < 500ms)
     - Error Budget Remaining (7 days)
     - Latency Histogram (7 days)
     - SLO Compliance Trend
   - **Refresh**: 1m
   - **Time Range**: Last 7 days
   - **Thresholds**: Color-coded (green/yellow/red)

**Dashboard Features**:
- **Variables**: Environment, namespace, pod selection
- **Annotations**: Deployments, incidents, maintenance
- **Links**:
  - Jump to logs (Loki)
  - View traces (Jaeger)
  - Alert history
- **Exemplars**: Click on metrics to see related traces
- **Time Sync**: All panels use same time range
- **Export**: JSON available for version control

**Access URLs**:
```
# Production
https://grafana.example.com/d/donelist-overview
https://grafana.example.com/d/donelist-database
https://grafana.example.com/d/donelist-business
https://grafana.example.com/d/donelist-slo

# Development
http://localhost:3000/d/donelist-overview
```

---

## SLO/SLI Tracking

### Service Level Objectives

**Defined SLOs**:

1. **Availability SLO**: 99.9% (43 minutes downtime/month)
   ```promql
   # SLI Calculation
   (1 - (
     sum(rate(http_requests_total{status=~"5.."}[30d])) /
     sum(rate(http_requests_total[30d]))
   )) * 100
   ```

2. **Latency SLO**: 95% of requests < 500ms
   ```promql
   # SLI Calculation
   histogram_quantile(0.95,
     sum(rate(http_request_duration_seconds_bucket[30d])) by (le)
   ) < 0.5
   ```

3. **Error Rate SLO**: < 0.5% error rate
   ```promql
   # SLI Calculation
   (sum(rate(http_requests_total{status=~"5.."}[30d])) /
    sum(rate(http_requests_total[30d]))) < 0.005
   ```

**Error Budget Calculation**:
```promql
# Error budget remaining (30 days)
((1 - 0.999) - (
  sum(rate(http_requests_total{status=~"5.."}[30d])) /
  sum(rate(http_requests_total[30d]))
)) / (1 - 0.999) * 100
```

**SLO Dashboard Features**:
- Real-time SLI values
- Error budget burn rate
- Historical trends
- Alerting on budget consumption > 80%
- Forecasting for budget exhaustion

---

## Deployment Guide

### Prerequisites

1. **Kubernetes Cluster**: 1.25+
2. **Prometheus Operator**: Installed
3. **Storage**: PersistentVolumes for Prometheus, Grafana
4. **DNS**: Working cluster DNS

### Step-by-Step Deployment

#### 1. Create Namespace
```bash
kubectl create namespace donelist
kubectl create namespace monitoring
```

#### 2. Deploy Prometheus Stack
```bash
# Install kube-prometheus-stack via Helm
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --set prometheus.prometheusSpec.retention=30d \
  --set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage=50Gi \
  --set grafana.adminPassword=changeme
```

#### 3. Deploy Application with Metrics
```bash
# Deploy API with metrics endpoint
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/service.yaml
kubectl apply -f deploy/k8s/configmap.yaml
```

#### 4. Deploy ServiceMonitor
```bash
# Tell Prometheus to scrape our API
kubectl apply -f deploy/k8s/monitoring.yaml
```

#### 5. Deploy Alert Rules
```bash
# Deploy alert rules
kubectl apply -f deploy/k8s/monitoring.yaml
```

#### 6. Deploy AlertManager Configuration
```bash
# Configure alerting
kubectl create secret generic alertmanager-slack-url \
  --from-literal=url='https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK' \
  --namespace monitoring

kubectl apply -f deploy/k8s/alertmanager-config.yaml
```

#### 7. Deploy Logging Stack
```bash
# Deploy Fluent Bit
kubectl apply -f deploy/k8s/logging-stack.yaml

# Optional: Deploy Loki
helm install loki grafana/loki-stack \
  --namespace monitoring \
  --set promtail.enabled=false
```

#### 8. Deploy OpenTelemetry Collector
```bash
kubectl apply -f deploy/k8s/opentelemetry.yaml
```

#### 9. Deploy Jaeger (for Tracing)
```bash
helm install jaeger jaegertracing/jaeger \
  --namespace monitoring \
  --set collector.service.otlp.grpc.name=otlp-grpc \
  --set collector.service.otlp.http.name=otlp-http
```

#### 10. Deploy Grafana Dashboards
```bash
kubectl apply -f deploy/k8s/grafana-dashboards.yaml
```

#### 11. Verify Deployment
```bash
# Check all pods are running
kubectl get pods -n donelist
kubectl get pods -n monitoring

# Check metrics endpoint
kubectl port-forward -n donelist svc/donelist-api 8080:8080
curl http://localhost:8080/metrics

# Access Grafana
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
# Open http://localhost:3000
# Login: admin / changeme

# Check Prometheus targets
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
# Open http://localhost:9090/targets
```

### Configuration Updates

**Update Slack Webhook**:
```bash
kubectl edit configmap alertmanager-config -n monitoring
# Update slack_api_url value
```

**Update Grafana Admin Password**:
```bash
kubectl exec -it -n monitoring deployment/prometheus-grafana -- grafana-cli admin reset-admin-password newpassword
```

**Update Metric Retention**:
```bash
kubectl edit prometheus -n monitoring prometheus-kube-prometheus-prometheus
# Update retention: 30d
```

---

## Troubleshooting

### Common Issues

#### 1. Metrics Not Appearing in Prometheus

**Symptoms**: Targets showing as down, no data in Grafana

**Diagnosis**:
```bash
# Check ServiceMonitor
kubectl get servicemonitor -n donelist

# Check Prometheus targets
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
# Visit http://localhost:9090/targets

# Check API metrics endpoint
kubectl exec -n donelist deployment/donelist-api -- wget -O- http://localhost:8080/metrics
```

**Solutions**:
- Verify ServiceMonitor labels match Prometheus selector
- Check network policies allow scraping
- Verify metrics port is exposed in Service
- Check Prometheus logs for scrape errors

#### 2. Alerts Not Firing

**Symptoms**: Conditions met but no notifications

**Diagnosis**:
```bash
# Check Prometheus alerts
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
# Visit http://localhost:9090/alerts

# Check AlertManager
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-alertmanager 9093:9093
# Visit http://localhost:9093

# Check AlertManager logs
kubectl logs -n monitoring alertmanager-prometheus-kube-prometheus-alertmanager-0
```

**Solutions**:
- Verify alert rules are loaded
- Check AlertManager configuration
- Test Slack webhook manually
- Check inhibition rules aren't suppressing

#### 3. Traces Not Appearing in Jaeger

**Symptoms**: No traces in Jaeger UI

**Diagnosis**:
```bash
# Check OTEL Collector
kubectl logs -n donelist deployment/otel-collector

# Check application is exporting
kubectl logs -n donelist deployment/donelist-api | grep -i trace

# Check Jaeger collector
kubectl logs -n monitoring deployment/jaeger-collector
```

**Solutions**:
- Verify OTEL_EXPORTER_OTLP_ENDPOINT is set correctly
- Check sampling rate (might be too low)
- Verify OTEL Collector can reach Jaeger
- Check for errors in application logs

#### 4. High Cardinality Issues

**Symptoms**: Prometheus using too much memory/CPU

**Diagnosis**:
```bash
# Check metric cardinality
kubectl exec -n monitoring prometheus-kube-prometheus-prometheus-0 -- \
  promtool tsdb analyze /prometheus

# Check series count
curl -s http://prometheus:9090/api/v1/label/__name__/values | jq '.data | length'
```

**Solutions**:
- Remove high-cardinality labels (user IDs, trace IDs)
- Use recording rules to pre-aggregate
- Increase Prometheus resources
- Implement metric relabeling

#### 5. Dashboard Not Loading

**Symptoms**: Grafana dashboard shows "No data"

**Diagnosis**:
```bash
# Check Grafana logs
kubectl logs -n monitoring deployment/prometheus-grafana

# Test Prometheus datasource
# In Grafana: Configuration → Data Sources → Prometheus → Test

# Test query manually
curl 'http://prometheus:9090/api/v1/query?query=up'
```

**Solutions**:
- Verify datasource configuration
- Check time range (data might be outside range)
- Verify metric names are correct
- Check Prometheus has scraped data

---

## Performance Considerations

### Metrics Optimization

1. **Reduce Label Cardinality**:
   ```go
   // Bad - unbounded labels
   metric.WithLabelValues(userID, traceID, timestamp)

   // Good - bounded labels
   metric.WithLabelValues(endpoint, method, statusClass)
   ```

2. **Use Recording Rules**:
   ```yaml
   # Pre-aggregate expensive queries
   - record: job:http_requests:rate5m
     expr: sum(rate(http_requests_total[5m])) by (job)
   ```

3. **Appropriate Histogram Buckets**:
   ```go
   // Optimize buckets for your data
   Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2.5, 5, 10}
   ```

### Storage Optimization

1. **Prometheus Retention**:
   ```yaml
   # Balance between cost and data retention
   retention: 30d  # Adjust based on needs
   ```

2. **Compaction**:
   ```yaml
   # Prometheus auto-compacts, ensure sufficient disk space
   # ~1-2GB per million samples
   ```

3. **Remote Write** (for long-term storage):
   ```yaml
   remote_write:
   - url: https://cortex.example.com/api/prom/push
     queue_config:
       capacity: 10000
       max_shards: 10
   ```

---

## Security Considerations

1. **Metrics Endpoint Security**:
   ```yaml
   # Use NetworkPolicy to restrict access
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-prometheus
   spec:
     podSelector:
       matchLabels:
         app: donelist
     ingress:
     - from:
       - namespaceSelector:
           matchLabels:
             name: monitoring
       ports:
       - port: 9090
   ```

2. **Grafana Authentication**:
   - Enable OAuth/LDAP
   - Use strong passwords
   - Implement RBAC
   - Enable audit logging

3. **Alert Notification Security**:
   - Use secrets for webhooks
   - Encrypt sensitive data
   - Limit alert content (no PII)

---

## Monitoring Coverage Summary

### Task Completion Status

| Task | Status | Details |
|------|--------|---------|
| 17.1 | ✅ Done | Health endpoints implemented |
| 17.2 | ✅ Done | Response time percentiles (p50, p95, p99) |
| 17.3 | ✅ Done | Business metrics (checkins/min, active users, etc.) |
| 17.4 | ✅ Done | Log aggregation (Fluent Bit → Loki/ES) |
| 17.5 | ✅ Done | Distributed tracing (OpenTelemetry → Jaeger/Tempo) |
| 17.6 | ✅ Done | Alert rules with thresholds |
| 17.7 | ✅ Done | Grafana dashboards (4 dashboards) |
| 17.8 | ✅ Done | Runbooks and incident response procedures |

### Metrics Coverage

**Infrastructure Metrics**:
- ✅ CPU, memory, disk usage
- ✅ Network I/O
- ✅ Pod health and restarts
- ✅ Node resources

**Application Metrics**:
- ✅ HTTP requests (rate, duration, errors)
- ✅ Response time percentiles
- ✅ In-flight requests
- ✅ Request/response sizes

**Database Metrics**:
- ✅ Connection pool status
- ✅ Query duration
- ✅ Query errors
- ✅ Slow queries

**Cache Metrics**:
- ✅ Hit/miss rates
- ✅ Operation duration
- ✅ Connection status

**Business Metrics**:
- ✅ Active users (DAU/WAU/MAU)
- ✅ Check-in rates
- ✅ Feature usage
- ✅ Webhook deliveries
- ✅ Premium adoption

### Alert Coverage

**Critical Alerts**: 6 rules
- Error rate, database failures, pod crashes, deployment issues

**Warning Alerts**: 8 rules
- Performance degradation, resource pressure, connectivity issues

**Notification Channels**:
- ✅ Slack (multiple channels)
- ✅ PagerDuty (critical only)
- ✅ Email (optional)

### Documentation

- ✅ Runbooks for all critical alerts
- ✅ Investigation procedures
- ✅ Mitigation strategies
- ✅ Escalation paths
- ✅ Post-incident review template

---

## Next Steps and Recommendations

### Short Term (Week 1-2)

1. **Deploy to Staging**:
   ```bash
   # Test full monitoring stack
   kubectl apply -f deploy/k8s/ --namespace=staging
   ```

2. **Validate Alerts**:
   - Trigger test alerts
   - Verify notifications received
   - Test runbook procedures

3. **Team Training**:
   - Dashboard walkthrough
   - Alert response training
   - Runbook review

### Medium Term (Month 1-2)

1. **SLO Refinement**:
   - Review initial SLO targets
   - Adjust based on actual performance
   - Implement SLO-based alerting

2. **Capacity Planning**:
   - Analyze resource trends
   - Plan scaling thresholds
   - Implement predictive alerting

3. **Cost Optimization**:
   - Review metric retention
   - Optimize storage
   - Tune sampling rates

### Long Term (Quarter 1-2)

1. **Advanced Features**:
   - Implement anomaly detection
   - Add predictive alerting (ML-based)
   - Synthetic monitoring

2. **Integration Enhancements**:
   - Auto-remediation for common issues
   - ChatOps integration
   - Incident management platform integration

3. **Continuous Improvement**:
   - Monthly metrics review
   - Quarterly runbook updates
   - Regular disaster recovery drills

---

## References

- [MONITORING_RUNBOOKS.md](./MONITORING_RUNBOOKS.md) - Incident response procedures
- [HEALTH_ENDPOINTS.md](./HEALTH_ENDPOINTS.md) - Health check implementation
- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [Fluent Bit Documentation](https://docs.fluentbit.io/)

---

**Document Version**: 1.0
**Last Updated**: 2024-01-15
**Maintained By**: Platform Team
**Review Frequency**: Quarterly
