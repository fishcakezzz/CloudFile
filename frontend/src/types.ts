import type { LucideIcon } from "lucide-react";

export type UploadKind = "general" | "image" | "video" | "archive" | "batch" | "instant" | "resume";

export type UploadStatus =
  | "IDLE"
  | "HASHING"
  | "INSTANT_SUCCESS"
  | "UPLOADING"
  | "PAUSED"
  | "OFFLINE"
  | "MERGING"
  | "SUCCESS"
  | "MEDIA_PROCESSING"
  | "MEDIA_SUCCESS"
  | "FAILED"
  | "CANCELED";

export type ChunkStatus = "UPLOADED" | "UPLOADING" | "PENDING" | "FAILED";

export interface UploadAction {
  kind: UploadKind;
  label: string;
  hint: string;
  accept?: string;
  icon: LucideIcon;
}

export interface Chunk {
  index: number;
  status: ChunkStatus;
}

export interface UploadTask {
  id: string;
  uploadId?: string;
  file: File;
  fileName: string;
  fileType: string;
  size: number;
  hash?: string;
  kind: UploadKind;
  status: UploadStatus;
  progress: number;
  uploadedChunks: number;
  chunkCount: number;
  chunks: Chunk[];
  speed: number;
  remainingSeconds: number;
  message: string;
  createdAt: number;
  link?: string;
  mediaText?: string;
}

export interface FileItem {
  id: string;
  name: string;
  type: string;
  size: number;
  status: UploadStatus;
  uploadedAt: number;
  link: string;
  taskId: string;
}

export interface InitUploadResult {
  instant: boolean;
  uploadId?: string;
  uploadedChunks: number[];
  chunkSize: number;
  chunkCount: number;
  fileId?: string;
}

export interface UploadStats {
  todayUploads: number;
  successRate: number;
  instantHits: number;
  uploadingTasks: number;
  mediaProcessingTasks: number;
  failedTasks: number;
}
