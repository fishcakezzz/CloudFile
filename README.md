# CloudFile

CloudFile 是一个按照 `项目介绍.md` 中“云端大文件上传与异步处理调度平台”实现的 Go/Gin 后端示例，覆盖：

- 上传初始化与秒传校验
- 分片上传、幂等记录、断点续传
- Redis 分布式锁入口控制与数据库 CAS 合并兜底
- 对象存储临时分片与最终文件路径组织
- 文件元数据和用户文件引用解耦
- 媒体处理任务、重试、死信状态和补偿接口

## Go/Gin 后端快速运行

```bash
cd CloudFile
cd backend
go mod download
go run ./cmd/api
```

默认使用 SQLite、LocalObjectStore、InlineQueue 和 CopyProcessor，接口运行在 `http://127.0.0.1:8000`。

如需 MySQL、Redis、RabbitMQ、MinIO：

```bash
docker compose up -d
```

真实组件驱动可通过环境变量切换：

```bash
STORAGE_DRIVER=minio
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=cloudfile
QUEUE_DRIVER=rabbitmq
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
MEDIA_PROCESSOR=ffmpeg
```

未设置这些驱动时，系统继续使用本地目录对象存储、inline 队列和 CopyProcessor，方便开发和测试。

Go 后端主目录为 `backend/`：

- `cmd/api`：Gin HTTP 服务
- `cmd/worker`：RabbitMQ/inline 媒体处理 worker
- `internal/storage`：ObjectStore、LocalObjectStore、MinioObjectStore
- `internal/queue`：TaskQueue、InlineQueue、RabbitMQQueue
- `internal/media`：CopyProcessor、FFmpegProcessor
- `internal/service`：上传、秒传、分片、合并、媒体任务、补偿逻辑

## 主要接口

- `POST /api/uploads/init` 初始化上传，命中秒传时直接创建 `user_file`
- `POST /api/uploads/{upload_id}/chunks/{chunk_index}` 上传分片
- `GET /api/uploads/{upload_id}/status` 查询已上传分片，用于断点续传
- `POST /api/uploads/{upload_id}/merge` 合并分片并投递媒体任务
- `GET /api/files` 查询用户文件视图
- `GET /api/media-tasks/{task_id}` 查询媒体处理任务
- `POST /api/admin/compensate` 执行异常任务补偿

## 验证

当前目录仍保留早期 Python 参考实现，但 Docker 和 README 已切换到 Go/Gin 后端。

## 前端控制台

前端位于 `frontend/`，使用 React + TypeScript + Tailwind CSS + Framer Motion + lucide-react，实现了完整 mock 交互：

```bash
cd frontend
npm install
npm run dev
```

打开 Vite 输出的地址即可访问 CloudFile 可视化控制台。当前前端默认使用 `src/mockApi.ts`，后续接入真实后端时只需要替换该文件中的接口函数。
