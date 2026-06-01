from functools import lru_cache

from cloudfile.config import get_settings
from cloudfile.locks import InMemoryLockManager, LockManager, RedisLockManager
from cloudfile.media import CopyProcessor, FFmpegProcessor, MediaProcessor
from cloudfile.queue import InlineQueue, RabbitMQQueue, TaskQueue
from cloudfile.storage import LocalObjectStore, MinioObjectStore, ObjectStore


@lru_cache
def get_storage() -> ObjectStore:
    settings = get_settings()
    if settings.storage_driver.lower() == "minio":
        try:
            return MinioObjectStore(
                endpoint=settings.minio_endpoint,
                access_key=settings.minio_access_key,
                secret_key=settings.minio_secret_key,
                bucket=settings.minio_bucket,
                secure=settings.minio_secure,
            )
        except Exception:
            if settings.storage_driver.lower() == "minio":
                raise
    return LocalObjectStore(settings.object_storage_root, settings.object_storage_bucket)


@lru_cache
def get_lock_manager() -> LockManager:
    settings = get_settings()
    if settings.redis_url:
        try:
            manager = RedisLockManager(settings.redis_url)
            manager.client.ping()
            return manager
        except Exception:
            pass
    return InMemoryLockManager()


@lru_cache
def get_media_queue() -> TaskQueue:
    settings = get_settings()
    driver = settings.queue_driver or settings.queue_mode
    if driver.lower() == "rabbitmq" and settings.rabbitmq_url:
        return RabbitMQQueue(settings.rabbitmq_url)
    return InlineQueue()


@lru_cache
def get_media_processor() -> MediaProcessor:
    settings = get_settings()
    if settings.media_processor.lower() == "ffmpeg":
        return FFmpegProcessor(settings.ffmpeg_timeout_seconds)
    return CopyProcessor()
