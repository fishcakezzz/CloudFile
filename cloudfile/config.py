from functools import lru_cache
from pathlib import Path

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    app_name: str = "CloudFile"
    database_url: str = "sqlite:///./cloudfile.db"
    object_storage_root: Path = Path("./object_storage")
    object_storage_bucket: str = "cloudfile"
    storage_driver: str = "local"
    minio_endpoint: str = "localhost:9000"
    minio_access_key: str = "minioadmin"
    minio_secret_key: str = "minioadmin"
    minio_bucket: str = "cloudfile"
    minio_secure: bool = False
    redis_url: str | None = None
    rabbitmq_url: str | None = None
    queue_driver: str = "inline"
    queue_mode: str = "inline"
    media_processor: str = "copy"
    max_media_retry: int = 3
    merge_lock_ttl_seconds: int = 120
    processing_timeout_seconds: int = 600
    ffmpeg_timeout_seconds: int = 300

    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8")


@lru_cache
def get_settings() -> Settings:
    return Settings()
