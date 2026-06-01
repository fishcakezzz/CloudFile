package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"regexp"
	"sort"
	"strings"
	"time"

	"cloudfile/backend/internal/config"
	"cloudfile/backend/internal/lock"
	"cloudfile/backend/internal/media"
	"cloudfile/backend/internal/model"
	"cloudfile/backend/internal/queue"
	"cloudfile/backend/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	cfg       config.Config
	db        *gorm.DB
	store     storage.ObjectStore
	locker    lock.Lock
	queue     queue.TaskQueue
	processor media.Processor
}

func New(cfg config.Config, db *gorm.DB, store storage.ObjectStore, locker lock.Lock, q queue.TaskQueue, processor media.Processor) *Service {
	return &Service{cfg: cfg, db: db, store: store, locker: locker, queue: q, processor: processor}
}

type InitUploadRequest struct {
	UserID      string `json:"user_id" binding:"required"`
	FileName    string `json:"file_name" binding:"required"`
	FileHash    string `json:"file_hash" binding:"required"`
	FileSize    int64  `json:"file_size" binding:"required"`
	ChunkSize   int64  `json:"chunk_size" binding:"required"`
	ChunkCount  int    `json:"chunk_count" binding:"required"`
	ContentType string `json:"content_type"`
	ParentID    *uint  `json:"parent_id"`
}

func (s *Service) InitUpload(ctx context.Context, req InitUploadRequest) (map[string]any, error) {
	var meta model.FileMeta
	err := s.db.Where("file_hash = ? AND file_size = ? AND status = ?", req.FileHash, req.FileSize, model.FileSuccess).First(&meta).Error
	if err == nil {
		userFile := model.UserFile{UserID: req.UserID, FileID: meta.ID, FileName: req.FileName, ParentID: req.ParentID}
		if err := s.db.Create(&userFile).Error; err != nil {
			return nil, err
		}
		return map[string]any{
			"instant": true, "upload_id": nil, "file_id": meta.ID, "user_file_id": userFile.ID,
			"chunk_size": req.ChunkSize, "chunk_count": req.ChunkCount, "uploaded_chunks": []int{}, "status": model.UploadSuccess,
		}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	task := model.UploadTask{
		UserID: req.UserID, FileName: req.FileName, FileHash: req.FileHash, FileSize: req.FileSize,
		ChunkSize: req.ChunkSize, ChunkCount: req.ChunkCount, ContentType: req.ContentType, Status: model.UploadUploading,
	}
	if err := s.db.Create(&task).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"instant": false, "upload_id": task.ID, "file_id": nil, "user_file_id": nil,
		"chunk_size": task.ChunkSize, "chunk_count": task.ChunkCount, "uploaded_chunks": []int{}, "status": task.Status,
	}, nil
}

func (s *Service) UploadStatus(uploadID uint, userID string) (model.UploadTask, []int, error) {
	task, err := s.uploadForUser(uploadID, userID)
	if err != nil {
		return task, nil, err
	}
	var chunks []model.UploadChunk
	if err := s.db.Where("upload_id = ? AND status = ?", uploadID, model.ChunkUploaded).Order("chunk_index ASC").Find(&chunks).Error; err != nil {
		return task, nil, err
	}
	indexes := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		indexes = append(indexes, chunk.ChunkIndex)
	}
	return task, indexes, nil
}

func (s *Service) UploadChunk(ctx context.Context, uploadID uint, userID string, chunkIndex int, chunkHash string, header *multipart.FileHeader) (model.UploadTask, bool, error) {
	task, err := s.uploadForUser(uploadID, userID)
	if err != nil {
		return task, false, err
	}
	if task.Status == model.UploadSuccess {
		return task, true, nil
	}
	if task.Status != model.UploadUploading && task.Status != model.UploadInit {
		return task, false, fmt.Errorf("upload status is %s", task.Status)
	}
	if chunkIndex < 0 || chunkIndex >= task.ChunkCount {
		return task, false, fmt.Errorf("chunk_index out of range")
	}
	var existing model.UploadChunk
	err = s.db.Where("upload_id = ? AND chunk_index = ?", uploadID, chunkIndex).First(&existing).Error
	if err == nil && existing.Status == model.ChunkUploaded {
		return task, true, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return task, false, err
	}

	file, err := header.Open()
	if err != nil {
		return task, false, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return task, false, err
	}
	if len(data) == 0 {
		return task, false, fmt.Errorf("empty chunk")
	}
	isLast := chunkIndex == task.ChunkCount-1
	if int64(len(data)) > task.ChunkSize || (!isLast && int64(len(data)) != task.ChunkSize) {
		return task, false, fmt.Errorf("invalid chunk size")
	}
	if chunkHash != "" && sha256Hex(data) != chunkHash {
		return task, false, fmt.Errorf("chunk_hash mismatch")
	}

	key := fmt.Sprintf("temp/%d/%d.part", uploadID, chunkIndex)
	if err := storage.PutBytes(ctx, s.store, key, data, "application/octet-stream"); err != nil {
		return task, false, err
	}

	firstUpload := false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var chunk model.UploadChunk
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("upload_id = ? AND chunk_index = ?", uploadID, chunkIndex).First(&chunk).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			chunk = model.UploadChunk{UploadID: uploadID, ChunkIndex: chunkIndex, ChunkHash: chunkHash, ChunkSize: int64(len(data)), ObjectKey: key, Status: model.ChunkUploaded}
			if err := tx.Create(&chunk).Error; err != nil {
				return err
			}
			firstUpload = true
		} else if err != nil {
			return err
		} else if chunk.Status != model.ChunkUploaded {
			firstUpload = true
			chunk.ChunkHash = chunkHash
			chunk.ChunkSize = int64(len(data))
			chunk.ObjectKey = key
			chunk.Status = model.ChunkUploaded
			if err := tx.Save(&chunk).Error; err != nil {
				return err
			}
		}
		if firstUpload {
			return tx.Model(&model.UploadTask{}).Where("id = ?", uploadID).Updates(map[string]any{
				"uploaded_count": gorm.Expr("uploaded_count + 1"),
				"status":         model.UploadUploading,
			}).Error
		}
		return nil
	})
	if err != nil {
		return task, false, err
	}
	task, err = s.uploadForUser(uploadID, userID)
	return task, false, err
}

func (s *Service) MergeUpload(ctx context.Context, uploadID uint, userID string) (model.UploadTask, *model.FileMeta, *model.UserFile, []model.MediaTask, error) {
	task, err := s.uploadForUser(uploadID, userID)
	if err != nil {
		return task, nil, nil, nil, err
	}
	if task.Status == model.UploadSuccess {
		var meta model.FileMeta
		_ = s.db.Where("object_key = ?", task.ObjectKey).First(&meta).Error
		return task, &meta, nil, nil, nil
	}

	lockKey := fmt.Sprintf("merge_lock:%d", uploadID)
	value, ok, err := s.locker.Acquire(ctx, lockKey, s.cfg.MergeLockTTL)
	if err != nil || !ok {
		return task, nil, nil, nil, fmt.Errorf("merge is already running")
	}
	defer s.locker.Release(ctx, lockKey, value)

	result := s.db.Model(&model.UploadTask{}).
		Where("id = ? AND user_id = ? AND status = ? AND uploaded_count = chunk_count", uploadID, userID, model.UploadUploading).
		Update("status", model.UploadMerging)
	if result.Error != nil {
		return task, nil, nil, nil, result.Error
	}
	if result.RowsAffected != 1 {
		return task, nil, nil, nil, fmt.Errorf("upload cannot be merged")
	}
	task, _ = s.uploadForUser(uploadID, userID)

	var chunks []model.UploadChunk
	if err := s.db.Where("upload_id = ? AND status = ?", uploadID, model.ChunkUploaded).Order("chunk_index ASC").Find(&chunks).Error; err != nil {
		return s.markUploadFailed(task, err)
	}
	if len(chunks) != task.ChunkCount {
		return s.markUploadFailed(task, fmt.Errorf("uploaded chunk count mismatch"))
	}
	keys := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		keys = append(keys, chunk.ObjectKey)
	}
	objectKey := finalObjectKey(task)
	if err := s.store.ComposeObject(ctx, objectKey, keys); err != nil {
		return s.markUploadFailed(task, err)
	}
	info, err := s.store.StatObject(ctx, objectKey)
	if err != nil || info.Size != task.FileSize {
		return s.markUploadFailed(task, fmt.Errorf("merged file size mismatch"))
	}
	hash, err := objectHash(ctx, s.store, objectKey)
	if err != nil || hash != task.FileHash {
		return s.markUploadFailed(task, fmt.Errorf("merged file hash mismatch"))
	}

	var meta model.FileMeta
	err = s.db.Where("file_hash = ? AND file_size = ?", task.FileHash, task.FileSize).First(&meta).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		meta = model.FileMeta{FileHash: task.FileHash, FileSize: task.FileSize, ObjectKey: objectKey, ContentType: task.ContentType, StorageBucket: s.store.Bucket(), Status: model.FileSuccess}
		if err := s.db.Create(&meta).Error; err != nil {
			return s.markUploadFailed(task, err)
		}
	} else if err != nil {
		return s.markUploadFailed(task, err)
	}
	userFile := model.UserFile{UserID: userID, FileID: meta.ID, FileName: task.FileName}
	tasks := s.createMediaTasks(meta)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&userFile).Error; err != nil {
			return err
		}
		if len(tasks) > 0 {
			if err := tx.Create(&tasks).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.UploadTask{}).Where("id = ?", task.ID).Updates(map[string]any{"status": model.UploadSuccess, "object_key": objectKey}).Error
	})
	if err != nil {
		return s.markUploadFailed(task, err)
	}
	_ = s.store.RemovePrefix(ctx, fmt.Sprintf("temp/%d", uploadID))
	for _, mediaTask := range tasks {
		_ = s.queue.PublishMediaTask(ctx, mediaTask.ID)
	}
	task, _ = s.uploadForUser(uploadID, userID)
	return task, &meta, &userFile, tasks, nil
}

func (s *Service) RunMediaTask(ctx context.Context, taskID uint) (queue.Action, error) {
	var task model.MediaTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return queue.Ack, err
	}
	if task.Status == model.MediaSuccess {
		return queue.Ack, nil
	}
	if task.Status != model.MediaPending && task.Status != model.MediaFailed {
		return queue.Ack, nil
	}
	result := s.db.Model(&model.MediaTask{}).
		Where("id = ? AND status IN ?", taskID, []model.MediaTaskStatus{model.MediaPending, model.MediaFailed}).
		Updates(map[string]any{"status": model.MediaProcessing, "error_msg": ""})
	if result.Error != nil {
		return queue.Ack, result.Error
	}
	if result.RowsAffected != 1 {
		return queue.Ack, nil
	}
	if err := s.db.First(&task, taskID).Error; err != nil {
		return queue.Ack, err
	}
	if err := s.processor.Process(ctx, task); err != nil {
		task.RetryCount++
		task.ErrorMsg = err.Error()
		if task.RetryCount >= task.MaxRetry {
			task.Status = model.MediaDead
			_ = s.db.Save(&task).Error
			return queue.Dead, nil
		}
		task.Status = model.MediaFailed
		_ = s.db.Save(&task).Error
		return queue.Retry, nil
	}
	task.Status = model.MediaSuccess
	task.ErrorMsg = ""
	return queue.Ack, s.db.Save(&task).Error
}

func (s *Service) Compensate(ctx context.Context) (map[string]int, error) {
	repairedUploads, retriedMedia, deadMedia := 0, 0, 0
	staleBefore := time.Now().Add(-s.cfg.ProcessingTimeout)
	var uploads []model.UploadTask
	if err := s.db.Where("status = ? AND updated_at < ?", model.UploadMerging, staleBefore).Find(&uploads).Error; err != nil {
		return nil, err
	}
	for _, upload := range uploads {
		key := upload.ObjectKey
		if key == "" {
			key = finalObjectKey(upload)
		}
		if s.store.Exists(ctx, key) {
			if info, err := s.store.StatObject(ctx, key); err == nil && info.Size == upload.FileSize {
				hash, _ := objectHash(ctx, s.store, key)
				if hash == upload.FileHash {
					s.db.Model(&upload).Updates(map[string]any{"status": model.UploadSuccess, "object_key": key, "error_msg": ""})
					repairedUploads++
					continue
				}
			}
		}
		if upload.UploadedCount == upload.ChunkCount {
			s.db.Model(&upload).Updates(map[string]any{"status": model.UploadUploading, "error_msg": "reset for merge retry"})
		} else {
			s.db.Model(&upload).Updates(map[string]any{"status": model.UploadFailed, "error_msg": "stale merge could not be repaired"})
		}
		repairedUploads++
	}

	var mediaTasks []model.MediaTask
	if err := s.db.Where("status IN ? AND updated_at < ?", []model.MediaTaskStatus{model.MediaProcessing, model.MediaFailed}, staleBefore).Find(&mediaTasks).Error; err != nil {
		return nil, err
	}
	for _, task := range mediaTasks {
		if s.store.Exists(ctx, task.OutputObjectKey) {
			s.db.Model(&task).Updates(map[string]any{"status": model.MediaSuccess, "error_msg": ""})
			continue
		}
		if task.RetryCount >= task.MaxRetry {
			s.db.Model(&task).Update("status", model.MediaDead)
			_ = s.queue.PublishDead(ctx, task.ID)
			deadMedia++
		} else {
			s.db.Model(&task).Update("status", model.MediaPending)
			_ = s.queue.PublishMediaTask(ctx, task.ID)
			retriedMedia++
		}
	}
	var pending []model.MediaTask
	if err := s.db.Where("status = ?", model.MediaPending).Find(&pending).Error; err != nil {
		return nil, err
	}
	for _, task := range pending {
		_ = s.queue.PublishMediaTask(ctx, task.ID)
		retriedMedia++
	}
	return map[string]int{"repaired_uploads": repairedUploads, "retried_media_tasks": retriedMedia, "dead_media_tasks": deadMedia}, nil
}

func (s *Service) ListFiles(userID string) ([]model.UserFile, error) {
	var files []model.UserFile
	err := s.db.Preload("File").Where("user_id = ?", userID).Order("created_at DESC").Find(&files).Error
	return files, err
}

func (s *Service) DownloadFile(ctx context.Context, userID string, fileID uint) (model.UserFile, io.ReadCloser, error) {
	var file model.UserFile
	if err := s.db.Preload("File").Where("id = ? AND user_id = ?", fileID, userID).First(&file).Error; err != nil {
		return file, nil, err
	}
	reader, err := s.store.GetObject(ctx, file.File.ObjectKey)
	return file, reader, err
}

func (s *Service) DeletePhysicalFile(ctx context.Context, userID string, userFileID uint) (map[string]any, error) {
	var file model.UserFile
	if err := s.db.Preload("File").Where("id = ? AND user_id = ?", userFileID, userID).First(&file).Error; err != nil {
		return nil, err
	}

	var mediaTasks []model.MediaTask
	if err := s.db.Where("file_id = ?", file.FileID).Find(&mediaTasks).Error; err != nil {
		return nil, err
	}

	removedObjects := 0
	if file.File.ObjectKey != "" {
		if err := s.store.RemoveObject(ctx, file.File.ObjectKey); err != nil {
			return nil, err
		}
		removedObjects++
	}
	for _, task := range mediaTasks {
		if task.OutputObjectKey == "" {
			continue
		}
		if err := s.store.RemoveObject(ctx, task.OutputObjectKey); err != nil {
			return nil, err
		}
		removedObjects++
	}

	var removedRefs int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("file_id = ?", file.FileID).Delete(&model.MediaTask{}).Error; err != nil {
			return err
		}
		refs := tx.Where("file_id = ?", file.FileID).Delete(&model.UserFile{})
		if refs.Error != nil {
			return refs.Error
		}
		removedRefs = refs.RowsAffected
		return tx.Delete(&model.FileMeta{}, file.FileID).Error
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"deleted":         true,
		"physical":        true,
		"user_file_id":    userFileID,
		"file_id":         file.FileID,
		"removed_refs":    removedRefs,
		"removed_objects": removedObjects,
	}, nil
}

func (s *Service) MediaTask(taskID uint) (model.MediaTask, error) {
	var task model.MediaTask
	err := s.db.First(&task, taskID).Error
	return task, err
}

func (s *Service) Queue() queue.TaskQueue {
	return s.queue
}

func (s *Service) uploadForUser(uploadID uint, userID string) (model.UploadTask, error) {
	var task model.UploadTask
	err := s.db.Where("id = ? AND user_id = ?", uploadID, userID).First(&task).Error
	return task, err
}

func (s *Service) markUploadFailed(task model.UploadTask, err error) (model.UploadTask, *model.FileMeta, *model.UserFile, []model.MediaTask, error) {
	s.db.Model(&task).Updates(map[string]any{"status": model.UploadFailed, "error_msg": err.Error()})
	return task, nil, nil, nil, err
}

func (s *Service) createMediaTasks(meta model.FileMeta) []model.MediaTask {
	var types []model.MediaTaskType
	if strings.HasPrefix(meta.ContentType, "image/") {
		types = []model.MediaTaskType{model.ImageCompress, model.MediaMetadata}
	} else if strings.HasPrefix(meta.ContentType, "video/") {
		types = []model.MediaTaskType{model.VideoTranscode, model.VideoThumbnail, model.MediaMetadata}
	}
	tasks := make([]model.MediaTask, 0, len(types))
	for _, taskType := range types {
		tasks = append(tasks, model.MediaTask{
			FileID: meta.ID, TaskType: taskType, InputObjectKey: meta.ObjectKey,
			OutputObjectKey: fmt.Sprintf("processed/%d/%s", meta.ID, strings.ToLower(string(taskType))),
			Status:          model.MediaPending, MaxRetry: s.cfg.MaxMediaRetry,
		})
	}
	return tasks
}

func objectHash(ctx context.Context, store storage.ObjectStore, key string) (string, error) {
	reader, err := store.GetObject(ctx, key)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func finalObjectKey(task model.UploadTask) string {
	return fmt.Sprintf("files/%s/%s", task.FileHash, safeFileName(task.FileName))
}

func safeFileName(name string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	safe := strings.Trim(re.ReplaceAllString(name, "_"), "._")
	if safe == "" {
		return "file"
	}
	return safe
}

func SortChunks(chunks []model.UploadChunk) {
	sort.Slice(chunks, func(i, j int) bool { return chunks[i].ChunkIndex < chunks[j].ChunkIndex })
}
