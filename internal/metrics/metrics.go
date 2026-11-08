package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "avatars_http_requests_total",
		Help: "Total number of HTTP requests",
	}, []string{"method", "path", "status"})
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "avatars_http_request_duration_seconds",
		Help:    "HTTP request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})
	HTTPErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "avatars_http_errors_total",
		Help: "Total number of HTTP 5xx responses",
	}, []string{"method", "path"})
	UploadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "avatars_uploads_total",
		Help: "Total number of avatar uploads",
	}, []string{"status"})
	UploadDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "avatars_upload_duration_seconds",
		Help:    "Avatar upload duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"status"})
	WorkerEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "avatars_worker_events_total",
		Help: "Total number of processed broker events",
	}, []string{"type", "status"})
	WorkerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "avatars_worker_event_duration_seconds",
		Help:    "Broker event processing duration including retries",
		Buckets: prometheus.DefBuckets,
	}, []string{"type", "status"})
)
