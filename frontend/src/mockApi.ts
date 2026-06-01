import type { FileItem, InitUploadResult, UploadTask } from "./types";

const API_BASE = import.meta.env.VITE_API_BASE || "http://127.0.0.1:8000";
export const USER_ID = "user_10001";
export const CHUNK_SIZE = 10 * 1024 * 1024;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, init);
  if (!response.ok) {
    let message = response.statusText;
    try {
      const body = await response.json();
      message = body.detail || message;
    } catch {
      // Keep the HTTP status text when the response is not JSON.
    }
    throw new Error(message);
  }
  return response.json() as Promise<T>;
}

export async function calculateFileHash(file: File): Promise<string> {
  const buffer = await file.arrayBuffer();
  const digest = await crypto.subtle.digest("SHA-256", buffer);
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}

export async function initUpload(file: File, hash: string, _forceInstant = false): Promise<InitUploadResult> {
  const chunkCount = Math.max(1, Math.ceil(file.size / CHUNK_SIZE));
  const result = await request<{
    instant: boolean;
    upload_id?: number | null;
    uploaded_chunks: number[];
    chunk_size: number;
    chunk_count: number;
    file_id?: number | null;
  }>("/api/uploads/init", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      user_id: USER_ID,
      file_name: file.name,
      file_hash: hash,
      file_size: file.size,
      chunk_size: CHUNK_SIZE,
      chunk_count: chunkCount,
      content_type: file.type || "application/octet-stream",
    }),
  });

  return {
    instant: result.instant,
    uploadId: result.upload_id == null ? undefined : String(result.upload_id),
    uploadedChunks: result.uploaded_chunks || [],
    chunkSize: result.chunk_size,
    chunkCount: result.chunk_count,
    fileId: result.file_id == null ? undefined : String(result.file_id),
  };
}

export async function uploadChunk(task: UploadTask, chunkIndex: number): Promise<{ chunkIndex: number }> {
  if (!task.uploadId) throw new Error("upload id is missing");
  const start = chunkIndex * CHUNK_SIZE;
  const end = Math.min(start + CHUNK_SIZE, task.file.size);
  const form = new FormData();
  form.append("user_id", USER_ID);
  form.append("file", task.file.slice(start, end), task.fileName);

  await request(`/api/uploads/${task.uploadId}/chunks/${chunkIndex}`, {
    method: "POST",
    body: form,
  });
  return { chunkIndex };
}

export async function queryUploadStatus(task: UploadTask): Promise<number[]> {
  if (!task.uploadId) return [];
  const result = await request<{ uploaded_chunks: number[] }>(
    `/api/uploads/${task.uploadId}/status?user_id=${encodeURIComponent(USER_ID)}`,
  );
  return result.uploaded_chunks || [];
}

export async function mergeChunks(task: UploadTask): Promise<void> {
  if (!task.uploadId) throw new Error("upload id is missing");
  await request(`/api/uploads/${task.uploadId}/merge?user_id=${encodeURIComponent(USER_ID)}`, {
    method: "POST",
  });
}

export async function listFiles(): Promise<FileItem[]> {
  const files = await request<
    Array<{
      id: number;
      file_name: string;
      created_at: string;
      file: {
        content_type: string;
        file_size: number;
        status: "SUCCESS" | "FAILED";
      };
    }>
  >(`/api/files?user_id=${encodeURIComponent(USER_ID)}`);

  return files.map((item) => ({
    id: String(item.id),
    taskId: `file-${item.id}`,
    name: item.file_name,
    type: item.file.content_type || "application/octet-stream",
    size: item.file.file_size,
    status: item.file.status === "SUCCESS" ? "SUCCESS" : "FAILED",
    uploadedAt: new Date(item.created_at).getTime(),
    link: `${API_BASE}/api/files/${item.id}/download?user_id=${encodeURIComponent(USER_ID)}`,
  }));
}

export async function processMedia(_task?: UploadTask): Promise<void> {
  return;
}

export async function deleteUserFileRef(fileId: string): Promise<void> {
  await request(`/api/files/${fileId}?user_id=${encodeURIComponent(USER_ID)}`, {
    method: "DELETE",
  });
}

export async function recoverUnfinishedTasks(): Promise<number> {
  return 0;
}
