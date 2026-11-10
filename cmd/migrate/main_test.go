package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunReportsConnectionPoolStage(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")
	t.Setenv("MAX_FILE_SIZE", "invalid")
	err := run()
	require.ErrorContains(t, err, "create PostgreSQL connection pool")
}
