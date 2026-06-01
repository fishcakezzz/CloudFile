package main

import (
	"log"

	"cloudfile/backend/internal/config"
	"cloudfile/backend/internal/db"
	apihttp "cloudfile/backend/internal/http"
	"cloudfile/backend/internal/lock"
	"cloudfile/backend/internal/media"
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
	locker := lock.New(cfg)
	processor := media.New(cfg, store)
	svc := service.New(cfg, database, store, locker, taskQueue, processor)
	router := apihttp.NewRouter(svc)
	if err := router.Run(cfg.HTTPAddr); err != nil {
		log.Fatal(err)
	}
}
