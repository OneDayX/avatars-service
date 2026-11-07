package observability

import (
	"context"
	"time"

	"github.com/example/gophprofile/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type UsageRepository interface {
	StorageUsage(ctx context.Context) ([]domain.UserStorageUsage, error)
}

type Queue interface {
	QueueDepth(ctx context.Context) (int, error)
}

type StorageUsageCollector struct {
	repo UsageRepository
	desc *prometheus.Desc
}

func NewStorageUsageCollector(repo UsageRepository) *StorageUsageCollector {
	return &StorageUsageCollector{repo: repo, desc: prometheus.NewDesc("avatars_storage_bytes", "Storage used by active avatar originals", []string{"user_id"}, nil)}
}

func (c *StorageUsageCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *StorageUsageCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	usage, err := c.repo.StorageUsage(ctx)
	if err != nil {
		return
	}
	for _, item := range usage {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(item.Bytes), item.UserID)
	}
}

type InfrastructureCollector struct {
	pool        *pgxpool.Pool
	queue       Queue
	connections *prometheus.Desc
	queueDepth  *prometheus.Desc
	queueUp     *prometheus.Desc
}

func NewInfrastructureCollector(pool *pgxpool.Pool, queue Queue) *InfrastructureCollector {
	return &InfrastructureCollector{
		pool: pool, queue: queue,
		connections: prometheus.NewDesc("avatars_db_connections", "PostgreSQL connection pool state", []string{"state"}, nil),
		queueDepth:  prometheus.NewDesc("avatars_queue_messages", "Number of messages ready in the processing queue", []string{"queue"}, nil),
		queueUp:     prometheus.NewDesc("avatars_queue_up", "Whether the processing queue is available", nil, nil),
	}
}

func (c *InfrastructureCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.connections
	ch <- c.queueDepth
	ch <- c.queueUp
}

func (c *InfrastructureCollector) Collect(ch chan<- prometheus.Metric) {
	stats := c.pool.Stat()
	for state, value := range map[string]int64{
		"acquired": int64(stats.AcquiredConns()), "idle": int64(stats.IdleConns()), "total": int64(stats.TotalConns()),
	} {
		ch <- prometheus.MustNewConstMetric(c.connections, prometheus.GaugeValue, float64(value), state)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	depth, err := c.queue.QueueDepth(ctx)
	available := float64(1)
	if err != nil {
		available = 0
		depth = 0
	}
	ch <- prometheus.MustNewConstMetric(c.queueUp, prometheus.GaugeValue, available)
	ch <- prometheus.MustNewConstMetric(c.queueDepth, prometheus.GaugeValue, float64(depth), "avatars.processing")
}
