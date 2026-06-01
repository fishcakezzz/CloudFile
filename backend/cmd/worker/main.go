package main

import (
	"context"
	"log"
	"time"

	"cloudfile/backend/internal/config"
	"cloudfile/backend/internal/db"
	"cloudfile/backend/internal/lock"
	"cloudfile/backend/internal/media"
	"cloudfile/backend/internal/model"
	"cloudfile/backend/internal/queue"
	"cloudfile/backend/internal/service"
	"cloudfile/backend/internal/storage"
)

func main() {
	cfg := config.Load()
	database, err := db.Open(cfg)
	if err != nil {
		log.Fatal(err)
	}
	store, err := storage.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	taskQueue, err := queue.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	svc := service.New(cfg, database, store, lock.New(cfg), taskQueue, media.New(cfg, store))
	ctx := context.Background()

	if cfg.QueueDriver == "rabbitmq" {
		log.Println("worker consuming RabbitMQ media queue")
		if err := taskQueue.ConsumeMediaTask(ctx, svc.RunMediaTask); err != nil {
			log.Fatal(err)
		}
		return
	}

	log.Println("worker polling inline/local media tasks")
	for {
		_ = taskQueue.ConsumeMediaTask(ctx, svc.RunMediaTask)
		var task model.MediaTask
		if err := database.Where("status = ?", model.MediaPending).Order("id ASC").First(&task).Error; err == nil {
			action, err := svc.RunMediaTask(ctx, task.ID)
			if err == nil {
				switch action {
				case queue.Retry:
					_ = taskQueue.PublishRetry(ctx, task.ID)
				case queue.Dead:
					_ = taskQueue.PublishDead(ctx, task.ID)
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
}
