import { AnimatePresence, motion } from "framer-motion";
import { FileStack, X } from "lucide-react";
import type { ChunkStatus, UploadTask } from "../types";

interface ChunkDrawerProps {
  task: UploadTask | null;
  onClose: () => void;
}

const chunkStyle: Record<ChunkStatus, string> = {
  UPLOADED: "bg-emerald-500",
  UPLOADING: "bg-indigo-500 animate-pulse",
  PENDING: "bg-slate-200",
  FAILED: "bg-rose-500",
};

const chunkLabel: Record<ChunkStatus, string> = {
  UPLOADED: "已上传",
  UPLOADING: "上传中",
  PENDING: "待上传",
  FAILED: "失败",
};

export function ChunkDrawer({ task, onClose }: ChunkDrawerProps) {
  return (
    <AnimatePresence>
      {task && (
        <>
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-40 bg-slate-950/20 backdrop-blur-sm"
            onClick={onClose}
          />
          <motion.aside
            initial={{ x: 420 }}
            animate={{ x: 0 }}
            exit={{ x: 420 }}
            transition={{ type: "spring", damping: 28, stiffness: 220 }}
            className="fixed bottom-0 right-0 top-0 z-50 w-full max-w-md overflow-y-auto border-l border-slate-200 bg-white p-5 shadow-2xl"
          >
            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="flex items-center gap-2 text-sm font-semibold text-slate-950">
                  <FileStack size={18} className="text-indigo-600" />
                  分片详情
                </div>
                <p className="mt-2 text-sm text-slate-600">{task.fileName}</p>
              </div>
              <button className="icon-btn" type="button" onClick={onClose} title="关闭">
                <X size={17} />
              </button>
            </div>

            <div className="mt-5 grid grid-cols-2 gap-3 text-sm">
              <div className="rounded-lg bg-slate-50 p-3">
                <div className="text-xs text-slate-500">Upload ID</div>
                <div className="mt-1 truncate font-medium text-slate-800">{task.uploadId || "-"}</div>
              </div>
              <div className="rounded-lg bg-slate-50 p-3">
                <div className="text-xs text-slate-500">Hash</div>
                <div className="mt-1 truncate font-medium text-slate-800">{task.hash || "-"}</div>
              </div>
              <div className="rounded-lg bg-slate-50 p-3">
                <div className="text-xs text-slate-500">已上传分片</div>
                <div className="mt-1 font-medium text-slate-800">{task.uploadedChunks} / {task.chunkCount}</div>
              </div>
              <div className="rounded-lg bg-slate-50 p-3">
                <div className="text-xs text-slate-500">状态</div>
                <div className="mt-1 font-medium text-slate-800">{task.status}</div>
              </div>
            </div>

            <div className="mt-5 flex flex-wrap gap-3 text-xs text-slate-600">
              {Object.entries(chunkLabel).map(([status, label]) => (
                <span key={status} className="flex items-center gap-1.5">
                  <span className={`h-2.5 w-2.5 rounded-full ${chunkStyle[status as ChunkStatus].replace(" animate-pulse", "")}`} />
                  {label}
                </span>
              ))}
            </div>

            <div className="mt-5 grid grid-cols-8 gap-2">
              {task.chunks.map((chunk) => (
                <motion.div
                  key={chunk.index}
                  layout
                  title={`chunk ${chunk.index}: ${chunkLabel[chunk.status]}`}
                  className={`grid aspect-square place-items-center rounded-md text-[10px] font-semibold text-white ${chunkStyle[chunk.status]} ${
                    chunk.status === "PENDING" ? "text-slate-500" : ""
                  }`}
                >
                  {chunk.index + 1}
                </motion.div>
              ))}
            </div>
          </motion.aside>
        </>
      )}
    </AnimatePresence>
  );
}
