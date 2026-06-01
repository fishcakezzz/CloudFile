# CloudFile

CloudFile 是一个面向大文件上传、秒传校验、断点续传、对象存储和异步媒体处理的 DEMO 项目。项目包含 Go/Gin 后端、React 控制台，以及可选的 MySQL、Redis、RabbitMQ、MinIO、FFmpeg 组件。

它的重点不是只做一个上传按钮，而是把一个大文件从选择、Hash、秒传判断、分片上传、合并、入库、媒体处理、下载与物理删除的完整生命周期展示出来。

![CloudFile 控制台概览](docs/images/demo-dashboard-empty.png)

## Demo Highlights

### 1. 控制台总览

控制台顶部展示当前技术栈和运行状态，左侧提供通用文件、图片、视频、压缩包、批量上传、秒传检测和恢复任务入口。中间区域展示上传入口、任务队列和用户文件列表。

![控制台空状态](docs/images/demo-dashboard-empty.png)

核心能力：

- React + TypeScript + Tailwind CSS 构建可视化控制台
- Gin API 提供上传、合并、列表、下载、删除接口
- 文件列表支持搜索、类型筛选、状态筛选和排序
- 下载按钮直连后端真实文件流
- 删除按钮执行物理文件删除，而不是只删除引用

### 2. 图片上传与媒体处理

图片上传完成后会进入媒体处理链路，任务卡片展示 Hash、分片、合并和媒体处理状态，文件最终进入用户文件列表。

![图片上传完成](docs/images/demo-image-upload-complete.png)

技术链路：

1. 浏览器计算 SHA-256 文件 Hash
2. 后端根据 `file_hash + file_size` 判断是否命中秒传
3. 未命中时创建上传任务
4. 前端按固定大小切片上传
5. 后端合并分片并校验最终 Hash
6. 图片任务进入媒体处理队列
7. 文件元数据写入用户文件视图

### 3. 秒传命中

当同一份文件已经存在时，后端不会重复写入物理文件，而是直接创建新的用户文件引用，前端展示“秒传成功”。

![秒传成功](docs/images/demo-instant-upload.png)

核心技术点：

- 使用文件 Hash 作为内容寻址依据
- 数据库维护 `file_meta` 和 `user_file` 两层模型
- 相同物理文件可被多个用户文件记录引用
- 秒传路径跳过分片上传和合并流程

### 4. 压缩包大文件分片上传

压缩包上传支持 `.zip`、`.rar`、`.7z`、`.tar`、`.gz`、`.tgz`、`.bz2`、`.xz` 等常见格式。任务队列实时展示分片进度、上传速度和剩余时间。

![压缩包上传中](docs/images/demo-archive-uploading.png)

核心技术点：

- 前端按 `CHUNK_SIZE` 切片
- 后端校验分片大小和索引合法性
- 每个分片先写入临时对象路径
- 数据库记录每个分片状态，支持断点续传查询
- 合并时按分片索引顺序 Compose 成最终对象

### 5. 文件入库、下载与物理删除

上传完成后，文件进入用户文件列表。列表中的下载按钮调用后端下载接口，删除按钮会删除底层对象存储文件、用户引用、媒体任务和文件元数据。

![压缩包上传完成](docs/images/demo-archive-complete.png)

相关接口：

```http
GET    /api/files?user_id=user_10001
GET    /api/files/:file_id/download?user_id=user_10001
DELETE /api/files/:file_id?user_id=user_10001
```

## Architecture

```text
React Console
  |
  |  init / chunk / status / merge / files / download / delete
  v
Gin API
  |
  +-- Upload Service
  |     +-- SHA-256 秒传校验
  |     +-- 分片状态记录
  |     +-- 合并事务控制
  |
  +-- Object Store
  |     +-- LocalObjectStore
  |     +-- MinIOObjectStore
  |
  +-- Queue
  |     +-- InlineQueue
  |     +-- RabbitMQQueue
  |
  +-- Media Processor
        +-- CopyProcessor
        +-- FFmpegProcessor
```

## Tech Stack

- Frontend: React, TypeScript, Vite, Tailwind CSS, Framer Motion, lucide-react
- Backend: Go, Gin, GORM
- Storage: LocalObjectStore, MinIO
- Database: SQLite for local demo, MySQL for full deployment
- Cache and Lock: Redis CAS style distributed lock
- Queue: InlineQueue, RabbitMQ
- Media: CopyProcessor, FFmpegProcessor
- Deployment: Docker Compose

## Quick Start

### Backend

```bash
cd backend
go mod download
go run ./cmd/api
```

默认使用 SQLite、本地对象存储、InlineQueue 和 CopyProcessor，API 地址为：

```text
http://127.0.0.1:8000
```

健康检查：

```text
http://127.0.0.1:8000/health
```

Windows 本地运行 SQLite 驱动时需要启用 CGO：

```powershell
cd F:\CloudFile\backend
$env:CGO_ENABLED="1"
go run ./cmd/api
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

前端地址：

```text
http://127.0.0.1:5173/
```

### Full Components

如需启动 MySQL、Redis、RabbitMQ、MinIO 和 API/Worker 容器：

```bash
docker compose up -d
```

可通过环境变量切换真实组件：

```bash
DATABASE_DRIVER=mysql
DATABASE_DSN=cloudfile:cloudfile@tcp(mysql:3306)/cloudfile?parseTime=true&loc=Local
STORAGE_DRIVER=minio
MINIO_ENDPOINT=minio:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=cloudfile
QUEUE_DRIVER=rabbitmq
RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
MEDIA_PROCESSOR=ffmpeg
```

## API Overview

```http
POST   /api/uploads/init
POST   /api/uploads/:upload_id/chunks/:chunk_index
GET    /api/uploads/:upload_id/status
POST   /api/uploads/:upload_id/merge
GET    /api/files
GET    /api/files/:file_id/download
DELETE /api/files/:file_id
GET    /api/media-tasks/:task_id
POST   /api/worker/media-tasks/:task_id/run
POST   /api/admin/compensate
```

## Repository Layout

```text
backend/
  cmd/api          Gin HTTP service
  cmd/worker       media task worker
  internal/config  environment config
  internal/db      GORM database setup
  internal/http    HTTP routes and handlers
  internal/service upload, merge, file and media orchestration
  internal/storage local and MinIO object stores
  internal/queue   inline and RabbitMQ queues
  internal/media   copy and FFmpeg processors

frontend/
  src/App.tsx                  main console
  src/components/UploadPanel   upload type selector
  src/components/UploadTaskCard upload task lifecycle card
  src/components/FileList      file list, download and delete
  src/mockApi.ts               real API client used by the demo
```


