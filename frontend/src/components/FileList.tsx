import { motion } from "framer-motion";
import { Copy, Download, Eye, Search, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import type { FileItem, UploadStatus } from "../types";

interface FileListProps {
  files: FileItem[];
  onDelete: (id: string) => void;
  onView: (id: string) => void;
  onToast: (message: string, tone?: "success" | "info" | "warning") => void;
}

type SortKey = "time-desc" | "time-asc" | "size-desc" | "size-asc";

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

function statusText(status: UploadStatus) {
  if (status === "MEDIA_SUCCESS") return "处理完成";
  if (status === "INSTANT_SUCCESS") return "秒传成功";
  if (status === "SUCCESS") return "上传完成";
  return status;
}

export function FileList({ files, onDelete, onView, onToast }: FileListProps) {
  const [query, setQuery] = useState("");
  const [typeFilter, setTypeFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState("all");
  const [sortKey, setSortKey] = useState<SortKey>("time-desc");

  const filtered = useMemo(() => {
    return files
      .filter((file) => file.name.toLowerCase().includes(query.toLowerCase()))
      .filter((file) => typeFilter === "all" || file.type.startsWith(typeFilter))
      .filter((file) => statusFilter === "all" || file.status === statusFilter)
      .sort((a, b) => {
        if (sortKey === "time-desc") return b.uploadedAt - a.uploadedAt;
        if (sortKey === "time-asc") return a.uploadedAt - b.uploadedAt;
        if (sortKey === "size-desc") return b.size - a.size;
        return a.size - b.size;
      });
  }, [files, query, sortKey, statusFilter, typeFilter]);

  const copyLink = async (link: string) => {
    await navigator.clipboard?.writeText(link);
    onToast("文件链接已复制", "success");
  };

  return (
    <section className="rounded-lg border border-white/80 bg-white/90 p-4 shadow-soft">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h2 className="text-base font-semibold text-slate-950">文件列表</h2>
          <p className="mt-1 text-xs text-slate-500">按类型、状态、时间和大小筛选已经进入用户文件视图的文件。</p>
        </div>
        <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          <label className="relative">
            <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索文件"
              className="h-10 w-full rounded-lg border border-slate-200 bg-white pl-9 pr-3 text-sm outline-none transition focus:border-indigo-300"
            />
          </label>
          <select className="filter-select" value={typeFilter} onChange={(event) => setTypeFilter(event.target.value)}>
            <option value="all">全部类型</option>
            <option value="image/">图片</option>
            <option value="video/">视频</option>
            <option value="application/">压缩包/文档</option>
          </select>
          <select className="filter-select" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
            <option value="all">全部状态</option>
            <option value="INSTANT_SUCCESS">秒传成功</option>
            <option value="SUCCESS">上传完成</option>
            <option value="MEDIA_SUCCESS">处理完成</option>
          </select>
          <select className="filter-select" value={sortKey} onChange={(event) => setSortKey(event.target.value as SortKey)}>
            <option value="time-desc">上传时间 新到旧</option>
            <option value="time-asc">上传时间 旧到新</option>
            <option value="size-desc">文件大小 大到小</option>
            <option value="size-asc">文件大小 小到大</option>
          </select>
        </div>
      </div>

      <div className="mt-4 overflow-hidden rounded-lg border border-slate-200">
        <div className="grid grid-cols-[1.4fr_0.9fr_0.7fr_0.8fr_140px] bg-slate-50 px-4 py-3 text-xs font-semibold text-slate-500 max-lg:hidden">
          <span>文件名</span>
          <span>类型</span>
          <span>大小</span>
          <span>状态</span>
          <span className="text-right">操作</span>
        </div>
        <div className="divide-y divide-slate-100">
          {filtered.map((file) => (
            <motion.div
              key={file.id}
              layout
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              className="grid gap-3 px-4 py-3 text-sm text-slate-700 lg:grid-cols-[1.4fr_0.9fr_0.7fr_0.8fr_140px] lg:items-center"
            >
              <div className="min-w-0">
                <div className="truncate font-medium text-slate-950">{file.name}</div>
                <div className="mt-1 text-xs text-slate-500">{new Date(file.uploadedAt).toLocaleString()}</div>
              </div>
              <div className="truncate text-xs">{file.type || "unknown"}</div>
              <div>{formatBytes(file.size)}</div>
              <div>
                <span className="rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-semibold text-emerald-700">
                  {statusText(file.status)}
                </span>
              </div>
              <div className="flex justify-start gap-2 lg:justify-end">
                <button className="icon-btn" type="button" title="查看详情" onClick={() => onView(file.taskId)}>
                  <Eye size={15} />
                </button>
                <button className="icon-btn" type="button" title="复制链接" onClick={() => copyLink(file.link)}>
                  <Copy size={15} />
                </button>
                <a className="icon-btn" title="下载" href={file.link} download={file.name}>
                  <Download size={15} />
                </a>
                <button className="icon-btn-danger" type="button" title="删除" onClick={() => onDelete(file.id)}>
                  <Trash2 size={15} />
                </button>
              </div>
            </motion.div>
          ))}
          {filtered.length === 0 && (
            <div className="px-4 py-8 text-center text-sm text-slate-500">暂无匹配文件</div>
          )}
        </div>
      </div>
    </section>
  );
}
