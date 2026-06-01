from __future__ import annotations

from collections.abc import Iterable
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path
from typing import Protocol
import shutil

from minio import Minio
from minio.commonconfig import ComposeSource
from minio.error import S3Error


@dataclass(frozen=True)
class ObjectStat:
    size: int


class ObjectStore(Protocol):
    bucket: str

    def put_object(self, object_key: str, data: bytes) -> None:
        ...

    def get_object(self, object_key: str) -> bytes:
        ...

    def stat_object(self, object_key: str) -> ObjectStat:
        ...

    def remove_object(self, object_key: str) -> None:
        ...

    def compose_object(self, destination_key: str, source_keys: Iterable[str]) -> None:
        ...

    def copy_object(self, source_key: str, destination_key: str) -> None:
        ...

    def remove_prefix(self, prefix: str) -> None:
        ...


class LocalObjectStore:
    """ObjectStore implementation backed by the local filesystem."""

    def __init__(self, root: Path, bucket: str) -> None:
        self.root = root
        self.bucket = bucket
        self.bucket_root.mkdir(parents=True, exist_ok=True)

    @property
    def bucket_root(self) -> Path:
        return self.root / self.bucket

    def _path(self, object_key: str) -> Path:
        path = (self.bucket_root / object_key).resolve()
        root = self.bucket_root.resolve()
        if root not in path.parents and path != root:
            raise ValueError("object_key escapes storage root")
        return path

    def put_object(self, object_key: str, data: bytes) -> None:
        path = self._path(object_key)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)

    def get_object(self, object_key: str) -> bytes:
        return self._path(object_key).read_bytes()

    def stat_object(self, object_key: str) -> ObjectStat:
        return ObjectStat(size=self._path(object_key).stat().st_size)

    def remove_object(self, object_key: str) -> None:
        path = self._path(object_key)
        if path.exists():
            path.unlink()

    def compose_object(self, destination_key: str, source_keys: Iterable[str]) -> None:
        destination = self._path(destination_key)
        destination.parent.mkdir(parents=True, exist_ok=True)
        tmp = destination.with_suffix(destination.suffix + ".tmp")
        with tmp.open("wb") as output:
            for source_key in source_keys:
                with self._path(source_key).open("rb") as source:
                    shutil.copyfileobj(source, output, length=1024 * 1024)
        tmp.replace(destination)

    def copy_object(self, source_key: str, destination_key: str) -> None:
        destination = self._path(destination_key)
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(self._path(source_key), destination)

    def remove_prefix(self, prefix: str) -> None:
        path = self._path(prefix)
        if path.exists() and path.is_dir():
            shutil.rmtree(path)

    def exists(self, object_key: str) -> bool:
        try:
            self.stat_object(object_key)
            return True
        except FileNotFoundError:
            return False

    def size(self, object_key: str) -> int:
        return self.stat_object(object_key).size

    def put_bytes(self, object_key: str, data: bytes) -> None:
        self.put_object(object_key, data)

    def get_bytes(self, object_key: str) -> bytes:
        return self.get_object(object_key)

    def compose(self, destination_key: str, source_keys: Iterable[str]) -> None:
        self.compose_object(destination_key, source_keys)

    def copy(self, source_key: str, destination_key: str) -> None:
        self.copy_object(source_key, destination_key)

    def delete_prefix(self, prefix: str) -> None:
        self.remove_prefix(prefix)


class MinioObjectStore:
    """ObjectStore implementation backed by real MinIO/S3-compatible storage."""

    def __init__(self, endpoint: str, access_key: str, secret_key: str, bucket: str, secure: bool = False) -> None:
        self.bucket = bucket
        self.client = Minio(endpoint, access_key=access_key, secret_key=secret_key, secure=secure)
        if not self.client.bucket_exists(bucket):
            self.client.make_bucket(bucket)

    def put_object(self, object_key: str, data: bytes) -> None:
        self.client.put_object(self.bucket, object_key, BytesIO(data), length=len(data))

    def get_object(self, object_key: str) -> bytes:
        response = self.client.get_object(self.bucket, object_key)
        try:
            return response.read()
        finally:
            response.close()
            response.release_conn()

    def stat_object(self, object_key: str) -> ObjectStat:
        stat = self.client.stat_object(self.bucket, object_key)
        return ObjectStat(size=stat.size or 0)

    def remove_object(self, object_key: str) -> None:
        self.client.remove_object(self.bucket, object_key)

    def compose_object(self, destination_key: str, source_keys: Iterable[str]) -> None:
        sources = [ComposeSource(self.bucket, source_key) for source_key in source_keys]
        if not sources:
            raise ValueError("compose_object requires at least one source")
        self.client.compose_object(self.bucket, destination_key, sources)

    def copy_object(self, source_key: str, destination_key: str) -> None:
        self.compose_object(destination_key, [source_key])

    def remove_prefix(self, prefix: str) -> None:
        for item in self.client.list_objects(self.bucket, prefix=prefix, recursive=True):
            self.client.remove_object(self.bucket, item.object_name)

    def exists(self, object_key: str) -> bool:
        try:
            self.stat_object(object_key)
            return True
        except S3Error as exc:
            if exc.code in {"NoSuchKey", "NoSuchObject", "NotFound"}:
                return False
            raise

    def size(self, object_key: str) -> int:
        return self.stat_object(object_key).size

    def put_bytes(self, object_key: str, data: bytes) -> None:
        self.put_object(object_key, data)

    def get_bytes(self, object_key: str) -> bytes:
        return self.get_object(object_key)

    def compose(self, destination_key: str, source_keys: Iterable[str]) -> None:
        self.compose_object(destination_key, source_keys)

    def copy(self, source_key: str, destination_key: str) -> None:
        self.copy_object(source_key, destination_key)

    def delete_prefix(self, prefix: str) -> None:
        self.remove_prefix(prefix)


LocalObjectStorage = LocalObjectStore
