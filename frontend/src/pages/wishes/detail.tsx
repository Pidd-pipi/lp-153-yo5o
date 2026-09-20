import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/router";
import type { Blessing } from "@/api/blessing";
import type { WishDetail } from "@/api/wish";
import { wishApi } from "@/api/wish";
import { blessingApi } from "@/api/blessing";
import { claimApi } from "@/api/claim";
import type { WishExtension } from "@/api/extension";
import { extensionApi } from "@/api/extension";
import GiftPicker from "@/components/GiftPicker";
import ProgressBar from "@/components/ProgressBar";
import StatusBadge from "@/components/StatusBadge";
import { useToast } from "@/components/Toast";
import { useAuth } from "@/hooks/useAuth";
import { EXTENSION_STATUS_STYLE } from "@/constants";
import { formatCategory, formatDate, formatDeadline, formatDifficulty, formatExtensionStatus } from "@/utils/format";

// toDateInputValue 把 Date 格式化为 date 输入框的 YYYY-MM-DD。
function toDateInputValue(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

// addDaysToDateStr 在 YYYY-MM-DD 日期串上加减天数（避免时区偏移）。
function addDaysToDateStr(dateStr: string, days: number): string {
  const [y, m, d] = dateStr.slice(0, 10).split("-").map(Number);
  if (!y || !m || !d) return "";
  return toDateInputValue(new Date(y, m - 1, d + days));
}

export default function WishDetailPage() {
  const router = useRouter();
  const { user, isAuthed } = useAuth();
  const toast = useToast();
  const id = Number(router.query.id);
  const [wish, setWish] = useState<WishDetail | null>(null);
  const [blessings, setBlessings] = useState<Blessing[]>([]);
  const [loading, setLoading] = useState(true);
  const [blessText, setBlessText] = useState("");
  const [gift, setGift] = useState("");
  const [progress, setProgress] = useState(0);
  const [note, setNote] = useState("");
  const [actionLoading, setActionLoading] = useState(false);
  const [extensions, setExtensions] = useState<WishExtension[]>([]);
  const [extDate, setExtDate] = useState("");
  const [extReason, setExtReason] = useState("");

  const load = useCallback(async (wishId: number) => {
    if (!wishId) return;
    setLoading(true);
    try {
      const detail = await wishApi.detail(wishId);
      setWish(detail);
      if (detail.claim) setProgress(detail.claim.progress);
      const bl = await blessingApi.list(wishId, { page: 1, page_size: 50 });
      setBlessings(bl.items);
      const exts = await extensionApi.listByWish(wishId);
      setExtensions(exts);
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    if (id) load(id);
  }, [id, load]);

  const claim = async () => {
    if (!isAuthed()) {
      toast.show("请先登录再认领", "error");
      return;
    }
    setActionLoading(true);
    try {
      await claimApi.claim(id);
      toast.show("认领成功，你已成为圆梦人 🤝");
      load(id);
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setActionLoading(false);
    }
  };

  const updateProgress = async (progressValue: number, isMilestone: boolean) => {
    if (!wish?.claim) return;
    setActionLoading(true);
    try {
      await claimApi.updateProgress(wish.claim.id, { progress: progressValue, note, is_milestone: isMilestone });
      toast.show(isMilestone ? "里程碑打卡成功 🎯" : "进度更新成功");
      setNote("");
      load(id);
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setActionLoading(false);
    }
  };

  const complete = async () => {
    if (!wish?.claim) return;
    setActionLoading(true);
    try {
      await claimApi.complete(wish.claim.id, { note });
      toast.show("心愿完成，进入庆祝时刻 🎉");
      setNote("");
      load(id);
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setActionLoading(false);
    }
  };

  const sendBlessing = async () => {
    if (!isAuthed()) {
      toast.show("请先登录再送祝福", "error");
      return;
    }
    if (!blessText.trim()) {
      toast.show("写点祝福内容吧", "error");
      return;
    }
    try {
      await blessingApi.create(id, { content: blessText, gift_emoji: gift || undefined });
      toast.show("祝福已送达 💌");
      setBlessText("");
      setGift("");
      const bl = await blessingApi.list(id, { page: 1, page_size: 50 });
      setBlessings(bl.items);
    } catch (e) {
      toast.show((e as Error).message, "error");
    }
  };

  const submitExtension = async () => {
    if (!extDate) {
      toast.show("请选择新的完成日期", "error");
      return;
    }
    if (extReason.trim().length < 2) {
      toast.show("请填写延期原因", "error");
      return;
    }
    setActionLoading(true);
    try {
      await extensionApi.submit(id, { new_deadline: `${extDate}T23:59:59+08:00`, reason: extReason.trim() });
      toast.show("延期申请已提交，等待心愿发布者审核 ⏳");
      setExtDate("");
      setExtReason("");
      load(id);
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setActionLoading(false);
    }
  };

  const reviewExtension = async (extensionId: number, action: "approve" | "reject") => {
    setActionLoading(true);
    try {
      await extensionApi.review(extensionId, action);
      toast.show(action === "approve" ? "已批准延期，心愿截止时间已更新 ✅" : "已驳回延期申请");
      load(id);
    } catch (e) {
      toast.show((e as Error).message, "error");
    } finally {
      setActionLoading(false);
    }
  };

  if (loading || !wish) {
    return <p className="py-16 text-center text-gray-400">加载中...</p>;
  }

  const isOwner = isAuthed() && wish.user_id === user?.id;
  const isFulfiller = Boolean(wish.claim);
  const isClaimOwner = isAuthed() && Boolean(wish.claim) && wish.claim?.user_id === user?.id;
  const pendingExt = extensions.find((e) => e.status === "pending");
  const deadlinePassed = Boolean(wish.expected_deadline) && new Date(`${wish.expected_deadline}T23:59:59`) < new Date();
  const canSubmitExtension = isClaimOwner && wish.status !== "completed" && !pendingExt && Boolean(wish.expected_deadline) && !deadlinePassed;
  const minExtDate = wish.expected_deadline ? addDaysToDateStr(wish.expected_deadline, 1) : "";
  const maxExtDate = (() => {
    const now = new Date();
    return toDateInputValue(new Date(now.getFullYear(), now.getMonth(), now.getDate() + 90));
  })();

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <div className={`card space-y-4 ${wish.status === "completed" ? "border-emerald-200 bg-gradient-to-br from-emerald-50 to-pink-50" : ""}`}>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex h-11 w-11 items-center justify-center rounded-full bg-gradient-to-br from-pink-400 to-purple-500 text-lg text-white">
              {wish.author_nickname ? wish.author_nickname.slice(0, 1) : "心"}
            </div>
            <div>
              <p className="font-medium text-gray-800">{wish.author_nickname || "神秘人"}</p>
              <p className="text-xs text-gray-400">{formatDate(wish.created_at)} · {formatCategory(wish.category)}</p>
            </div>
          </div>
          <StatusBadge status={wish.status} />
        </div>

        <h1 className="text-2xl font-bold text-gray-900">{wish.title}</h1>
        <p className="whitespace-pre-wrap text-gray-700">{wish.content}</p>

        {wish.image_urls.length > 0 && (
          <div className="grid gap-3 sm:grid-cols-2">
            {wish.image_urls.map((url) => <img key={url} src={url} className="h-48 w-full rounded-xl object-cover" alt="wish" />)}
          </div>
        )}

        <div className="flex flex-wrap gap-2 text-xs">
          <span className="rounded-full bg-sky-50 px-2 py-0.5 text-sky-600">{formatDifficulty(wish.difficulty)}</span>
          <span className="rounded-full bg-amber-50 px-2 py-0.5 text-amber-600">截止 {formatDeadline(wish.expected_deadline)}</span>
          <span className="rounded-full bg-purple-50 px-2 py-0.5 text-purple-600">❤️ {wish.likes_count}</span>
        </div>

        {wish.status === "completed" && (
          <div className="rounded-xl border border-emerald-200 bg-white p-4 text-center">
            <div className="text-3xl">🎉🎁🎈</div>
            <p className="mt-1 font-semibold text-emerald-700">心愿达成！进入庆祝时刻</p>
            {wish.completion_note && <p className="mt-1 text-sm text-gray-600">{wish.completion_note}</p>}
          </div>
        )}

        {wish.claim && (
          <div className="rounded-xl bg-purple-50 p-4">
            <div className="mb-2 flex items-center justify-between">
              <p className="text-sm font-medium text-purple-700">
                圆梦人：{wish.claim.fulfiller_name || `#${wish.claim.user_id}`} · 里程碑 {wish.claim.milestone_count} 次
              </p>
              <span className="text-xs text-purple-400">认领于 {formatDate(wish.claim.created_at)}</span>
            </div>
            <ProgressBar progress={wish.claim.progress} label="圆梦进度" />
            {wish.claim.latest_note && <p className="mt-2 text-sm text-gray-600">{wish.claim.latest_note}</p>}
          </div>
        )}

        {isFulfiller && wish.status !== "completed" && (
          <div className="space-y-3 rounded-xl border border-purple-100 bg-white p-4">
            <p className="text-sm font-medium text-gray-700">更新圆梦进度</p>
            <input type="range" min={0} max={100} value={progress} onChange={(e) => setProgress(Number(e.target.value))} className="w-full" />
            <div className="flex items-center gap-2 text-sm text-gray-500">
              <span>当前进度：{progress}%</span>
              <button className="btn-secondary !py-1 !px-3" disabled={actionLoading} onClick={() => updateProgress(progress, true)}>里程碑打卡 🎯</button>
            </div>
            <textarea className="input min-h-[60px]" value={note} onChange={(e) => setNote(e.target.value)} placeholder="记录进度说明/故事..." />
            <div className="flex gap-3">
              <button className="btn-secondary" disabled={actionLoading} onClick={() => updateProgress(progress, false)}>保存进度</button>
              <button className="btn-primary" disabled={actionLoading} onClick={complete}>标记完成 🎉</button>
            </div>
          </div>
        )}

        {!wish.claim && !isOwner && wish.status === "pending" && (
          <button className="btn-primary w-full" disabled={actionLoading} onClick={claim}>
            {actionLoading ? "认领中..." : "🤝 认领这个心愿，成为圆梦人"}
          </button>
        )}
      </div>

      {(wish.claim || extensions.length > 0) && (
        <div className="card space-y-4">
          <div className="flex items-center gap-2">
            <span className="text-xl">⏳</span>
            <h2 className="text-lg font-semibold text-gray-800">延期申请</h2>
            {pendingExt && <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700">待审核</span>}
          </div>

          {extensions.length === 0 && <p className="py-2 text-center text-sm text-gray-400">还没有延期申请</p>}

          <div className="space-y-3">
            {extensions.map((ext) => (
              <div key={ext.id} className="rounded-xl bg-purple-50/60 p-3">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-sm font-medium text-gray-700">
                    {ext.fulfiller_name || `#${ext.user_id}`} 申请延期至 {ext.new_deadline}
                  </p>
                  <span className={`shrink-0 rounded-full px-2 py-0.5 text-xs ${EXTENSION_STATUS_STYLE[ext.status] || "bg-gray-100 text-gray-600"}`}>
                    {formatExtensionStatus(ext.status)}
                  </span>
                </div>
                <p className="mt-1 text-xs text-gray-500">
                  原截止 {ext.old_deadline || "不限时"} → 新截止 {ext.new_deadline} · 提交于 {formatDate(ext.created_at)}
                </p>
                <p className="mt-1 text-sm text-gray-600">原因：{ext.reason}</p>
                {ext.reviewed_at && (
                  <p className="mt-1 text-xs text-gray-400">
                    {formatExtensionStatus(ext.status)} by {ext.reviewer_name || `#${ext.reviewer_id}`} · {formatDate(ext.reviewed_at)}
                  </p>
                )}
                {isOwner && ext.status === "pending" && (
                  <div className="mt-2 flex gap-2">
                    <button className="btn-primary !px-3 !py-1 text-sm" disabled={actionLoading} onClick={() => reviewExtension(ext.id, "approve")}>批准 ✓</button>
                    <button className="btn-secondary !px-3 !py-1 text-sm" disabled={actionLoading} onClick={() => reviewExtension(ext.id, "reject")}>驳回 ✗</button>
                  </div>
                )}
              </div>
            ))}
          </div>

          {canSubmitExtension && (
            <div className="space-y-2 border-t border-purple-50 pt-3">
              <p className="text-sm font-medium text-gray-700">申请延期（新日期须晚于当前截止，且不超过今天起 90 天）</p>
              <input type="date" className="input" value={extDate} min={minExtDate} max={maxExtDate} onChange={(e) => setExtDate(e.target.value)} />
              <textarea className="input min-h-[60px]" value={extReason} onChange={(e) => setExtReason(e.target.value)} placeholder="说明延期原因..." />
              <button className="btn-primary" disabled={actionLoading} onClick={submitExtension}>提交延期申请</button>
            </div>
          )}
        </div>
      )}

      <div className="card space-y-4">
        <div className="flex items-center gap-2">
          <span className="text-xl">💬</span>
          <h2 className="text-lg font-semibold text-gray-800">祝福留言板（{blessings.length}）</h2>
          {wish.status === "completed" && <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700">庆祝模式</span>}
        </div>

        <div className="space-y-3">
          {blessings.map((b) => (
            <div key={b.id} className="flex gap-3 rounded-xl bg-purple-50/60 p-3">
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-pink-300 to-purple-400 text-xs text-white">
                {b.sender_name ? b.sender_name.slice(0, 1) : "友"}
              </div>
              <div className="min-w-0">
                <p className="text-xs text-gray-500">{b.sender_name || "匿名"} · {formatDate(b.created_at)} {b.is_celebrating && "🎉"}</p>
                <p className="mt-0.5 text-sm text-gray-700">{b.content}</p>
                {b.gift_emoji && <span className="mt-1 inline-block text-2xl">{b.gift_emoji}</span>}
              </div>
            </div>
          ))}
          {blessings.length === 0 && <p className="py-4 text-center text-sm text-gray-400">还没有祝福，来抢沙发吧</p>}
        </div>

        <div className="space-y-2 border-t border-purple-50 pt-3">
          <GiftPicker value={gift} onChange={setGift} />
          <textarea className="input min-h-[60px]" value={blessText} onChange={(e) => setBlessText(e.target.value)} placeholder="送上你的祝福与鼓励..." />
          <button className="btn-primary" onClick={sendBlessing}>发送祝福 {gift}</button>
        </div>
      </div>
    </div>
  );
}
