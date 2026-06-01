import hashlib
import os
import tempfile

os.environ["DATABASE_URL"] = f"sqlite:///{tempfile.mkdtemp()}/cloudfile-test.db"
os.environ["OBJECT_STORAGE_ROOT"] = tempfile.mkdtemp()
os.environ["REDIS_URL"] = ""
os.environ["QUEUE_MODE"] = "inline"

from fastapi.testclient import TestClient

from cloudfile.main import app


client = TestClient(app)


def test_chunk_upload_merge_media_task_and_instant_upload():
    with client:
        content = b"0123456789abcdef"
        digest = hashlib.sha256(content).hexdigest()
        init_payload = {
            "user_id": "u1",
            "file_name": "demo.png",
            "file_hash": digest,
            "file_size": len(content),
            "chunk_size": 5,
            "chunk_count": 4,
            "content_type": "image/png",
        }

        response = client.post("/api/uploads/init", json=init_payload)
        assert response.status_code == 200
        body = response.json()
        assert body["instant"] is False
        upload_id = body["upload_id"]

        chunks = [content[0:5], content[5:10], content[10:15], content[15:]]
        for index, chunk in enumerate(chunks):
            response = client.post(
                f"/api/uploads/{upload_id}/chunks/{index}",
                data={"user_id": "u1", "chunk_hash": hashlib.sha256(chunk).hexdigest()},
                files={"file": (f"{index}.part", chunk, "application/octet-stream")},
            )
            assert response.status_code == 200

        duplicate = client.post(
            f"/api/uploads/{upload_id}/chunks/0",
            data={"user_id": "u1"},
            files={"file": ("0.part", chunks[0], "application/octet-stream")},
        )
        assert duplicate.status_code == 200
        assert duplicate.json()["already_uploaded"] is True
        assert duplicate.json()["uploaded_count"] == 4

        status_response = client.get(f"/api/uploads/{upload_id}/status", params={"user_id": "u1"})
        assert status_response.status_code == 200
        assert status_response.json()["uploaded_chunks"] == [0, 1, 2, 3]

        merge_response = client.post(f"/api/uploads/{upload_id}/merge", params={"user_id": "u1"})
        assert merge_response.status_code == 200
        merged = merge_response.json()
        assert merged["status"] == "SUCCESS"
        assert len(merged["media_task_ids"]) == 2

        task_response = client.post(f"/api/worker/media-tasks/{merged['media_task_ids'][0]}/run")
        assert task_response.status_code == 200
        assert task_response.json()["status"] == "SUCCESS"

        instant_payload = dict(init_payload)
        instant_payload["user_id"] = "u2"
        instant_response = client.post("/api/uploads/init", json=instant_payload)
        assert instant_response.status_code == 200
        assert instant_response.json()["instant"] is True

        files_response = client.get("/api/files", params={"user_id": "u2"})
        assert files_response.status_code == 200
        assert files_response.json()[0]["file_name"] == "demo.png"
