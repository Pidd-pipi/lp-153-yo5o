import { http } from "@/utils/request";
import type { PageResult } from "./wish";

// 心愿延期申请：圆梦人提交，发布者批准/驳回（与后端 deadline_extension 实体对应）。
export interface DeadlineExtension {
  id: number;
  wish_id: number;
  claim_id: number;
  applicant_id: number;
  applicant_name?: string;
  reviewer_id: number;
  current_deadline?: string | null;
  new_deadline: string;
  reason: string;
  status: "pending" | "approved" | "rejected" | string;
  status_text?: string;
  review_note?: string;
  reviewed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateExtensionPayload {
  new_deadline: string; // yyyy-MM-dd
  reason: string;
}

export const extensionApi = {
  submit: (wishId: number, payload: CreateExtensionPayload) =>
    http.post<DeadlineExtension>(`/wishes/${wishId}/extensions`, payload),
  listByWish: (wishId: number, params?: Record<string, string | number | undefined>) =>
    http.get<PageResult<DeadlineExtension>>(`/wishes/${wishId}/extensions`, params),
  approve: (extensionId: number, note?: string) =>
    http.post<DeadlineExtension>(`/extensions/${extensionId}/approve`, note !== undefined ? { note } : undefined),
  reject: (extensionId: number, note?: string) =>
    http.post<DeadlineExtension>(`/extensions/${extensionId}/reject`, note !== undefined ? { note } : undefined),
};
