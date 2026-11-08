package observability

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type usageRepositoryStub struct{}

func (usageRepositoryStub) StorageUsage(context.Context) (int64, error) {
	return 2048, nil
}

type queueStub struct{}

func (queueStub) QueueDepth(context.Context) (int, error) { return 7, nil }

func TestStorageUsageCollector(t *testing.T) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewStorageUsageCollector(usageRepositoryStub{}))
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Equal(t, "avatars_storage_bytes", families[0].GetName())
	require.Len(t, families[0].Metric, 1)
	require.Empty(t, families[0].Metric[0].Label)
	require.Equal(t, float64(2048), families[0].Metric[0].Gauge.GetValue())
}

func TestInfrastructureCollector(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/test")
	require.NoError(t, err)
	defer pool.Close()
	registry := prometheus.NewRegistry()
	registry.MustRegister(NewInfrastructureCollector(pool, queueStub{}))
	families, err := registry.Gather()
	require.NoError(t, err)
	values := make(map[string]float64)
	for _, family := range families {
		for _, metric := range family.Metric {
			values[family.GetName()] = metric.Gauge.GetValue()
		}
	}
	require.Equal(t, float64(7), values["avatars_queue_messages"])
	require.Equal(t, float64(1), values["avatars_queue_up"])
	require.Contains(t, values, "avatars_db_connections")
}
