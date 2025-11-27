package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Label value constants to prevent typos
const (
	// Queue types
	QueueTypeWebhook = "webhook"
	QueueTypeSyncJob = "sync_job"

	// Queue results
	ResultSuccess = "success"
	ResultRetry   = "retry"
	ResultDropped = "dropped"
	ResultFailure = "failure"

	// Worker outcomes
	OutcomeWebhookFound = "webhook_found"
	OutcomeSyncJobFound = "sync_job_found"
	OutcomeIdle         = "idle"

	// HTTP endpoints
	EndpointOAuthStart    = "oauth_start"
	EndpointOAuthCallback = "oauth_callback"
	EndpointWebhook       = "webhook_callback"
	EndpointEvents        = "events"
	EndpointHealth        = "health"

	// Strava API operations
	OpExchangeCode       = "exchange_code"
	OpRefreshToken       = "refresh_token"
	OpGetActivity        = "get_activity"
	OpListActivities     = "list_activities"
	OpCreateSubscription = "create_subscription"
	OpDeleteSubscription = "delete_subscription"
	OpListSubscriptions  = "list_subscriptions"

	// Rate limit types
	RateLimitOverall15Min = "overall_15min"
	RateLimitOverallDaily = "overall_daily"
	RateLimitRead15Min    = "read_15min"
	RateLimitReadDaily    = "read_daily"

	// Rate limit buckets
	BucketLimit = "limit"
	BucketUsage = "usage"

	// Database operations
	DBOpEnqueueWebhook             = "enqueue_webhook"
	DBOpClaimWebhook               = "claim_webhook"
	DBOpDeleteWebhook              = "delete_webhook"
	DBOpReleaseWebhook             = "release_webhook"
	DBOpGetQueueLength             = "get_queue_length"
	DBOpGetReadyQueueLength        = "get_ready_queue_length"
	DBOpGetProcessingQueueLength   = "get_processing_queue_length"
	DBOpEnqueueSyncJob             = "enqueue_sync_job"
	DBOpClaimSyncJob               = "claim_sync_job"
	DBOpDeleteSyncJob              = "delete_sync_job"
	DBOpReleaseSyncJob             = "release_sync_job"
	DBOpGetSyncJobQueueLength      = "get_sync_job_queue_length"
	DBOpGetReadySyncJobQueueLength = "get_ready_sync_job_queue_length"
	DBOpInsertActivityEvent        = "insert_activity_event"
	DBOpGetEvents                  = "get_events"
	DBOpDeleteAthleteEvents        = "delete_athlete_events"
	DBOpGetAthlete                 = "get_athlete"
	DBOpUpsertAthlete              = "upsert_athlete"
	DBOpGetCircuitBreakerState     = "get_circuit_breaker_state"
	DBOpOpenCircuitBreaker         = "open_circuit_breaker"
	DBOpTransitionCircuitBreaker   = "transition_circuit_breaker"
)

// HTTP Metrics
var (
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"endpoint", "status_code"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"endpoint", "status_code"},
	)
)

// Queue Metrics
var (
	QueueDepthTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_depth_total",
			Help: "Total number of items in queue (all states)",
		},
		[]string{"queue_type"},
	)

	QueueDepthReady = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_depth_ready",
			Help: "Number of items ready for processing",
		},
		[]string{"queue_type"},
	)

	QueueDepthProcessing = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "queue_depth_processing",
			Help: "Number of items currently being processed",
		},
		[]string{"queue_type"},
	)

	QueueEnqueueTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "queue_enqueue_total",
			Help: "Total number of items enqueued",
		},
		[]string{"queue_type"},
	)

	QueueDequeueTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "queue_dequeue_total",
			Help: "Total number of items dequeued with outcome",
		},
		[]string{"queue_type", "result"},
	)

	QueueProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "queue_processing_duration_seconds",
			Help:    "Time spent processing queue items",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
		},
		[]string{"queue_type", "result"},
	)

	QueueItemAge = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "queue_item_age_seconds",
			Help:    "Time from enqueue to processing start",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 600, 1800, 3600, 7200},
		},
		[]string{"queue_type"},
	)

	QueueRetryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "queue_retry_total",
			Help: "Total number of retry attempts",
		},
		[]string{"queue_type", "retry_count"},
	)
)

// Worker Metrics
var (
	WorkerPollCyclesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "worker_poll_cycles_total",
			Help: "Total number of worker poll cycles by outcome",
		},
		[]string{"outcome"},
	)

	WorkerActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "worker_active",
			Help: "Whether the worker is currently active (1) or not (0)",
		},
	)
)

// Strava API Metrics
var (
	StravaAPIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "strava_api_requests_total",
			Help: "Total number of Strava API requests",
		},
		[]string{"operation", "status_code"},
	)

	StravaAPIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "strava_api_request_duration_seconds",
			Help:    "Strava API request latency in seconds",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"operation", "status_code"},
	)

	StravaRateLimitUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "strava_rate_limit_usage",
			Help: "Strava API rate limit usage",
		},
		[]string{"limit_type", "bucket"},
	)
)

// Database Metrics
var (
	DBOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_operation_duration_seconds",
			Help:    "Database operation latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"operation"},
	)

	DBOperationErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "db_operation_errors_total",
			Help: "Total number of database operation errors",
		},
		[]string{"operation"},
	)
)

// Business Metrics
var (
	WebhookEventsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "webhook_events_processed_total",
			Help: "Total number of webhook events processed",
		},
		[]string{"object_type", "aspect_type"},
	)

	SyncJobsCompletedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sync_jobs_completed_total",
			Help: "Total number of sync jobs completed",
		},
		[]string{"job_type"},
	)

	SyncAllActivitiesCount = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "sync_all_activities_count",
			Help:    "Number of activities synced per sync_all_activities job",
			Buckets: []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000},
		},
	)
)

// Circuit Breaker Metrics
var (
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=half_open, 2=open)",
		},
		[]string{"breaker_type"},
	)

	CircuitBreakerOpened = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "circuit_breaker_opened_total",
			Help: "Total number of times circuit breaker opened due to rate limits",
		},
	)

	CircuitBreakerRecovered = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "circuit_breaker_recovered_total",
			Help: "Total number of times circuit breaker recovered to closed state",
		},
	)

	BackfillJobsThrottled = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "backfill_jobs_throttled_total",
			Help: "Total number of backfill jobs skipped due to proactive throttling",
		},
	)

	RateLimitBudgetAvailable = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rate_limit_budget_available",
			Help: "Available rate limit budget after webhook reserve",
		},
		[]string{"limit_type"},
	)
)
