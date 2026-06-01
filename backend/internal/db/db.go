package db

import (
	"cloudfile/backend/internal/config"
	"cloudfile/backend/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector
	if cfg.DatabaseDriver == "mysql" {
		dialector = mysql.Open(cfg.DatabaseDSN)
	} else {
		dialector = sqlite.Open(cfg.DatabaseDSN)
	}
	database, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := database.AutoMigrate(
		&model.UploadTask{},
		&model.UploadChunk{},
		&model.FileMeta{},
		&model.UserFile{},
		&model.MediaTask{},
	); err != nil {
		return nil, err
	}
	return database, nil
}
