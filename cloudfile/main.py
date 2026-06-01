from contextlib import asynccontextmanager

from fastapi import FastAPI

from cloudfile.api import router
from cloudfile.database import init_db


def create_app() -> FastAPI:
    @asynccontextmanager
    async def lifespan(app: FastAPI):
        init_db()
        yield

    app = FastAPI(
        title="CloudFile",
        description="Large-file chunk upload, resumable merge and asynchronous media processing platform.",
        version="0.1.0",
        lifespan=lifespan,
    )

    @app.get("/health")
    def health():
        return {"status": "ok"}

    app.include_router(router)
    return app


app = create_app()
