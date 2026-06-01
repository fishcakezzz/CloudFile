package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr                 string
	DatabaseDriver           string
	DatabaseDSN              string
	StorageDriver            string
	LocalStorageRoot         string
	MinioEndpoint            string
	MinioAccessKey           string
	MinioSecretKey           string
	MinioBucket              string
	MinioSecure              bool
	RedisURL                 string
	QueueDriver              string
	RabbitMQURL              string
	MediaProcessor           string
	MaxMediaRetry            int
	MergeLockTTL             time.Duration
	ProcessingTimeout        time.Duration
	FFmpegTimeout            time.Duration
	RabbitRetryTTL           time.Duration
}

func Load() Config {
	return Config{
		HTTPAddr:          env("HTTP_ADDR", ":8000"),
		DatabaseDriver:    env("DATABASE_DRIVER", "sqlite"),
		DatabaseDSN:       env("DATABASE_DSN", "cloudfile-go.db"),
		StorageDriver:     env("STORAGE_DRIVER", "local"),
		LocalStorageRoot:  env("LOCAL_STORAGE_ROOT", "./object_storage"),
		MinioEndpoint:     env("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey:    env("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey:    env("MINIO_SECRET_KEY", "minioadmin"),
		MinioBucket:       env("MINIO_BUCKET", "cloudfile"),
		MinioSecure:       envBool("MINIO_SECURE", false),
		RedisURL:          env("REDIS_URL", ""),
		QueueDriver:       env("QUEUE_DRIVER", "inline"),
		RabbitMQURL:       env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		MediaProcessor:    env("MEDIA_PROCESSOR", "copy"),
		MaxMediaRetry:     envInt("MAX_MEDIA_RETRY", 3),
		MergeLockTTL:      time.Duration(envInt("MERGE_LOCK_TTL_SECONDS", 120)) * time.Second,
		ProcessingTimeout: time.Duration(envInt("PROCESSING_TIMEOUT_SECONDS", 600)) * time.Second,
		FFmpegTimeout:     time.Duration(envInt("FFMPEG_TIMEOUT_SECONDS", 300)) * time.Second,
		RabbitRetryTTL:    time.Duration(envInt("RABBIT_RETRY_TTL_SECONDS", 30)) * time.Second,
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(env(key, ""))
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := env(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
