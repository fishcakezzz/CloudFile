package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cloudfile/backend/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type ObjectInfo struct {
	Size int64
}

type ObjectStore interface {
	Bucket() string
	PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	StatObject(ctx context.Context, key string) (ObjectInfo, error)
	RemoveObject(ctx context.Context, key string) error
	RemovePrefix(ctx context.Context, prefix string) error
	ComposeObject(ctx context.Context, destination string, sources []string) error
	Exists(ctx context.Context, key string) bool
}

func New(cfg config.Config) (ObjectStore, error) {
	if cfg.StorageDriver == "minio" {
		return NewMinioStore(ctxBackground(), cfg)
	}
	return NewLocalStore(cfg.LocalStorageRoot, cfg.MinioBucket), nil
}

func ctxBackground() context.Context {
	return context.Background()
}

type LocalStore struct {
	root   string
	bucket string
}

func NewLocalStore(root, bucket string) *LocalStore {
	_ = os.MkdirAll(filepath.Join(root, bucket), 0o755)
	return &LocalStore{root: root, bucket: bucket}
}

func (s *LocalStore) Bucket() string { return s.bucket }

func (s *LocalStore) path(key string) (string, error) {
	base, err := filepath.Abs(filepath.Join(s.root, s.bucket))
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(base, key))
	if err != nil {
		return "", err
	}
	if path != base && !strings.HasPrefix(path, base+string(os.PathSeparator)) {
		return "", errors.New("object key escapes storage root")
	}
	return path, nil
}

func (s *LocalStore) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, reader)
	return err
}

func (s *LocalStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *LocalStore) StatObject(ctx context.Context, key string) (ObjectInfo, error) {
	path, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{Size: stat.Size()}, nil
}

func (s *LocalStore) RemoveObject(ctx context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *LocalStore) RemovePrefix(ctx context.Context, prefix string) error {
	path, err := s.path(prefix)
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func (s *LocalStore) ComposeObject(ctx context.Context, destination string, sources []string) error {
	path, err := s.path(destination)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for _, source := range sources {
		in, err := s.GetObject(ctx, source)
		if err != nil {
			out.Close()
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			in.Close()
			out.Close()
			return err
		}
		in.Close()
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *LocalStore) Exists(ctx context.Context, key string) bool {
	_, err := s.StatObject(ctx, key)
	return err == nil
}

type MinioStore struct {
	client *minio.Client
	bucket string
}

func NewMinioStore(ctx context.Context, cfg config.Config) (*MinioStore, error) {
	client, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioSecure,
	})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(ctx, cfg.MinioBucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.MinioBucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &MinioStore{client: client, bucket: cfg.MinioBucket}, nil
}

func (s *MinioStore) Bucket() string { return s.bucket }

func (s *MinioStore) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *MinioStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
}

func (s *MinioStore) StatObject(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{Size: info.Size}, nil
}

func (s *MinioStore) RemoveObject(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *MinioStore) RemovePrefix(ctx context.Context, prefix string) error {
	for item := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if item.Err != nil {
			return item.Err
		}
		if err := s.RemoveObject(ctx, item.Key); err != nil {
			return err
		}
	}
	return nil
}

func (s *MinioStore) ComposeObject(ctx context.Context, destination string, sources []string) error {
	items := make([]minio.CopySrcOptions, 0, len(sources))
	for _, source := range sources {
		items = append(items, minio.CopySrcOptions{Bucket: s.bucket, Object: source})
	}
	_, err := s.client.ComposeObject(ctx, minio.CopyDestOptions{Bucket: s.bucket, Object: destination}, items...)
	return err
}

func (s *MinioStore) Exists(ctx context.Context, key string) bool {
	_, err := s.StatObject(ctx, key)
	return err == nil
}

func PutBytes(ctx context.Context, store ObjectStore, key string, data []byte, contentType string) error {
	return store.PutObject(ctx, key, bytes.NewReader(data), int64(len(data)), contentType)
}
