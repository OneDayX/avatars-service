package observability

import (
	"context"
	"testing"

	"github.com/example/gophprofile/internal/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestSetupTracingDisabled(t *testing.T) {
	shutdown, err := SetupTracing(context.Background(), config.Config{OTelEnabled: false})
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(context.Background(), carrier)
	require.NotNil(t, carrier)
}
