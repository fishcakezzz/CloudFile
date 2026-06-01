package lock

import (
	"context"
	"sync"
	"time"

	"cloudfile/backend/internal/config"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

type Lock interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error)
	Release(ctx context.Context, key, value string) error
}

func New(cfg config.Config) Lock {
	if cfg.RedisURL != "" {
		opt, err := redis.ParseURL(cfg.RedisURL)
		if err == nil {
			client := redis.NewClient(opt)
			if client.Ping(context.Background()).Err() == nil {
				return &RedisLock{client: client}
			}
		}
	}
	return &MemoryLock{locks: map[string]string{}}
}

type RedisLock struct {
	client *redis.Client
}

func (l *RedisLock) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	value := uuid.NewString()
	ok, err := l.client.SetNX(ctx, key, value, ttl).Result()
	return value, ok, err
}

func (l *RedisLock) Release(ctx context.Context, key, value string) error {
	script := redis.NewScript(`if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`)
	return script.Run(ctx, l.client, []string{key}, value).Err()
}

type MemoryLock struct {
	mu    sync.Mutex
	locks map[string]string
}

func (l *MemoryLock) Acquire(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.locks[key]; exists {
		return "", false, nil
	}
	value := uuid.NewString()
	l.locks[key] = value
	return value, true, nil
}

func (l *MemoryLock) Release(ctx context.Context, key, value string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locks[key] == value {
		delete(l.locks, key)
	}
	return nil
}
