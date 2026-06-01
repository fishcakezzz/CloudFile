package model

import "time"

type UploadStatus string
type ChunkStatus string
type FileStatus string
type MediaTaskStatus string
type MediaTaskType string

const (
	UploadInit      UploadStatus = "INIT"
	UploadUploading UploadStatus = "UPLOADING"
	UploadMerging   UploadStatus = "MERGING"
	UploadSuccess   UploadStatus = "SUCCESS"
	UploadFailed    UploadStatus = "FAILED"
	UploadCanceled  UploadStatus = "CANCELED"

	ChunkPending  ChunkStatus = "PENDING"
	ChunkUploaded ChunkStatus = "UPLOADED"
	ChunkFailed   ChunkStatus = "FAILED"

	FileSuccess FileStatus = "SUCCESS"
	FileFailed  FileStatus = "FAILED"

	MediaPending    MediaTaskStatus = "PENDING"
	MediaProcessing MediaTaskStatus = "PROCESSING"
	MediaSuccess    MediaTaskStatus = "SUCCESS"
	MediaFailed     MediaTaskStatus = "FAILED"
	MediaDead       MediaTaskStatus = "DEAD"

	ImageCompress MediaTaskType = "IMAGE_COMPRESS"
	VideoTranscode MediaTaskType = "VIDEO_TRANSCODE"
	VideoThumbnail MediaTaskType = "VIDEO_THUMBNAIL"
	MediaMetadata  MediaTaskType = "MEDIA_METADATA"
)

type UploadTask struct {
	ID            uint         `gorm:"primaryKey" json:"id"`
	UserID        string       `gorm:"size:64;index;not null" json:"user_id"`
	FileName      string       `gorm:"size:255;not null" json:"file_name"`
	FileHash      string       `gorm:"size:128;index;not null" json:"file_hash"`
	FileSize      int64        `gorm:"not null" json:"file_size"`
	ChunkSize     int64        `gorm:"not null" json:"chunk_size"`
	ChunkCount    int          `gorm:"not null" json:"chunk_count"`
	UploadedCount int          `gorm:"not null;default:0" json:"uploaded_count"`
	ContentType   string       `gorm:"size:128" json:"content_type"`
	Status        UploadStatus `gorm:"size:32;index;not null" json:"status"`
	ObjectKey     string       `gorm:"size:512" json:"object_key"`
	ErrorMsg      string       `gorm:"type:text" json:"error_msg"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

type UploadChunk struct {
	ID         uint        `gorm:"primaryKey" json:"id"`
	UploadID   uint        `gorm:"uniqueIndex:uq_upload_chunk;index;not null" json:"upload_id"`
	ChunkIndex int         `gorm:"uniqueIndex:uq_upload_chunk;not null" json:"chunk_index"`
	ChunkHash  string      `gorm:"size:128" json:"chunk_hash"`
	ChunkSize  int64       `gorm:"not null" json:"chunk_size"`
	ObjectKey  string      `gorm:"size:512;not null" json:"object_key"`
	Status     ChunkStatus `gorm:"size:32;index;not null" json:"status"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

type FileMeta struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	FileHash      string     `gorm:"size:128;uniqueIndex:uq_file_hash_size;not null" json:"file_hash"`
	FileSize      int64      `gorm:"uniqueIndex:uq_file_hash_size;not null" json:"file_size"`
	ObjectKey     string     `gorm:"size:512;not null" json:"object_key"`
	ContentType   string     `gorm:"size:128" json:"content_type"`
	StorageBucket string     `gorm:"size:128;not null" json:"storage_bucket"`
	Status        FileStatus `gorm:"size:32;index;not null" json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type UserFile struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    string    `gorm:"size:64;index;not null" json:"user_id"`
	FileID    uint      `gorm:"index;not null" json:"file_id"`
	FileName  string    `gorm:"size:255;not null" json:"file_name"`
	ParentID  *uint     `json:"parent_id"`
	File      FileMeta  `gorm:"foreignKey:FileID" json:"file"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MediaTask struct {
	ID              uint            `gorm:"primaryKey" json:"id"`
	FileID          uint            `gorm:"index;not null" json:"file_id"`
	TaskType        MediaTaskType   `gorm:"size:64;not null" json:"task_type"`
	InputObjectKey  string          `gorm:"size:512;not null" json:"input_object_key"`
	OutputObjectKey string          `gorm:"size:512;not null" json:"output_object_key"`
	Status          MediaTaskStatus `gorm:"size:32;index;not null" json:"status"`
	RetryCount      int             `gorm:"not null;default:0" json:"retry_count"`
	MaxRetry        int             `gorm:"not null;default:3" json:"max_retry"`
	ErrorMsg        string          `gorm:"type:text" json:"error_msg"`
	File            FileMeta        `gorm:"foreignKey:FileID" json:"file"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}
