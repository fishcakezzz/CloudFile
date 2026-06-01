import { Archive, CheckCheck, FileUp, FolderUp, Image, RotateCcw, UploadCloud, Video } from "lucide-react";
import type { UploadAction, UploadKind } from "../types";

export const uploadActions: UploadAction[] = [
  { kind: "general", label: "通用文件上传", hint: "适合任意大文件", icon: FileUp },
  { kind: "image", label: "图片上传", hint: "自动压缩与缩略图", accept: "image/*", icon: Image },
  { kind: "video", label: "视频上传", hint: "模拟转码与封面", accept: "video/*", icon: Video },
  {
    kind: "archive",
    label: "压缩包上传",
    hint: "保持原始文件结构",
    accept:
      ".zip,.rar,.7z,.tar,.gz,.tgz,.tar.gz,.bz2,.xz,application/zip,application/x-zip-compressed,application/x-rar-compressed,application/x-7z-compressed,application/gzip,application/x-tar",
    icon: Archive,
  },
  { kind: "batch", label: "批量上传", hint: "多文件队列调度", icon: FolderUp },
  { kind: "instant", label: "秒传检测", hint: "仅创建用户引用", icon: CheckCheck },
  { kind: "resume", label: "恢复未完成任务", hint: "查询已上传分片", icon: RotateCcw },
];

interface UploadPanelProps {
  selectedKind: UploadKind;
  onSelect: (kind: UploadKind) => void;
  onRecover: () => void;
}

export function UploadPanel({ selectedKind, onSelect, onRecover }: UploadPanelProps) {
  return (
    <aside className="space-y-3">
      <div className="rounded-lg border border-white/80 bg-white/90 p-4 shadow-soft">
        <div className="mb-4 flex items-center gap-2 text-sm font-semibold text-slate-900">
          <UploadCloud size={18} className="text-indigo-600" />
          关键操作
        </div>
        <div className="space-y-2">
          {uploadActions.map((action) => {
            const Icon = action.icon;
            const active = selectedKind === action.kind;
            const handleClick = () => {
              onSelect(action.kind);
              if (action.kind === "resume") onRecover();
            };
            return (
              <button
                key={action.kind}
                type="button"
                onClick={handleClick}
                className={`w-full rounded-lg border p-3 text-left transition ${
                  active
                    ? "border-indigo-200 bg-indigo-50 text-indigo-900 shadow-sm"
                    : "border-slate-200 bg-white text-slate-700 hover:border-indigo-200 hover:bg-slate-50"
                }`}
              >
                <div className="flex items-center gap-3">
                  <span className="rounded-lg bg-white p-2 text-indigo-600 shadow-sm">
                    <Icon size={17} />
                  </span>
                  <span className="min-w-0">
                    <span className="block text-sm font-semibold">{action.label}</span>
                    <span className="mt-0.5 block text-xs text-slate-500">{action.hint}</span>
                  </span>
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </aside>
  );
}
