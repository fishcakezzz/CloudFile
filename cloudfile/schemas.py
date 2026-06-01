from datetime import datetime

from pydantic import BaseModel, ConfigDict, Field, field_validator

from cloudfile.models import MediaTaskStatus, UploadStatus


class InitUploadRequest(BaseModel):
    user_id: str = Field(min_length=1, max_length=64)
    file_name: str = Field(min_length=1, max_length=255)
    file_hash: str = Field(min_length=8, max_length=128)
    file_size: int = Field(gt=0)
    chunk_size: int = Field(gt=0)
    chunk_count: int = Field(gt=0)
    content_type: str | None = Field(default=None, max_length=128)
    parent_id: int | None = None

    @field_validator("chunk_count")
    @classmethod
    def chunk_count_must_match(cls, chunk_count: int, info):
        data = info.data
        file_size = data.get("file_size")
        chunk_size = data.get("chunk_size")
        if file_size and chunk_size:
            expected = (file_size + chunk_size - 1) // chunk_size
            if chunk_count != expected:
                raise ValueError(f"chunk_count must be {expected} for file_size/chunk_size")
        return chunk_count


class InitUploadResponse(BaseModel):
    instant: bool
    upload_id: int | None = None
    file_id: int | None = None
    user_file_id: int | None = None
    chunk_size: int
    chunk_count: int
    uploaded_chunks: list[int]
    status: str


class UploadStatusResponse(BaseModel):
    upload_id: int
    status: UploadStatus
    uploaded_count: int
    chunk_count: int
    uploaded_chunks: list[int]


class ChunkUploadResponse(BaseModel):
    upload_id: int
    chunk_index: int
    uploaded_count: int
    chunk_count: int
    already_uploaded: bool
    can_merge: bool


class MergeResponse(BaseModel):
    upload_id: int
    status: UploadStatus
    file_id: int | None = None
    user_file_id: int | None = None
    media_task_ids: list[int] = []
    message: str


class UserFileResponse(BaseModel):
    id: int
    user_id: str
    file_id: int
    file_name: str
    object_key: str
    file_size: int
    content_type: str | None
    created_at: datetime


class MediaTaskResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    file_id: int
    task_type: str
    input_object_key: str
    output_object_key: str
    status: MediaTaskStatus
    retry_count: int
    max_retry: int
    error_msg: str | None


class CompensationResponse(BaseModel):
    repaired_uploads: int
    retried_media_tasks: int
    dead_media_tasks: int
