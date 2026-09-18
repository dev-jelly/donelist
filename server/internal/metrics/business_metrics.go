package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BusinessMetrics holds domain-specific business metrics for Donelist
type BusinessMetrics struct {
	// User engagement metrics
	UserSignups              *prometheus.CounterVec
	UserLogins               *prometheus.CounterVec
	UserActiveDailyGauge     prometheus.Gauge
	UserActiveWeeklyGauge    prometheus.Gauge
	UserActiveMonthlyGauge   prometheus.Gauge

	// Content metrics
	CheckinCreations         *prometheus.CounterVec
	CheckinUpdates           *prometheus.CounterVec
	CheckinDeletions         *prometheus.CounterVec
	CheckinDurationHistogram *prometheus.HistogramVec

	// Category metrics
	CategoryCreations        *prometheus.CounterVec
	CategoriesPerUser        *prometheus.HistogramVec

	// Timeline metrics
	TimelineViews            *prometheus.CounterVec
	TimelineLoadDuration     *prometheus.HistogramVec

	// Search metrics
	SearchQueries            *prometheus.CounterVec
	SearchResultCount        *prometheus.HistogramVec
	SearchDuration           *prometheus.HistogramVec

	// Team metrics
	TeamCreations            *prometheus.CounterVec
	TeamMemberAdds           *prometheus.CounterVec
	TeamMemberRemoves        *prometheus.CounterVec
	TeamsPerUser             *prometheus.HistogramVec

	// Webhook metrics (extended from webhook package)
	WebhookDeliveries        *prometheus.CounterVec
	WebhookFailures          *prometheus.CounterVec
	WebhookRetries           *prometheus.CounterVec

	// Sync metrics
	SyncOperations           *prometheus.CounterVec
	SyncConflicts            *prometheus.CounterVec
	SyncDuration             *prometheus.HistogramVec

	// Premium features
	PremiumUpgrades          *prometheus.CounterVec
	PremiumFeatureUsage      *prometheus.CounterVec

	// API Key usage
	APIKeyCreations          *prometheus.CounterVec
	APIKeyUsage              *prometheus.CounterVec
	APIKeyRevocations        *prometheus.CounterVec
}

// NewBusinessMetrics creates and registers all business-specific metrics
func NewBusinessMetrics() *BusinessMetrics {
	return &BusinessMetrics{
		// User engagement metrics
		UserSignups: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_user_signups_total",
				Help: "Total number of user signups",
			},
			[]string{"method"}, // oauth, email, etc.
		),
		UserLogins: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_user_logins_total",
				Help: "Total number of user logins",
			},
			[]string{"method", "success"},
		),
		UserActiveDailyGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "donelist_users_active_daily",
				Help: "Number of daily active users",
			},
		),
		UserActiveWeeklyGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "donelist_users_active_weekly",
				Help: "Number of weekly active users",
			},
		),
		UserActiveMonthlyGauge: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "donelist_users_active_monthly",
				Help: "Number of monthly active users",
			},
		),

		// Content metrics
		CheckinCreations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_checkin_creations_total",
				Help: "Total number of check-ins created",
			},
			[]string{"category_type", "has_tags"},
		),
		CheckinUpdates: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_checkin_updates_total",
				Help: "Total number of check-in updates",
			},
			[]string{"update_type"}, // content, category, tags, etc.
		),
		CheckinDeletions: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_checkin_deletions_total",
				Help: "Total number of check-in deletions",
			},
			[]string{"soft_delete"},
		),
		CheckinDurationHistogram: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "donelist_checkin_duration_minutes",
				Help:    "Duration between check-in creation and completion in minutes",
				Buckets: []float64{5, 15, 30, 60, 120, 240, 480, 1440}, // 5m to 24h
			},
			[]string{"category_type"},
		),

		// Category metrics
		CategoryCreations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_category_creations_total",
				Help: "Total number of categories created",
			},
			[]string{"icon_type"},
		),
		CategoriesPerUser: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "donelist_categories_per_user",
				Help:    "Distribution of categories per user",
				Buckets: []float64{1, 3, 5, 10, 15, 20, 30, 50},
			},
			[]string{},
		),

		// Timeline metrics
		TimelineViews: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_timeline_views_total",
				Help: "Total number of timeline views",
			},
			[]string{"view_type", "cache_hit"}, // daily, weekly, monthly
		),
		TimelineLoadDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "donelist_timeline_load_duration_seconds",
				Help:    "Timeline load duration in seconds",
				Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
			},
			[]string{"view_type", "cache_hit"},
		),

		// Search metrics
		SearchQueries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_search_queries_total",
				Help: "Total number of search queries",
			},
			[]string{"query_type"}, // full_text, tag, category
		),
		SearchResultCount: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "donelist_search_results_count",
				Help:    "Distribution of search result counts",
				Buckets: []float64{0, 1, 5, 10, 25, 50, 100, 250},
			},
			[]string{"query_type"},
		),
		SearchDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "donelist_search_duration_seconds",
				Help:    "Search query duration in seconds",
				Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
			},
			[]string{"query_type"},
		),

		// Team metrics
		TeamCreations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_team_creations_total",
				Help: "Total number of teams created",
			},
			[]string{},
		),
		TeamMemberAdds: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_team_member_adds_total",
				Help: "Total number of team member additions",
			},
			[]string{"role"},
		),
		TeamMemberRemoves: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_team_member_removes_total",
				Help: "Total number of team member removals",
			},
			[]string{"reason"}, // left, removed, etc.
		),
		TeamsPerUser: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "donelist_teams_per_user",
				Help:    "Distribution of teams per user",
				Buckets: []float64{0, 1, 2, 3, 5, 10},
			},
			[]string{},
		),

		// Webhook metrics
		WebhookDeliveries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_webhook_deliveries_total",
				Help: "Total number of webhook delivery attempts",
			},
			[]string{"event_type", "status"},
		),
		WebhookFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_webhook_failures_total",
				Help: "Total number of webhook delivery failures",
			},
			[]string{"event_type", "failure_reason"},
		),
		WebhookRetries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_webhook_retries_total",
				Help: "Total number of webhook retry attempts",
			},
			[]string{"event_type", "retry_count"},
		),

		// Sync metrics
		SyncOperations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_sync_operations_total",
				Help: "Total number of sync operations",
			},
			[]string{"operation_type", "status"},
		),
		SyncConflicts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_sync_conflicts_total",
				Help: "Total number of sync conflicts",
			},
			[]string{"conflict_type", "resolution"},
		),
		SyncDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "donelist_sync_duration_seconds",
				Help:    "Sync operation duration in seconds",
				Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"operation_type"},
		),

		// Premium features
		PremiumUpgrades: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_premium_upgrades_total",
				Help: "Total number of premium upgrades",
			},
			[]string{"plan_type"},
		),
		PremiumFeatureUsage: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_premium_feature_usage_total",
				Help: "Total usage of premium features",
			},
			[]string{"feature_name"},
		),

		// API Key usage
		APIKeyCreations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_apikey_creations_total",
				Help: "Total number of API keys created",
			},
			[]string{"key_type"},
		),
		APIKeyUsage: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_apikey_usage_total",
				Help: "Total number of API key uses",
			},
			[]string{"key_id", "endpoint"},
		),
		APIKeyRevocations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "donelist_apikey_revocations_total",
				Help: "Total number of API key revocations",
			},
			[]string{"reason"},
		),
	}
}
