from datetime import datetime
from enum import StrEnum

from sqlalchemy import BigInteger, DateTime, Enum, ForeignKey, Integer, String, Text, UniqueConstraint, func
from sqlalchemy.orm import Mapped, mapped_column, relationship

from cloudfile.database import Base


class UploadStatus(StrEnum):
    INIT = "INIT"
    UPLOADING = "UPLOADING"
    MERGING = "MERGING"
    SUCCESS = "SUCCESS"
    FAILED = "FAILED"
    CANCELED = "CANCELED"


class ChunkStatus(StrEnum):
    PENDING = "PENDING"
    UPLOADED = "UPLOADED"
    FAILED = "FAILED"


class FileStatus(StrEnum):
    SUCCESS = "SUCCESS"
    FAILED = "FAILED"


class MediaTaskStatus(StrEnum):
    PENDING = "PENDING"
    PROCESSING = "PROCESSING"
    SUCCESS = "SUCCESS"
    FAILED = "FAILED"
    DEAD = "DEAD"


class MediaTaskType(StrEnum):
    IMAGE_COMPRESS = "IMAGE_COMPRESS"
    VIDEO_TRANSCODE = "VIDEO_TRANSCODE"
    VIDEO_THUMBNAIL = "VIDEO_THUMBNAIL"
    MEDIA_METADATA = "MEDIA_METADATA"
    THUMBNAIL_GENERATE = "THUMBNAIL_GENERATE"


class TimestampMixin:
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now(), nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime,
        server_default=func.now(),
        onupdate=func.now(),
        nullable=False,
    )


class UploadTask(TimestampMixin, Base):
    __tablename__ = "upload_task"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    user_id: Mapped[str] = mapped_column(String(64), index=True, nullable=False)
    file_name: Mapped[str] = mapped_column(String(255), nullable=False)
    file_hash: Mapped[str] = mapped_column(String(128), index=True, nullable=False)
    file_size: Mapped[int] = mapped_column(BigInteger, nullable=False)
    chunk_size: Mapped[int] = mapped_column(BigInteger, nullable=False)
    chunk_count: Mapped[int] = mapped_column(Integer, nullable=False)
    uploaded_count: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    content_type: Mapped[str | None] = mapped_column(String(128))
    status: Mapped[UploadStatus] = mapped_column(Enum(UploadStatus), default=UploadStatus.INIT, nullable=False)
    object_key: Mapped[str | None] = mapped_column(String(512))
    error_msg: Mapped[str | None] = mapped_column(Text)

    chunks: Mapped[list["UploadChunk"]] = relationship(back_populates="upload", cascade="all, delete-orphan")


class UploadChunk(TimestampMixin, Base):
    __tablename__ = "upload_chunk"
    __table_args__ = (UniqueConstraint("upload_id", "chunk_index", name="uq_upload_chunk_index"),)

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    upload_id: Mapped[int] = mapped_column(ForeignKey("upload_task.id"), index=True, nullable=False)
    chunk_index: Mapped[int] = mapped_column(Integer, nullable=False)
    chunk_hash: Mapped[str | None] = mapped_column(String(128))
    chunk_size: Mapped[int] = mapped_column(BigInteger, nullable=False)
    object_key: Mapped[str] = mapped_column(String(512), nullable=False)
    status: Mapped[ChunkStatus] = mapped_column(Enum(ChunkStatus), default=ChunkStatus.PENDING, nullable=False)

    upload: Mapped[UploadTask] = relationship(back_populates="chunks")


class FileMeta(TimestampMixin, Base):
    __tablename__ = "file_meta"
    __table_args__ = (UniqueConstraint("file_hash", "file_size", name="uq_file_hash_size"),)

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    file_hash: Mapped[str] = mapped_column(String(128), nullable=False)
    file_size: Mapped[int] = mapped_column(BigInteger, nullable=False)
    object_key: Mapped[str] = mapped_column(String(512), nullable=False)
    content_type: Mapped[str | None] = mapped_column(String(128))
    storage_bucket: Mapped[str] = mapped_column(String(128), nullable=False)
    status: Mapped[FileStatus] = mapped_column(Enum(FileStatus), default=FileStatus.SUCCESS, nullable=False)


class UserFile(TimestampMixin, Base):
    __tablename__ = "user_file"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    user_id: Mapped[str] = mapped_column(String(64), index=True, nullable=False)
    file_id: Mapped[int] = mapped_column(ForeignKey("file_meta.id"), nullable=False)
    file_name: Mapped[str] = mapped_column(String(255), nullable=False)
    parent_id: Mapped[int | None] = mapped_column(Integer)

    file: Mapped[FileMeta] = relationship()


class MediaTask(TimestampMixin, Base):
    __tablename__ = "media_task"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    file_id: Mapped[int] = mapped_column(ForeignKey("file_meta.id"), index=True, nullable=False)
    task_type: Mapped[MediaTaskType] = mapped_column(Enum(MediaTaskType), nullable=False)
    input_object_key: Mapped[str] = mapped_column(String(512), nullable=False)
    output_object_key: Mapped[str] = mapped_column(String(512), nullable=False)
    status: Mapped[MediaTaskStatus] = mapped_column(
        Enum(MediaTaskStatus),
        default=MediaTaskStatus.PENDING,
        nullable=False,
    )
    retry_count: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    max_retry: Mapped[int] = mapped_column(Integer, default=3, nullable=False)
    error_msg: Mapped[str | None] = mapped_column(Text)

    file: Mapped[FileMeta] = relationship()
