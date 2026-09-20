import { useMemo, useState } from "react";
import type { DeadlineExtension } from "@/api/extension";
import { extensionApi } from "@/api/extension";
import StatusBadge from "@/components/StatusBadge";
import { useToast } from "@/components/Toast";
import { EXTENSION_MAX_DAYS } from "@/constants";
import { formatDate } from "@/utils/format";

interface ExtensionPanelProps {
  wishId: number;
  currentUserId?: number;
  ownerId: number;
  fulfillerId?: number;
  currentDeadline?: string | null;
  wishStatus: string;
  extension?: DeadlineExtension | null;
  onChanged: () => void;
}

function toISODay(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// 延期申请闭环面板：圆梦人提交、发布者批准/驳回、申请与处理结果展示（刷新后由详情接口回读）。
export default function ExtensionPanel({
  wishId,
  currentUserId,
  ownerId,
  fulfillerId,
  currentDeadline,
  wishStatus,
  extension,
  onChanged,
}: ExtensionPanelProps) {
  const toast = useToast();
  const [showForm, setShowForm] = useState(false);
  const [newDeadline, setNewDeadline] = useState("");
  const [reason, setReason] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [processing, setProcessing] = useState(false);

  const isOwner = Boolean(currentUserId && currentUserId === ownerId);
  const isFulfiller = Boolean(fulfillerId && currentUserId === fulfillerId);
  const pending = extension?.status === "pending";

  // 日期边界：新日期须晚于当前截止日；不超过提交日起 90 天。
  const { minDate, maxDate, deadlinePassed } = useMemo(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    let min = "";
    let passed = false;
    if (currentDeadline) {
      const cur = new Date(`${currentDeadline}T00:00:00`);
      passed = cur.getTime() < today.getTime();
      const next = new Date(cur);
      next.setDate(next.getDate() + 1);
      min = toISODay(next);
    }
    const max = new Date(today);
    max.setDate(max.getDate() + EXTENSION_MAX_DAYS);
    return { minDate: min, maxDate: toISODay(max), deadlinePassed: passed };
  }, [currentDeadline]);

  // 圆梦人可提交：心愿已认领、未完成、有截止日且未过截止日、没有待审申请。
  const canSubmit =
    isFulfiller &&
    !!fulfillerId &&
    wishStatus !== "completed" &&
    !!currentDeadline &&
    !deadlinePassed &&
    !pending;

  const canReview = isOwner && pending && wishStatus !== "completed";

  const submit = async () => {
    if (!newDeadline || !reason.trim()) {
      toast.show("请选择新完成日期并填写延期原因", "error");
      return;
    }
    if (currentDeadline && newDeadline <= currentDeadline) {
      toast.show("新完成日期必须晚于当前截止日期", "error");
      return;
    }
    if (newDeadline > maxDate) {
      toast.show(`新完成日期不能超过提交日起 ${EXTENSION_MAX_DAYS} 天`, "error");
      return;
    }
    setSubmitting(true);
    try {
      await extensionApi.submit(wishId, { new_deadline: newDeadline, reason: reason.trim() });
      toast.show("延期申请已提交，等待发布者审核 ⏳");
      setShowForm(false);
      setReason("");
      setNewDeadline("");
      onChanged();
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setSubmitting(false);
    }
  };

  const review = async (approve: boolean) => {
    if (!extension) return;
    setProcessing(true);
    try {
      if (approve) {
        await extensionApi.approve(extension.id);
        toast.show("已批准，心愿截止时间已更新 ✅");
      } else {
        await extensionApi.reject(extension.id);
        toast.show("已驳回，圆梦人可重新提交申请");
      }
      onChanged();
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setProcessing(false);
    }
  };

  // 无关用户也能看到申请与处理结果；只有相关角色展示操作区。
  return (
    <div className="rounded-xl border border-amber-100 bg-amber-50/50 p-4">
      <div className="mb-3 flex items-center justify-between">
        <p className="text-sm font-medium text-amber-800">⏳ 延期申请</p>
        {extension && <StatusBadge status={extension.status} kind="extension" />}
      </div>

      {extension ? (
        <div className="space-y-2 text-sm text-gray-700">
          <p>
            <span className="text-gray-500">申请人：</span>
            {extension.applicant_name || `#${extension.applicant_id}`}
            <span className="ml-3 text-xs text-gray-400">提交于 {formatDate(extension.created_at)}</span>
          </p>
          <p>
            <span className="text-gray-500">截止时间：</span>
            <span className="line-through decoration-rose-400">{extension.current_deadline || currentDeadline || "不限时"}</span>
            <span className="mx-2">→</span>
            <span className="font-medium text-emerald-700">{extension.new_deadline}</span>
          </p>
          <div className="rounded-lg bg-white p-3">
            <p className="text-xs text-gray-400">延期原因</p>
            <p className="mt-1 whitespace-pre-wrap text-gray-700">{extension.reason}</p>
          </div>
          {extension.status !== "pending" && (
            <div className="rounded-lg bg-white p-3">
              <p className="text-xs text-gray-400">
                处理结果{extension.reviewed_at ? ` · ${formatDate(extension.reviewed_at)}` : ""}
              </p>
              <p className={`mt-1 font-medium ${extension.status === "approved" ? "text-emerald-700" : "text-rose-700"}`}>
                {extension.status === "approved" ? "已批准，心愿截止时间已更新" : "已驳回"}
              </p>
              {extension.review_note && <p className="mt-1 whitespace-pre-wrap text-gray-600">{extension.review_note}</p>}
            </div>
          )}
          {canReview && (
            <div className="flex gap-3 pt-1">
              <button className="btn-primary !py-1.5" disabled={processing} onClick={() => review(true)}>
                批准
              </button>
              <button className="btn-secondary !py-1.5" disabled={processing} onClick={() => review(false)}>
                驳回
              </button>
            </div>
          )}
        </div>
      ) : (
        <p className="text-sm text-gray-400">暂无延期申请</p>
      )}

      {canSubmit && (
        <div className="mt-3 border-t border-amber-100 pt-3">
          {!showForm ? (
            <button className="btn-secondary !py-1.5 text-sm" onClick={() => setShowForm(true)}>
              申请延期
            </button>
          ) : (
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2 text-xs text-gray-500">
                <span>新完成日期：</span>
                <input
                  type="date"
                  className="input !py-1 !text-sm"
                  value={newDeadline}
                  min={minDate}
                  max={maxDate}
                  onChange={(e) => setNewDeadline(e.target.value)}
                />
                <span>（晚于当前截止日，最晚 {maxDate}）</span>
              </div>
              <textarea
                className="input min-h-[60px] !text-sm"
                value={reason}
                maxLength={500}
                onChange={(e) => setReason(e.target.value)}
                placeholder="请说明延期原因（2-500 字），需在当前截止日前提交"
              />
              <div className="flex gap-3">
                <button className="btn-primary !py-1.5 !text-sm" disabled={submitting} onClick={submit}>
                  {submitting ? "提交中..." : "提交申请"}
                </button>
                <button
                  className="btn-secondary !py-1.5 !text-sm"
                  disabled={submitting}
                  onClick={() => {
                    setShowForm(false);
                    setReason("");
                    setNewDeadline("");
                  }}
                >
                  取消
                </button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
