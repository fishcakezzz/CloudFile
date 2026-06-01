from contextlib import AbstractContextManager
from threading import Lock
from uuid import uuid4

import redis


class MergeLock(AbstractContextManager):
    def __init__(self, manager: "LockManager", name: str, ttl_seconds: int) -> None:
        self.manager = manager
        self.name = name
        self.ttl_seconds = ttl_seconds
        self.value = str(uuid4())
        self.acquired = False

    def __enter__(self) -> "MergeLock":
        self.acquired = self.manager.acquire(self.name, self.value, self.ttl_seconds)
        return self

    def __exit__(self, exc_type, exc_value, traceback) -> None:
        if self.acquired:
            self.manager.release(self.name, self.value)


class LockManager:
    def lock(self, name: str, ttl_seconds: int) -> MergeLock:
        return MergeLock(self, name, ttl_seconds)

    def acquire(self, name: str, value: str, ttl_seconds: int) -> bool:
        raise NotImplementedError

    def release(self, name: str, value: str) -> None:
        raise NotImplementedError


class RedisLockManager(LockManager):
    RELEASE_SCRIPT = """
    if redis.call("get", KEYS[1]) == ARGV[1] then
        return redis.call("del", KEYS[1])
    else
        return 0
    end
    """

    def __init__(self, url: str) -> None:
        self.client = redis.Redis.from_url(url, decode_responses=True)

    def acquire(self, name: str, value: str, ttl_seconds: int) -> bool:
        return bool(self.client.set(name, value, nx=True, ex=ttl_seconds))

    def release(self, name: str, value: str) -> None:
        self.client.eval(self.RELEASE_SCRIPT, 1, name, value)


class InMemoryLockManager(LockManager):
    def __init__(self) -> None:
        self._guard = Lock()
        self._locks: dict[str, str] = {}

    def acquire(self, name: str, value: str, ttl_seconds: int) -> bool:
        with self._guard:
            if name in self._locks:
                return False
            self._locks[name] = value
            return True

    def release(self, name: str, value: str) -> None:
        with self._guard:
            if self._locks.get(name) == value:
                del self._locks[name]
