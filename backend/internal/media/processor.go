package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"cloudfile/backend/internal/config"
	"cloudfile/backend/internal/model"
	"cloudfile/backend/internal/storage"
)

type Processor interface {
	Process(ctx context.Context, task model.MediaTask) error
}

func New(cfg config.Config, store storage.ObjectStore) Processor {
	if cfg.MediaProcessor == "ffmpeg" {
		return &FFmpegProcessor{store: store, timeout: cfg.FFmpegTimeout}
	}
	return &CopyProcessor{store: store}
}

type CopyProcessor struct {
	store storage.ObjectStore
}

func (p *CopyProcessor) Process(ctx context.Context, task model.MediaTask) error {
	reader, err := p.store.GetObject(ctx, task.InputObjectKey)
	if err != nil {
		return err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return storage.PutBytes(ctx, p.store, task.OutputObjectKey, data, "application/octet-stream")
}

type FFmpegProcessor struct {
	store   storage.ObjectStore
	timeout time.Duration
}

func (p *FFmpegProcessor) Process(ctx context.Context, task model.MediaTask) error {
	reader, err := p.store.GetObject(ctx, task.InputObjectKey)
	if err != nil {
		return err
	}
	defer reader.Close()

	dir, err := os.MkdirTemp("", "cloudfile-ffmpeg-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	input := filepath.Join(dir, "input")
	output := filepath.Join(dir, outputName(task.TaskType))
	inputFile, err := os.Create(input)
	if err != nil {
		return err
	}
	if _, err := io.Copy(inputFile, reader); err != nil {
		inputFile.Close()
		return err
	}
	if err := inputFile.Close(); err != nil {
		return err
	}

	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "ffmpeg", commandArgs(task.TaskType, input, output)...)
	stderr, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("ffmpeg timeout: %w", runCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %s", tail(string(stderr), 2000))
	}
	out, err := os.Open(output)
	if err != nil {
		return err
	}
	defer out.Close()
	stat, err := out.Stat()
	if err != nil {
		return err
	}
	return p.store.PutObject(ctx, task.OutputObjectKey, out, stat.Size(), "application/octet-stream")
}

func outputName(taskType model.MediaTaskType) string {
	switch taskType {
	case model.ImageCompress:
		return "image.jpg"
	case model.VideoTranscode:
		return "video.mp4"
	case model.VideoThumbnail:
		return "thumbnail.jpg"
	case model.MediaMetadata:
		return "metadata.txt"
	default:
		return "output.bin"
	}
}

func commandArgs(taskType model.MediaTaskType, input, output string) []string {
	switch taskType {
	case model.ImageCompress:
		return []string{"-y", "-i", input, "-vf", "scale='min(1920,iw)':-2", "-q:v", "3", output}
	case model.VideoTranscode:
		return []string{"-y", "-i", input, "-c:v", "libx264", "-preset", "veryfast", "-crf", "28", "-c:a", "aac", "-movflags", "+faststart", output}
	case model.VideoThumbnail:
		return []string{"-y", "-ss", "00:00:01", "-i", input, "-frames:v", "1", "-q:v", "3", output}
	case model.MediaMetadata:
		return []string{"-y", "-i", input, "-f", "ffmetadata", output}
	default:
		return []string{"-y", "-i", input, output}
	}
}

func tail(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}
