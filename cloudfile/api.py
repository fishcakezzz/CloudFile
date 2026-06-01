from fastapi import APIRouter, Depends, File, Form, UploadFile
from sqlalchemy import select
from sqlalchemy.orm import Session

from cloudfile.database import get_db
from cloudfile.dependencies import get_lock_manager, get_media_processor, get_media_queue, get_storage
from cloudfile.locks import LockManager
from cloudfile.media import MediaProcessor
from cloudfile.models import MediaTask, UploadTask, UserFile
from cloudfile.queue import MediaQueue
from cloudfile.schemas import (
    ChunkUploadResponse,
    CompensationResponse,
    InitUploadRequest,
    InitUploadResponse,
    MediaTaskResponse,
    MergeResponse,
    UploadStatusResponse,
    UserFileResponse,
)
from cloudfile.services import (
    compensate,
    init_upload,
    merge_upload,
    run_media_task,
    upload_chunk,
    uploaded_chunk_indexes,
)
from cloudfile.storage import ObjectStore

router = APIRouter(prefix="/api")


@router.post("/uploads/init", response_model=InitUploadResponse)
def initialize_upload(payload: InitUploadRequest, db: Session = Depends(get_db)):
    return init_upload(db, payload)


@router.get("/uploads/{upload_id}/status", response_model=UploadStatusResponse)
def get_upload_status(upload_id: int, user_id: str, db: Session = Depends(get_db)):
    upload = db.get(UploadTask, upload_id)
    if not upload or upload.user_id != user_id:
        from fastapi import HTTPException

        raise HTTPException(status_code=404, detail="upload task not found")
    return UploadStatusResponse(
        upload_id=upload.id,
        status=upload.status,
        uploaded_count=upload.uploaded_count,
        chunk_count=upload.chunk_count,
        uploaded_chunks=uploaded_chunk_indexes(db, upload.id),
    )


@router.post("/uploads/{upload_id}/chunks/{chunk_index}", response_model=ChunkUploadResponse)
async def post_chunk(
    upload_id: int,
    chunk_index: int,
    user_id: str = Form(...),
    chunk_hash: str | None = Form(default=None),
    file: UploadFile = File(...),
    db: Session = Depends(get_db),
    storage: ObjectStore = Depends(get_storage),
):
    upload, already_uploaded = await upload_chunk(db, storage, upload_id, user_id, chunk_index, file, chunk_hash)
    return ChunkUploadResponse(
        upload_id=upload.id,
        chunk_index=chunk_index,
        uploaded_count=upload.uploaded_count,
        chunk_count=upload.chunk_count,
        already_uploaded=already_uploaded,
        can_merge=upload.uploaded_count == upload.chunk_count,
    )


@router.post("/uploads/{upload_id}/merge", response_model=MergeResponse)
def merge(
    upload_id: int,
    user_id: str,
    db: Session = Depends(get_db),
    storage: ObjectStore = Depends(get_storage),
    locks: LockManager = Depends(get_lock_manager),
    media_queue: MediaQueue = Depends(get_media_queue),
):
    upload, file_meta, user_file, media_tasks = merge_upload(db, storage, locks, media_queue, upload_id, user_id)
    return MergeResponse(
        upload_id=upload.id,
        status=upload.status,
        file_id=file_meta.id if file_meta else None,
        user_file_id=user_file.id if user_file else None,
        media_task_ids=[task.id for task in media_tasks],
        message="merge completed" if media_tasks else "merge completed; no media task required",
    )


@router.get("/files", response_model=list[UserFileResponse])
def list_files(user_id: str, db: Session = Depends(get_db)):
    rows = db.scalars(select(UserFile).where(UserFile.user_id == user_id).order_by(UserFile.created_at.desc())).all()
    return [
        UserFileResponse(
            id=row.id,
            user_id=row.user_id,
            file_id=row.file_id,
            file_name=row.file_name,
            object_key=row.file.object_key,
            file_size=row.file.file_size,
            content_type=row.file.content_type,
            created_at=row.created_at,
        )
        for row in rows
    ]


@router.get("/media-tasks/{task_id}", response_model=MediaTaskResponse)
def get_media_task(task_id: int, db: Session = Depends(get_db)):
    from fastapi import HTTPException

    task = db.get(MediaTask, task_id)
    if not task:
        raise HTTPException(status_code=404, detail="media task not found")
    return task


@router.post("/worker/media-tasks/{task_id}/run", response_model=MediaTaskResponse)
def run_worker_task(
    task_id: int,
    db: Session = Depends(get_db),
    storage: ObjectStore = Depends(get_storage),
    processor: MediaProcessor = Depends(get_media_processor),
):
    task, _ = run_media_task(db, storage, processor, task_id)
    return task


@router.post("/admin/compensate", response_model=CompensationResponse)
def compensate_tasks(
    db: Session = Depends(get_db),
    storage: ObjectStore = Depends(get_storage),
    media_queue: MediaQueue = Depends(get_media_queue),
):
    repaired_uploads, retried_media_tasks, dead_media_tasks = compensate(db, storage, media_queue)
    return CompensationResponse(
        repaired_uploads=repaired_uploads,
        retried_media_tasks=retried_media_tasks,
        dead_media_tasks=dead_media_tasks,
    )
