from __future__ import annotations

from pathlib import Path
from tempfile import TemporaryDirectory
from typing import Protocol
import subprocess

from cloudfile.config import get_settings
from cloudfile.models import MediaTask, MediaTaskType
from cloudfile.storage import ObjectStore


class MediaProcessor(Protocol):
    def process(self, task: MediaTask, store: ObjectStore) -> None:
        ...


class CopyProcessor:
    def process(self, task: MediaTask, store: ObjectStore) -> None:
        if not store.exists(task.input_object_key):
            raise FileNotFoundError(task.input_object_key)
        store.copy_object(task.input_object_key, task.output_object_key)


class FFmpegProcessor:
    def __init__(self, timeout_seconds: int | None = None) -> None:
        self.timeout_seconds = timeout_seconds or get_settings().ffmpeg_timeout_seconds

    def process(self, task: MediaTask, store: ObjectStore) -> None:
        if not store.exists(task.input_object_key):
            raise FileNotFoundError(task.input_object_key)

        with TemporaryDirectory(prefix="cloudfile-media-") as tmp_dir:
            tmp = Path(tmp_dir)
            input_path = tmp / "input"
            output_path = self._output_path(tmp, task.task_type)
            input_path.write_bytes(store.get_object(task.input_object_key))
            command = self._command(task.task_type, input_path, output_path)
            result = subprocess.run(
                command,
                shell=False,
                capture_output=True,
                text=True,
                timeout=self.timeout_seconds,
                check=False,
            )
            if result.returncode != 0:
                stderr = result.stderr.strip() or result.stdout.strip() or "ffmpeg failed"
                raise RuntimeError(stderr[-2000:])
            if not output_path.exists():
                raise RuntimeError("ffmpeg did not create output file")
            store.put_object(task.output_object_key, output_path.read_bytes())

    def _output_path(self, tmp: Path, task_type: MediaTaskType) -> Path:
        if task_type == MediaTaskType.IMAGE_COMPRESS:
            return tmp / "output.jpg"
        if task_type == MediaTaskType.VIDEO_TRANSCODE:
            return tmp / "output.mp4"
        if task_type in {MediaTaskType.VIDEO_THUMBNAIL, MediaTaskType.THUMBNAIL_GENERATE}:
            return tmp / "thumbnail.jpg"
        if task_type == MediaTaskType.MEDIA_METADATA:
            return tmp / "metadata.txt"
        return tmp / "output.bin"

    def _command(self, task_type: MediaTaskType, input_path: Path, output_path: Path) -> list[str]:
        if task_type == MediaTaskType.IMAGE_COMPRESS:
            return [
                "ffmpeg",
                "-y",
                "-i",
                str(input_path),
                "-vf",
                "scale='min(1920,iw)':-2",
                "-q:v",
                "3",
                str(output_path),
            ]
        if task_type == MediaTaskType.VIDEO_TRANSCODE:
            return [
                "ffmpeg",
                "-y",
                "-i",
                str(input_path),
                "-c:v",
                "libx264",
                "-preset",
                "veryfast",
                "-crf",
                "28",
                "-c:a",
                "aac",
                "-movflags",
                "+faststart",
                str(output_path),
            ]
        if task_type in {MediaTaskType.VIDEO_THUMBNAIL, MediaTaskType.THUMBNAIL_GENERATE}:
            return [
                "ffmpeg",
                "-y",
                "-ss",
                "00:00:01",
                "-i",
                str(input_path),
                "-frames:v",
                "1",
                "-q:v",
                "3",
                str(output_path),
            ]
        if task_type == MediaTaskType.MEDIA_METADATA:
            return [
                "ffmpeg",
                "-y",
                "-i",
                str(input_path),
                "-f",
                "ffmetadata",
                str(output_path),
            ]
        raise ValueError(f"unsupported media task type: {task_type}")
