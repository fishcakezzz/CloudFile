from datetime import datetime, timedelta
import hashlib
import re

from fastapi import HTTPException, UploadFile, status
from sqlalchemy import select, update
from sqlalchemy.orm import Session

from cloudfile.config import get_settings
from cloudfile.locks import LockManager
from cloudfile.media import MediaProcessor
from cloudfile.models import (
    ChunkStatus,
    FileMeta,
    FileStatus,
    MediaTask,
    MediaTaskStatus,
    MediaTaskType,
    UploadChunk,
    UploadStatus,
    UploadTask,
    UserFile,
)
from cloudfile.queue import MediaQueue, MediaTaskMessage, QueueAction
from cloudfile.schemas import InitUploadRequest
from cloudfile.storage import ObjectStore


def _hash_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _safe_file_name(file_name: str) -> str:
    safe = re.sub(r"[^A-Za-z0-9._-]+", "_", file_name).strip("._")
    return safe or "file"


def _final_object_key(upload: UploadTask) -> str:
    return f"files/{upload.file_hash}/{_safe_file_name(upload.file_name)}"


def _object_hash(storage: ObjectStore, object_key: str) -> str:
    return hashlib.sha256(storage.get_object(object_key)).hexdigest()


def create_user_file(db: Session, user_id: str, file_id: int, file_name: str, parent_id: int | None = None) -> UserFile:
    user_file = UserFile(user_id=user_id, file_id=file_id, file_name=file_name, parent_id=parent_id)
    db.add(user_file)
    db.flush()
    return user_file


def init_upload(db: Session, payload: InitUploadRequest):
    existing = db.scalar(
        select(FileMeta).where(
            FileMeta.file_hash == payload.file_hash,
            FileMeta.file_size == payload.file_size,
            FileMeta.status == FileStatus.SUCCESS,
        )
    )
    if existing:
        user_file = create_user_file(db, payload.user_id, existing.id, payload.file_name, payload.parent_id)
        db.commit()
        return {
            "instant": True,
            "upload_id": None,
            "file_id": existing.id,
            "user_file_id": user_file.id,
            "chunk_size": payload.chunk_size,
            "chunk_count": payload.chunk_count,
            "uploaded_chunks": [],
            "status": UploadStatus.SUCCESS.value,
        }

    upload = UploadTask(
        user_id=payload.user_id,
        file_name=payload.file_name,
        file_hash=payload.file_hash,
        file_size=payload.file_size,
        chunk_size=payload.chunk_size,
        chunk_count=payload.chunk_count,
        content_type=payload.content_type,
        status=UploadStatus.UPLOADING,
    )
    db.add(upload)
    db.commit()
    db.refresh(upload)
    return {
        "instant": False,
        "upload_id": upload.id,
        "file_id": None,
        "user_file_id": None,
        "chunk_size": upload.chunk_size,
        "chunk_count": upload.chunk_count,
        "uploaded_chunks": [],
        "status": upload.status.value,
    }


def get_upload_for_user(db: Session, upload_id: int, user_id: str) -> UploadTask:
    upload = db.get(UploadTask, upload_id)
    if not upload or upload.user_id != user_id:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="upload task not found")
    return upload


def uploaded_chunk_indexes(db: Session, upload_id: int) -> list[int]:
    return list(
        db.scalars(
            select(UploadChunk.chunk_index)
            .where(UploadChunk.upload_id == upload_id, UploadChunk.status == ChunkStatus.UPLOADED)
            .order_by(UploadChunk.chunk_index)
        )
    )


async def upload_chunk(
    db: Session,
    storage: ObjectStore,
    upload_id: int,
    user_id: str,
    chunk_index: int,
    upload_file: UploadFile,
    chunk_hash: str | None,
):
    upload = get_upload_for_user(db, upload_id, user_id)
    if upload.status == UploadStatus.SUCCESS:
        return upload, True
    if upload.status not in {UploadStatus.UPLOADING, UploadStatus.INIT}:
        raise HTTPException(status_code=409, detail=f"upload status is {upload.status}")
    if chunk_index < 0 or chunk_index >= upload.chunk_count:
        raise HTTPException(status_code=400, detail="chunk_index out of range")

    existing = db.scalar(
        select(UploadChunk).where(UploadChunk.upload_id == upload_id, UploadChunk.chunk_index == chunk_index)
    )
    if existing and existing.status == ChunkStatus.UPLOADED:
        return upload, True

    data = await upload_file.read()
    if not data:
        raise HTTPException(status_code=400, detail="empty chunk")
    expected_max = upload.chunk_size
    is_last = chunk_index == upload.chunk_count - 1
    if len(data) > expected_max or (not is_last and len(data) != expected_max):
        raise HTTPException(status_code=400, detail="invalid chunk size")
    if chunk_hash and _hash_bytes(data) != chunk_hash:
        raise HTTPException(status_code=400, detail="chunk_hash mismatch")

    object_key = f"temp/{upload_id}/{chunk_index}.part"
    storage.put_object(object_key, data)

    first_upload = False
    if existing:
        existing.chunk_hash = chunk_hash
        existing.chunk_size = len(data)
        existing.object_key = object_key
        if existing.status != ChunkStatus.UPLOADED:
            existing.status = ChunkStatus.UPLOADED
            first_upload = True
    else:
        db.add(
            UploadChunk(
                upload_id=upload_id,
                chunk_index=chunk_index,
                chunk_hash=chunk_hash,
                chunk_size=len(data),
                object_key=object_key,
                status=ChunkStatus.UPLOADED,
            )
        )
        first_upload = True

    if first_upload:
        upload.uploaded_count += 1
    if upload.status == UploadStatus.INIT:
        upload.status = UploadStatus.UPLOADING
    db.commit()
    db.refresh(upload)
    return upload, False


def merge_upload(
    db: Session,
    storage: ObjectStore,
    locks: LockManager,
    media_queue: MediaQueue,
    upload_id: int,
    user_id: str,
):
    settings = get_settings()
    upload = get_upload_for_user(db, upload_id, user_id)
    if upload.status == UploadStatus.SUCCESS:
        file_meta = db.scalar(select(FileMeta).where(FileMeta.object_key == upload.object_key))
        return upload, file_meta, None, []

    with locks.lock(f"merge_lock:{upload_id}", settings.merge_lock_ttl_seconds) as merge_lock:
        if not merge_lock.acquired:
            raise HTTPException(status_code=409, detail="merge is already running")

        affected = db.execute(
            update(UploadTask)
            .where(
                UploadTask.id == upload_id,
                UploadTask.user_id == user_id,
                UploadTask.status == UploadStatus.UPLOADING,
                UploadTask.uploaded_count == UploadTask.chunk_count,
            )
            .values(status=UploadStatus.MERGING)
        ).rowcount
        db.commit()
        if affected != 1:
            db.refresh(upload)
            if upload.uploaded_count < upload.chunk_count:
                raise HTTPException(status_code=409, detail="not all chunks uploaded")
            raise HTTPException(status_code=409, detail=f"upload cannot be merged from {upload.status}")

        upload = db.get(UploadTask, upload_id)
        try:
            chunks = list(
                db.scalars(
                    select(UploadChunk)
                    .where(UploadChunk.upload_id == upload_id, UploadChunk.status == ChunkStatus.UPLOADED)
                    .order_by(UploadChunk.chunk_index)
                )
            )
            if len(chunks) != upload.chunk_count:
                raise ValueError("uploaded chunk count mismatch")

            object_key = _final_object_key(upload)
            storage.compose_object(object_key, [chunk.object_key for chunk in chunks])
            if storage.stat_object(object_key).size != upload.file_size:
                raise ValueError("merged file size mismatch")
            merged_hash = _object_hash(storage, object_key)
            if merged_hash != upload.file_hash:
                raise ValueError("merged file hash mismatch")

            file_meta = db.scalar(
                select(FileMeta).where(FileMeta.file_hash == upload.file_hash, FileMeta.file_size == upload.file_size)
            )
            if not file_meta:
                file_meta = FileMeta(
                    file_hash=upload.file_hash,
                    file_size=upload.file_size,
                    object_key=object_key,
                    content_type=upload.content_type,
                    storage_bucket=getattr(storage, "bucket", settings.object_storage_bucket),
                    status=FileStatus.SUCCESS,
                )
                db.add(file_meta)
                db.flush()

            user_file = create_user_file(db, user_id, file_meta.id, upload.file_name)
            upload.status = UploadStatus.SUCCESS
            upload.object_key = object_key
            tasks = create_media_tasks(db, file_meta)
            db.commit()
            storage.remove_prefix(f"temp/{upload_id}")
            for task in tasks:
                media_queue.publish_media_task(MediaTaskMessage(task_id=task.id))
            return upload, file_meta, user_file, tasks
        except Exception as exc:
            upload.status = UploadStatus.FAILED
            upload.error_msg = str(exc)
            db.commit()
            raise HTTPException(status_code=500, detail=f"merge failed: {exc}") from exc


def create_media_tasks(db: Session, file_meta: FileMeta) -> list[MediaTask]:
    content_type = file_meta.content_type or ""
    task_types: list[MediaTaskType] = []
    if content_type.startswith("image/"):
        task_types = [MediaTaskType.IMAGE_COMPRESS, MediaTaskType.MEDIA_METADATA]
    elif content_type.startswith("video/"):
        task_types = [MediaTaskType.VIDEO_TRANSCODE, MediaTaskType.VIDEO_THUMBNAIL, MediaTaskType.MEDIA_METADATA]
    if not task_types:
        return []

    settings = get_settings()
    tasks = [
        MediaTask(
            file_id=file_meta.id,
            task_type=task_type,
            input_object_key=file_meta.object_key,
            output_object_key=f"processed/{file_meta.id}/{task_type.value.lower()}",
            status=MediaTaskStatus.PENDING,
            max_retry=settings.max_media_retry,
        )
        for task_type in task_types
    ]
    db.add_all(tasks)
    db.flush()
    return tasks


def run_media_task(
    db: Session,
    storage: ObjectStore,
    processor: MediaProcessor,
    task_id: int,
) -> tuple[MediaTask, QueueAction]:
    task = db.get(MediaTask, task_id)
    if not task:
        raise HTTPException(status_code=404, detail="media task not found")
    if task.status == MediaTaskStatus.SUCCESS:
        return task, QueueAction.ACK
    if task.status not in {MediaTaskStatus.PENDING, MediaTaskStatus.FAILED}:
        return task, QueueAction.ACK

    affected = db.execute(
        update(MediaTask)
        .where(
            MediaTask.id == task_id,
            MediaTask.status.in_([MediaTaskStatus.PENDING, MediaTaskStatus.FAILED]),
        )
        .values(status=MediaTaskStatus.PROCESSING, error_msg=None)
    ).rowcount
    db.commit()
    if affected != 1:
        db.refresh(task)
        return task, QueueAction.ACK

    task = db.get(MediaTask, task_id)
    try:
        processor.process(task, storage)
        task.status = MediaTaskStatus.SUCCESS
        task.error_msg = None
        action = QueueAction.ACK
    except Exception as exc:
        task.retry_count += 1
        task.error_msg = str(exc)
        if task.retry_count >= task.max_retry:
            task.status = MediaTaskStatus.DEAD
            action = QueueAction.DEAD
        else:
            task.status = MediaTaskStatus.FAILED
            action = QueueAction.RETRY
    db.commit()
    db.refresh(task)
    return task, action


def compensate(db: Session, storage: ObjectStore, media_queue: MediaQueue):
    settings = get_settings()
    repaired_uploads = 0
    retried_media_tasks = 0
    dead_media_tasks = 0

    stale_before = datetime.utcnow() - timedelta(seconds=settings.processing_timeout_seconds)
    stale_uploads = list(
        db.scalars(
            select(UploadTask).where(
                UploadTask.status == UploadStatus.MERGING,
                UploadTask.updated_at < stale_before,
            )
        )
    )
    for upload in stale_uploads:
        object_key = upload.object_key or _final_object_key(upload)
        if storage.exists(object_key) and storage.stat_object(object_key).size == upload.file_size:
            if _object_hash(storage, object_key) != upload.file_hash:
                upload.status = UploadStatus.FAILED
                upload.error_msg = "stale MERGING task hash mismatch"
            else:
                upload.status = UploadStatus.SUCCESS
                upload.object_key = object_key
        elif upload.uploaded_count == upload.chunk_count:
            upload.status = UploadStatus.UPLOADING
            upload.error_msg = "stale MERGING task reset for merge retry"
        else:
            upload.status = UploadStatus.FAILED
            upload.error_msg = "stale MERGING task could not be repaired"
        repaired_uploads += 1

    stale_tasks = list(
        db.scalars(
            select(MediaTask).where(
                MediaTask.status.in_([MediaTaskStatus.PROCESSING, MediaTaskStatus.FAILED]),
                MediaTask.updated_at < stale_before,
            )
        )
    )
    for task in stale_tasks:
        if storage.exists(task.output_object_key):
            task.status = MediaTaskStatus.SUCCESS
            task.error_msg = None
        elif task.retry_count >= task.max_retry:
            task.status = MediaTaskStatus.DEAD
            media_queue.publish_dead(MediaTaskMessage(task_id=task.id))
            dead_media_tasks += 1
        else:
            task.status = MediaTaskStatus.PENDING
            media_queue.publish_media_task(MediaTaskMessage(task_id=task.id))
            retried_media_tasks += 1

    pending_tasks = list(db.scalars(select(MediaTask).where(MediaTask.status == MediaTaskStatus.PENDING)))
    for task in pending_tasks:
        media_queue.publish_media_task(MediaTaskMessage(task_id=task.id))
        retried_media_tasks += 1

    db.commit()
    return repaired_uploads, retried_media_tasks, dead_media_tasks
