package minio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	storage, err := New("localhost:9000", "key", "secret", "avatars", false)
	require.NoError(t, err)
	require.NotNil(t, storage)
	require.Equal(t, "avatars", storage.bucket)
}

func TestNewRejectsInvalidEndpoint(t *testing.T) {
	_, err := New("http://bad url", "key", "secret", "avatars", false)
	require.Error(t, err)
}
