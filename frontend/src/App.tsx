import { AnimatePresence, motion } from "framer-motion";
import {
  Bell,
  Cloud,
  CloudOff,
  Database,
  HardDrive,
  Layers3,
  MessageSquare,
  Server,
  ShieldCheck,
  UserCircle2,
  X,
} from "lucide-react";
import { ChangeEvent, DragEvent, useEffect, useMemo, useRef, useState } from "react";
import { ChunkDrawer } from "./components/ChunkDrawer";
import { FileList } from "./components/FileList";
import { StatsCards } from "./components/StatsCards";
import { UploadPanel, uploadActions } from "./components/UploadPanel";
import { UploadTaskCard } from "./components/UploadTaskCard";
import {
  calculateFileHash,
  deleteUserFileRef,
  initUpload,
  listFiles,
  mergeChunks,
  processMedia,
  queryUploadStatus,
  recoverUnfinishedTasks,
  uploadChunk,
} from "./mockApi";
import type { Chunk, FileItem, UploadKind, UploadStats, UploadTask } from "./types";

interface Toast {
  id: string;
  message: string;
  tone: "success" | "info" | "warning";
}

interface ConfirmState {
  title: string;
  description: string;
  onConfirm: () => void;
}

const activeStatuses = new Set(["HASHING", "UPLOADING", "MERGING", "MEDIA_PROCESSING"]);

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

function isMedia(task: UploadTask) {
  return task.fileType.startsWith("image/") || task.fileType.startsWith("video/");
}

function mediaText(task: UploadTask) {
  if (task.fileType.startsWith("video/")) return "正在视频转码 / 正在生成缩略图";
  if (task.fileType.startsWith("image/")) return "正在生成缩略图 / 正在图片压缩";
  return "";
}

function makeChunks(count: number): Chunk[] {
  return Array.from({ length: count }, (_, index) => ({ index, status: "PENDING" }));
}

export default function App() {
  const [selectedKind, setSelectedKind] = useState<UploadKind>("general");
  const [tasks, setTasks] = useState<UploadTask[]>([]);
  const [files, setFiles] = useState<FileItem[]>([]);
  const [networkOnline, setNetworkOnline] = useState(true);
  const [dragActive, setDragActive] = useState(false);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [confirmState, setConfirmState] = useState<ConfirmState | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const tasksRef = useRef<UploadTask[]>([]);
  const timers = useRef<Record<string, number>>({});

  function pickFiles(kind: UploadKind = selectedKind) {
    setSelectedKind(kind);
    window.setTimeout(() => fileInputRef.current?.click(), 0);
  }

  useEffect(() => {
    tasksRef.current = tasks;
  }, [tasks]);

  useEffect(() => {
    return () => {
      Object.values(timers.current).forEach((timer) => window.clearTimeout(timer));
    };
  }, []);

  useEffect(() => {
    refreshFiles();
  }, []);

  const selectedTask = tasks.find((task) => task.id === selectedTaskId) || null;
  const selectedAction = uploadActions.find((action) => action.kind === selectedKind)!;
  const SelectedActionIcon = selectedAction.icon;

  const stats: UploadStats = useMemo(() => {
    const complete = files.length;
    const failed = tasks.filter((task) => task.status === "FAILED").length;
    const totalFinished = complete + failed;
    return {
      todayUploads: files.length + tasks.filter((task) => task.createdAt > Date.now() - 86_400_000).length,
      successRate: totalFinished === 0 ? 100 : Math.round((complete / totalFinished) * 100),
      instantHits: tasks.filter((task) => task.status === "INSTANT_SUCCESS").length,
      uploadingTasks: tasks.filter((task) => task.status === "UPLOADING").length,
      mediaProcessingTasks: tasks.filter((task) => task.status === "MEDIA_PROCESSING").length,
      failedTasks: failed,
    };
  }, [files.length, tasks]);

  function toast(message: string, tone: Toast["tone"] = "info") {
    const id = crypto.randomUUID();
    setToasts((current) => [...current, { id, message, tone }]);
    window.setTimeout(() => {
      setToasts((current) => current.filter((item) => item.id !== id));
    }, 3200);
  }

  function updateTask(id: string, patch: Partial<UploadTask> | ((task: UploadTask) => Partial<UploadTask>)) {
    setTasks((current) =>
      current.map((task) => {
        if (task.id !== id) return task;
        const nextPatch = typeof patch === "function" ? patch(task) : patch;
        return { ...task, ...nextPatch };
      }),
    );
  }

  function addFileFromTask(task: UploadTask, finalStatus = task.status) {
    const link = task.link || `https://cdn.cloudfile.local/${task.hash || task.id}/${encodeURIComponent(task.fileName)}`;
    setFiles((current) => {
      if (current.some((file) => file.taskId === task.id)) return current;
      return [
        {
          id: crypto.randomUUID(),
          taskId: task.id,
          name: task.fileName,
          type: task.fileType,
          size: task.size,
          status: finalStatus,
          uploadedAt: Date.now(),
          link,
        },
        ...current,
      ];
    });
  }

  async function refreshFiles() {
    try {
      setFiles(await listFiles());
    } catch (error) {
      toast(error instanceof Error ? error.message : "加载文件列表失败", "warning");
    }
  }

  async function handleFiles(fileList: globalThis.FileList | File[]) {
    const incoming = Array.from(fileList as ArrayLike<File>);
    if (incoming.length === 0) return;

    for (const file of incoming) {
      const chunkCount = Math.max(1, Math.ceil(file.size / (10 * 1024 * 1024)));
      const task: UploadTask = {
        id: crypto.randomUUID(),
        file,
        fileName: file.name,
        fileType: file.type || "application/octet-stream",
        size: file.size,
        kind: selectedKind,
        status: "HASHING",
        progress: 4,
        uploadedChunks: 0,
        chunkCount,
        chunks: makeChunks(chunkCount),
        speed: 0,
        remainingSeconds: 0,
        message: "正在计算文件 Hash，用于秒传校验",
        createdAt: Date.now(),
      };
      setTasks((current) => [task, ...current]);
      prepareUpload(task);
    }
  }

  async function prepareUpload(task: UploadTask) {
    try {
      const hash = await calculateFileHash(task.file);
      updateTask(task.id, { hash, progress: 8, message: "Hash 计算完成，正在初始化上传任务" });
      const result = await initUpload(task.file, hash, task.kind === "instant");

      if (result.instant) {
        const nextTask: UploadTask = {
          ...task,
          hash,
          uploadId: result.uploadId,
          status: "INSTANT_SUCCESS",
          progress: 100,
          uploadedChunks: result.chunkCount,
          chunkCount: result.chunkCount,
          chunks: makeChunks(result.chunkCount).map((chunk) => ({ ...chunk, status: "UPLOADED" })),
          speed: 0,
          remainingSeconds: 0,
          message: "该文件已上传，已为你秒传完成",
          link: `https://cdn.cloudfile.local/${hash}/${encodeURIComponent(task.fileName)}`,
        };
        updateTask(task.id, nextTask);
        await refreshFiles();
        toast("秒传成功：该文件已上传，已为你秒传完成", "success");
        return;
      }

      updateTask(task.id, {
        hash,
        uploadId: result.uploadId,
        status: networkOnline ? "UPLOADING" : "OFFLINE",
        progress: 10,
        chunkCount: result.chunkCount,
        chunks: makeChunks(result.chunkCount),
        message: networkOnline ? "上传链路已建立，正在上传分片" : "网络已断开，进度已保存",
      });
      if (networkOnline) startUploading(task.id);
    } catch (error) {
      updateTask(task.id, { status: "FAILED", message: error instanceof Error ? error.message : "初始化失败" });
    }
  }

  function startUploading(taskId: string) {
    window.clearTimeout(timers.current[taskId]);
    timers.current[taskId] = window.setTimeout(async () => {
      const task = tasksRef.current.find((item) => item.id === taskId);
      if (!task || task.status !== "UPLOADING") return;
      if (!networkOnline) {
        updateTask(taskId, { status: "OFFLINE", speed: 0, message: "网络已断开，进度已保存" });
        return;
      }

      const nextChunk = task.chunks.find((chunk) => chunk.status === "PENDING" || chunk.status === "FAILED");
      if (!nextChunk) {
        await finishUpload(task);
        return;
      }

      updateTask(taskId, (current) => ({
        chunks: current.chunks.map((chunk) =>
          chunk.index === nextChunk.index ? { ...chunk, status: "UPLOADING" } : chunk,
        ),
        message: `正在上传分片 ${nextChunk.index + 1} / ${current.chunkCount}`,
      }));

      try {
        await uploadChunk(task, nextChunk.index);
        updateTask(taskId, (current) => {
          const uploadedChunks = current.uploadedChunks + 1;
          const progress = Math.min(92, 10 + (uploadedChunks / current.chunkCount) * 82);
          const speed = 3_000_000 + Math.random() * 9_000_000;
          const remainingBytes = current.size * (1 - progress / 100);
          return {
            uploadedChunks,
            progress,
            speed,
            remainingSeconds: remainingBytes / speed,
            chunks: current.chunks.map((chunk) =>
              chunk.index === nextChunk.index ? { ...chunk, status: "UPLOADED" } : chunk,
            ),
          };
        });
        startUploading(taskId);
      } catch (error) {
        updateTask(taskId, (current) => ({
          status: "FAILED",
          speed: 0,
          message: error instanceof Error ? error.message : "分片上传失败",
          chunks: current.chunks.map((chunk) =>
            chunk.index === nextChunk.index ? { ...chunk, status: "FAILED" } : chunk,
          ),
        }));
      }
    }, 240);
  }

  async function finishUpload(taskSnapshot: UploadTask) {
    updateTask(taskSnapshot.id, {
      status: "MERGING",
      progress: 96,
      speed: 0,
      remainingSeconds: 0,
      message: "分片上传完成，服务端正在合并文件",
    });
    await mergeChunks(taskSnapshot);
    const latest = tasksRef.current.find((task) => task.id === taskSnapshot.id);
    if (!latest || latest.status === "CANCELED") return;

    if (isMedia(latest)) {
      updateTask(latest.id, {
        status: "MEDIA_PROCESSING",
        progress: 98,
        mediaText: mediaText(latest),
        message: mediaText(latest),
      });
      await processMedia(latest);
      const done = { ...latest, status: "MEDIA_SUCCESS" as const, progress: 100, message: "媒体处理完成" };
      updateTask(latest.id, done);
      await refreshFiles();
      toast("媒体处理完成，文件已可用", "success");
      return;
    }

    const done = { ...latest, status: "SUCCESS" as const, progress: 100, message: "上传完成，文件已写入用户文件列表" };
    updateTask(latest.id, done);
    await refreshFiles();
    toast("上传完成，服务端合并成功", "success");
  }

  function pauseTask(id: string) {
    updateTask(id, { status: "PAUSED", speed: 0, message: "上传已暂停，分片状态已保存" });
  }

  async function resumeTask(id: string, forceOnline = false) {
    const task = tasksRef.current.find((item) => item.id === id);
    if (!task) return;
    if (!networkOnline && !forceOnline) {
      updateTask(id, { status: "OFFLINE", message: "网络已断开，进度已保存" });
      return;
    }
    const uploadedIndexes = await queryUploadStatus(task);
    updateTask(id, (current) => ({
      status: "UPLOADING",
      uploadedChunks: uploadedIndexes.length,
      progress: Math.max(current.progress, 10 + (uploadedIndexes.length / current.chunkCount) * 82),
      message: "网络恢复，已查询服务端分片状态，只继续上传缺失分片",
      chunks: current.chunks.map((chunk) =>
        uploadedIndexes.includes(chunk.index) ? { ...chunk, status: "UPLOADED" } : chunk.status === "UPLOADING" ? { ...chunk, status: "PENDING" } : chunk,
      ),
    }));
    startUploading(id);
  }

  function retryTask(id: string) {
    updateTask(id, (current) => ({
      status: "UPLOADING",
      message: "正在重试失败分片",
      chunks: current.chunks.map((chunk) => (chunk.status === "UPLOADING" ? { ...chunk, status: "PENDING" } : chunk)),
    }));
    startUploading(id);
  }

  function cancelTask(id: string) {
    updateTask(id, { status: "CANCELED", speed: 0, message: "任务已取消" });
  }

  function requestDeleteTask(id: string) {
    setConfirmState({
      title: "删除上传任务",
      description: "该操作只会从当前界面移除上传任务记录。若要删除已上传的物理文件，请在文件列表中删除。",
      onConfirm: async () => {
        setTasks((current) => current.filter((task) => task.id !== id));
        setFiles((current) => current.filter((file) => file.taskId !== id));
        setSelectedTaskId((current) => (current === id ? null : current));
        toast("已删除当前上传任务记录", "success");
      },
    });
  }

  function requestDeleteFile(fileId: string) {
    setConfirmState({
      title: "删除物理文件",
      description: "该操作将删除底层物理文件、文件列表引用和关联媒体任务，删除后无法下载。",
      onConfirm: async () => {
        await deleteUserFileRef(fileId);
        await refreshFiles();
        toast("物理文件已删除", "success");
      },
    });
  }

  async function recoverTasks() {
    const count = await recoverUnfinishedTasks();
    toast(`已扫描未完成任务，发现 ${count} 个可恢复分片任务`, "info");
    tasksRef.current
      .filter((task) => ["PAUSED", "OFFLINE", "FAILED"].includes(task.status))
      .forEach((task) => resumeTask(task.id));
  }

  function toggleNetwork() {
    setNetworkOnline((online) => {
      const next = !online;
      if (!next) {
        tasksRef.current
          .filter((task) => task.status === "UPLOADING")
          .forEach((task) => updateTask(task.id, { status: "OFFLINE", speed: 0, message: "网络已断开，进度已保存" }));
        toast("网络已断开，上传进度保持在当前分片状态", "warning");
      } else {
        toast("网络已恢复，正在查询已上传分片", "success");
        tasksRef.current.filter((task) => task.status === "OFFLINE").forEach((task) => resumeTask(task.id, true));
      }
      return next;
    });
  }

  function onInputChange(event: ChangeEvent<HTMLInputElement>) {
    if (event.target.files) handleFiles(event.target.files);
    event.target.value = "";
  }

  function onDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragActive(false);
    handleFiles(event.dataTransfer.files);
  }

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 border-b border-white/70 bg-white/80 backdrop-blur-xl">
        <div className="mx-auto flex max-w-[1600px] items-center justify-between gap-4 px-4 py-3">
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-lg bg-gradient-to-br from-blue-600 to-violet-600 text-white shadow-soft">
              <Cloud size={21} />
            </div>
            <div>
              <div className="text-lg font-bold text-slate-950">CloudFile</div>
              <div className="text-xs text-slate-500">大文件生命周期调度控制台</div>
            </div>
          </div>

          <div className="hidden items-center gap-2 rounded-full border border-slate-200 bg-white px-3 py-2 text-xs text-slate-600 shadow-sm lg:flex">
            <Server size={14} /> Gin
            <Database size={14} /> MySQL
            <ShieldCheck size={14} /> Redis CAS
            <HardDrive size={14} /> MinIO
            <MessageSquare size={14} /> RabbitMQ
            <Layers3 size={14} /> FFmpeg
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={toggleNetwork}
              className={`flex items-center gap-2 rounded-full px-3 py-2 text-xs font-semibold transition ${
                networkOnline ? "bg-emerald-50 text-emerald-700" : "animate-pulse bg-orange-100 text-orange-700"
              }`}
            >
              {networkOnline ? <Cloud size={15} /> : <CloudOff size={15} />}
              {networkOnline ? "网络正常" : "网络断开"}
            </button>
            <button className="icon-btn" type="button" title="通知">
              <Bell size={17} />
            </button>
            <div className="hidden items-center gap-2 rounded-full bg-slate-100 px-3 py-2 text-sm font-medium text-slate-700 sm:flex">
              <UserCircle2 size={18} />
              user_10001
            </div>
          </div>
        </div>
      </header>

      <main className="mx-auto grid max-w-[1600px] gap-4 px-4 py-5 xl:grid-cols-[280px_minmax(0,1fr)]">
        <UploadPanel
          selectedKind={selectedKind}
          onSelect={setSelectedKind}
          onPickFiles={pickFiles}
          onRecover={recoverTasks}
        />

        <section className="min-w-0 space-y-4">
          <StatsCards stats={stats} />

          <div
            onDragOver={(event) => {
              event.preventDefault();
              setDragActive(true);
            }}
            onDragLeave={() => setDragActive(false)}
            onDrop={onDrop}
            className={`rounded-lg border-2 border-dashed p-6 shadow-soft transition ${
              dragActive ? "border-indigo-400 bg-indigo-50" : "border-slate-200 bg-white/90"
            }`}
          >
            <input
              ref={fileInputRef}
              type="file"
              multiple={selectedKind === "batch"}
              accept={selectedAction.accept}
              className="hidden"
              onChange={onInputChange}
            />
            <div className="flex flex-col items-center justify-center gap-3 py-6 text-center">
              <motion.div
                animate={{ y: dragActive ? -4 : 0, scale: dragActive ? 1.04 : 1 }}
                className="grid h-14 w-14 place-items-center rounded-lg bg-gradient-to-br from-blue-600 to-violet-600 text-white shadow-soft"
              >
                <SelectedActionIcon size={26} />
              </motion.div>
              <div>
                <h1 className="text-xl font-semibold text-slate-950">{selectedAction.label}</h1>
                <p className="mt-2 max-w-2xl text-sm text-slate-600">
                  {selectedAction.hint}。文件会先计算 Hash，再执行秒传校验；未命中时进入分片上传、断点续传、幂等合并和异步处理流程。
                </p>
              </div>
              <button
                type="button"
                onClick={() => fileInputRef.current?.click()}
                className="rounded-lg bg-slate-950 px-4 py-2 text-sm font-semibold text-white shadow-soft transition hover:bg-indigo-700"
              >
                选择文件
              </button>
            </div>
          </div>

          <section className="rounded-lg border border-white/80 bg-white/70 p-4 shadow-soft">
            <div className="mb-4 flex items-center justify-between gap-3">
              <div>
                <h2 className="text-base font-semibold text-slate-950">上传任务队列</h2>
                <p className="mt-1 text-xs text-slate-500">任务卡片展示 Hash、秒传、分片、合并、媒体处理和失败恢复状态。</p>
              </div>
              <span className="rounded-full bg-slate-100 px-3 py-1 text-xs font-medium text-slate-600">
                {tasks.filter((task) => activeStatuses.has(task.status)).length} 个活跃任务
              </span>
            </div>
            <div className="space-y-3">
              <AnimatePresence>
                {tasks.map((task) => (
                  <UploadTaskCard
                    key={task.id}
                    task={task}
                    onPause={pauseTask}
                    onResume={resumeTask}
                    onCancel={cancelTask}
                    onRetry={retryTask}
                    onDelete={requestDeleteTask}
                    onViewChunks={(item) => setSelectedTaskId(item.id)}
                  />
                ))}
              </AnimatePresence>
              {tasks.length === 0 && (
                <div className="rounded-lg border border-dashed border-slate-200 bg-white/70 px-4 py-10 text-center text-sm text-slate-500">
                  暂无上传任务，拖拽文件到上方区域开始模拟完整上传链路。
                </div>
              )}
            </div>
          </section>

          <FileList
            files={files}
            onDelete={requestDeleteFile}
            onView={setSelectedTaskId}
            onToast={toast}
          />
        </section>
      </main>

      <ChunkDrawer task={selectedTask} onClose={() => setSelectedTaskId(null)} />

      <div className="fixed right-4 top-20 z-[70] space-y-2">
        <AnimatePresence>
          {toasts.map((item) => (
            <motion.div
              key={item.id}
              initial={{ opacity: 0, x: 24 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 24 }}
              className={`rounded-lg px-4 py-3 text-sm font-medium shadow-soft ${
                item.tone === "success"
                  ? "bg-emerald-600 text-white"
                  : item.tone === "warning"
                    ? "bg-orange-500 text-white"
                    : "bg-slate-950 text-white"
              }`}
            >
              {item.message}
            </motion.div>
          ))}
        </AnimatePresence>
      </div>

      <AnimatePresence>
        {confirmState && (
          <motion.div className="fixed inset-0 z-[80] grid place-items-center bg-slate-950/30 p-4 backdrop-blur-sm">
            <motion.div
              initial={{ opacity: 0, scale: 0.96, y: 12 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.96, y: 12 }}
              className="w-full max-w-md rounded-lg bg-white p-5 shadow-2xl"
            >
              <div className="flex items-start justify-between gap-4">
                <div>
                  <h3 className="text-base font-semibold text-slate-950">{confirmState.title}</h3>
                  <p className="mt-2 text-sm leading-6 text-slate-600">{confirmState.description}</p>
                </div>
                <button className="icon-btn" type="button" title="关闭" onClick={() => setConfirmState(null)}>
                  <X size={16} />
                </button>
              </div>
              <div className="mt-5 flex justify-end gap-2">
                <button
                  type="button"
                  className="rounded-lg border border-slate-200 px-4 py-2 text-sm font-semibold text-slate-700 hover:bg-slate-50"
                  onClick={() => setConfirmState(null)}
                >
                  取消
                </button>
                <button
                  type="button"
                  className="rounded-lg bg-rose-600 px-4 py-2 text-sm font-semibold text-white hover:bg-rose-700"
                  onClick={() => {
                    confirmState.onConfirm();
                    setConfirmState(null);
                  }}
                >
                  确认删除
                </button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}
