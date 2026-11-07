package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	ProcessingPending    = "pending"
	ProcessingProcessing = "processing"
	ProcessingCompleted  = "completed"
	ProcessingFailed     = "failed"
)

type UserStorageUsage struct {
	UserID string
	Bytes  int64
}

type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Thumbnail struct {
	Size string `json:"size"`
	URL  string `json:"url"`
}

type Avatar struct {
	ID               uuid.UUID         `json:"id"`
	UserID           string            `json:"user_id"`
	FileName         string            `json:"file_name"`
	MIMEType         string            `json:"mime_type"`
	SizeBytes        int64             `json:"size"`
	Dimensions       Dimensions        `json:"dimensions"`
	S3Key            string            `json:"-"`
	ThumbnailS3Keys  map[string]string `json:"-"`
	UploadStatus     string            `json:"-"`
	ProcessingStatus string            `json:"status"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	DeletedAt        *time.Time        `json:"-"`
}

type UploadResponse struct {
	ID        uuid.UUID `json:"id"`
	UserID    string    `json:"user_id"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type MetadataResponse struct {
	ID         uuid.UUID   `json:"id"`
	UserID     string      `json:"user_id"`
	FileName   string      `json:"file_name"`
	MIMEType   string      `json:"mime_type"`
	Size       int64       `json:"size"`
	Dimensions Dimensions  `json:"dimensions"`
	Thumbnails []Thumbnail `json:"thumbnails"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

func (a *Avatar) URL() string {
	return "/api/v1/avatars/" + a.ID.String()
}

func (a *Avatar) UploadResponse() UploadResponse {
	status := a.ProcessingStatus
	if status == ProcessingPending {
		status = ProcessingProcessing
	}
	return UploadResponse{
		ID: a.ID, UserID: a.UserID, URL: a.URL(), Status: status, CreatedAt: a.CreatedAt,
	}
}

func (a *Avatar) MetadataResponse() MetadataResponse {
	thumbnails := make([]Thumbnail, 0, len(a.ThumbnailS3Keys))
	for _, size := range []string{"100x100", "300x300"} {
		if _, ok := a.ThumbnailS3Keys[size]; ok {
			thumbnails = append(thumbnails, Thumbnail{Size: size, URL: a.URL() + "?size=" + size})
		}
	}
	return MetadataResponse{
		ID: a.ID, UserID: a.UserID, FileName: a.FileName, MIMEType: a.MIMEType,
		Size: a.SizeBytes, Dimensions: a.Dimensions, Thumbnails: thumbnails,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}
