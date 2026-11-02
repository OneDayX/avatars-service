package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAvatarResponses(t *testing.T) {
	id := uuid.New()
	now := time.Now().UTC()
	avatar := Avatar{ID: id, UserID: "user", FileName: "me.png", MIMEType: "image/png", SizeBytes: 42,
		Dimensions: Dimensions{Width: 20, Height: 10}, ProcessingStatus: ProcessingCompleted,
		ThumbnailS3Keys: map[string]string{"300x300": "300", "100x100": "100"}, CreatedAt: now, UpdatedAt: now}
	require.Equal(t, "/api/v1/avatars/"+id.String(), avatar.URL())
	upload := avatar.UploadResponse()
	require.Equal(t, id, upload.ID)
	require.Equal(t, ProcessingCompleted, upload.Status)
	metadata := avatar.MetadataResponse()
	require.Len(t, metadata.Thumbnails, 2)
	require.Equal(t, "100x100", metadata.Thumbnails[0].Size)
	require.Contains(t, metadata.Thumbnails[0].URL, "size=100x100")
	require.Equal(t, int64(42), metadata.Size)
	avatar.ProcessingStatus = ProcessingPending
	require.Equal(t, ProcessingProcessing, avatar.UploadResponse().Status)
}
