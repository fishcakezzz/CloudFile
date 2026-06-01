import { AnimatePresence, motion } from "framer-motion";
import {
  Eye,
  Pause,
  Play,
  RotateCcw,
  Trash2,
  X,
  CheckCircle2,
  Loader2,
  CircleOff,
} from "lucide-react";
import type { UploadStatus, UploadTask } from "../types";

interface UploadTaskCardProps {
  task: UploadTask;
  onPause: (id: string) => void;
  onResume: (id: string) => void;
  onCancel: (id: string) => void;
  onRetry: (id: string) => void;
  onDelete: (id: string) => void;
  onViewChunks: (task: UploadTask) => void;
}

const statusLabel: Record<UploadStatus, string> = {
  IDLE: "等待上传",
  HASHING: "计算 Hash 中",
  INSTANT_SUCCESS: "秒传成功",
  UPLOADING: "上传中",
  PAUSED: "已暂停",
  OFFLINE: "网络已断开",
  MERGING: "合并中",
  SUCCESS: "上传完成",
  MEDIA_PROCESSING: "媒体处理中",
  MEDIA_SUCCESS: "处理完成",
  FAILED: "失败",
  CANCELED: "已取消",
};

const statusClass: Record<UploadStatus, string> = {
  IDLE: "bg-slate-100 text-slate-600",
  HASHING: "bg-sky-50 text-sky-700",
  INSTANT_SUCCESS: "bg-emerald-50 text-emerald-700",
  UPLOADING: "bg-indigo-50 text-indigo-700",
  PAUSED: "bg-amber-50 text-amber-700",
  OFFLINE: "bg-orange-50 text-orange-700",
  MERGING: "bg-violet-50 text-violet-700",
  SUCCESS: "bg-blue-50 text-blue-700",
  MEDIA_PROCESSING: "bg-fuchsia-50 text-fuchsia-700",
  MEDIA_SUCCESS: "bg-emerald-50 text-emerald-700",
  FAILED: "bg-rose-50 text-rose-700",
  CANCELED: "bg-slate-100 text-slate-500",
};

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let size = bytes / 1024;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size.toFixed(size > 99 ? 0 : 1)} ${units[index]}`;
}

function formatTime(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "--";
  if (seconds < 60) return `${Math.ceil(seconds)}s`;
  return `${Math.floor(seconds / 60)}m ${Math.ceil(seconds % 60)}s`;
}

function ProgressRing({ progress, active }: { progress: number; active: boolean }) {
  const radius = 21;
  const circumference = 2 * Math.PI * radius;
  const offset = circumference - (progress / 100) * circumference;
  return (
    <div className="relative h-14 w-14 shrink-0">
      <svg className="-rotate-90" width="56" height="56">
        <circle cx="28" cy="28" r={radius} stroke="#e2e8f0" strokeWidth="6" fill="none" />
        <motion.circle
          cx="28"
          cy="28"
          r={radius}
          stroke={active ? "#6366f1" : "#10b981"}
          strokeWidth="6"
          fill="none"
          strokeLinecap="round"
          initial={false}
          animate={{ strokeDashoffset: offset }}
          transition={{ type: "spring", stiffness: 90, damping: 22 }}
          strokeDasharray={circumference}
        />
      </svg>
      <span className="absolute inset-0 grid place-items-center text-xs font-semibold text-slate-700">
        {Math.round(progress)}%
      </span>
    </div>
  );
}

export function UploadTaskCard({
  task,
  onPause,
  onResume,
  onCancel,
  onRetry,
  onDelete,
  onViewChunks,
}: UploadTaskCardProps) {
  const active = ["HASHING", "UPLOADING", "MERGING", "MEDIA_PROCESSING"].includes(task.status);
  const isTerminal = ["INSTANT_SUCCESS", "SUCCESS", "MEDIA_SUCCESS", "CANCELED"].includes(task.status);

  return (
    <motion.article
      layout
      initial={{ opacity: 0, y: 18, scale: 0.98 }}
      animate={{ opacity: 1, y: 0, scale: 1 }}
      exit={{ opacity: 0, x: -32, scale: 0.96 }}
      className="rounded-lg border border-white/80 bg-white/95 p-4 shadow-soft"
    >
      <div className="flex flex-col gap-4 md:flex-row md:items-start">
        <ProgressRing progress={task.progress} active={active} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold text-slate-950">{task.fileName}</div>
              <div className="mt-1 text-xs text-slate-500">
                {task.fileType || "unknown"} · {formatBytes(task.size)}
              </div>
            </div>
            <motion.span
              key={task.status}
              initial={{ opacity: 0, y: -6 }}
              animate={{ opacity: 1, y: 0 }}
              className={`rounded-full px-2.5 py-1 text-xs font-semibold ${statusClass[task.status]}`}
            >
              {statusLabel[task.status]}
            </motion.span>
          </div>

          <div className="mt-4 h-2 overflow-hidden rounded-full bg-slate-100">
            <motion.div
              className="h-full rounded-full bg-gradient-to-r from-blue-500 via-indigo-500 to-violet-500"
              initial={false}
              animate={{ width: `${task.progress}%` }}
              transition={{ type: "spring", stiffness: 80, damping: 20 }}
            />
          </div>

          <AnimatePresence mode="wait">
            <motion.div
              key={task.message}
              initial={{ opacity: 0, y: 6 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -6 }}
              className="mt-3 flex items-center gap-2 text-xs text-slate-600"
            >
              {active ? <Loader2 size={14} className="animate-spin text-indigo-600" /> : <CheckCircle2 size={14} />}
              <span>{task.message}</span>
            </motion.div>
          </AnimatePresence>

          <div className="mt-4 grid gap-3 text-xs text-slate-600 sm:grid-cols-3">
            <div>分片：{task.uploadedChunks} / {task.chunkCount}</div>
            <div>速度：{task.speed > 0 ? `${formatBytes(task.speed)}/s` : "--"}</div>
            <div>剩余：{formatTime(task.remainingSeconds)}</div>
          </div>
        </div>

        <div className="flex shrink-0 flex-wrap gap-2 md:w-36 md:justify-end">
          {task.status === "UPLOADING" && (
            <button className="icon-btn" type="button" title="暂停" onClick={() => onPause(task.id)}>
              <Pause size={16} />
            </button>
          )}
          {["PAUSED", "OFFLINE"].includes(task.status) && (
            <button className="icon-btn" type="button" title="继续" onClick={() => onResume(task.id)}>
              <Play size={16} />
            </button>
          )}
          {task.status === "FAILED" && (
            <button className="icon-btn" type="button" title="重试" onClick={() => onRetry(task.id)}>
              <RotateCcw size={16} />
            </button>
          )}
          {!isTerminal && (
            <button className="icon-btn" type="button" title="取消" onClick={() => onCancel(task.id)}>
              <X size={16} />
            </button>
          )}
          <button className="icon-btn" type="button" title="查看分片" onClick={() => onViewChunks(task)}>
            <Eye size={16} />
          </button>
          <button className="icon-btn-danger" type="button" title="删除" onClick={() => onDelete(task.id)}>
            {task.status === "CANCELED" ? <CircleOff size={16} /> : <Trash2 size={16} />}
          </button>
        </div>
      </div>
    </motion.article>
  );
}
