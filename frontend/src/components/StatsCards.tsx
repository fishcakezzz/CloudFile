import { motion } from "framer-motion";
import { Activity, CheckCircle2, Clock3, Gauge, Sparkles, XCircle } from "lucide-react";
import type { UploadStats } from "../types";

interface StatsCardsProps {
  stats: UploadStats;
}

const items = [
  { key: "todayUploads", label: "今日上传数", icon: Activity, suffix: "" },
  { key: "successRate", label: "上传成功率", icon: Gauge, suffix: "%" },
  { key: "instantHits", label: "秒传命中数", icon: Sparkles, suffix: "" },
  { key: "uploadingTasks", label: "上传中任务数", icon: Clock3, suffix: "" },
  { key: "mediaProcessingTasks", label: "媒体处理中", icon: CheckCircle2, suffix: "" },
  { key: "failedTasks", label: "失败任务数", icon: XCircle, suffix: "" },
] as const;

export function StatsCards({ stats }: StatsCardsProps) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-6">
      {items.map((item, index) => {
        const Icon = item.icon;
        const value = stats[item.key];
        return (
          <motion.div
            key={item.key}
            initial={{ opacity: 0, y: 14 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: index * 0.04 }}
            className="rounded-lg border border-white/80 bg-white/85 p-4 shadow-soft backdrop-blur"
          >
            <div className="flex items-center justify-between">
              <div className="text-xs font-medium text-slate-500">{item.label}</div>
              <div className="rounded-lg bg-indigo-50 p-2 text-indigo-600">
                <Icon size={16} />
              </div>
            </div>
            <div className="mt-3 text-2xl font-semibold text-slate-900">
              {value}
              {item.suffix}
            </div>
          </motion.div>
        );
      })}
    </div>
  );
}
