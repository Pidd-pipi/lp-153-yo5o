import { http } from "@/utils/request";

export interface WishExtension {
  id: number;
  wish_id: number;
  claim_id: number;
  user_id: number;
  fulfiller_name?: string;
  old_deadline?: string | null;
  new_deadline: string;
  reason: string;
  status: string;
  reviewer_id: number;
  reviewer_name?: string;
  reviewed_at?: string | null;
  created_at: string;
}

export const extensionApi = {
  submit: (wishId: number, payload: { new_deadline: string; reason: string }) =>
    http.post<WishExtension>(`/wishes/${wishId}/extensions`, payload),
  listByWish: (wishId: number) => http.get<WishExtension[]>(`/wishes/${wishId}/extensions`),
  review: (id: number, action: "approve" | "reject") =>
    http.post<WishExtension>(`/extensions/${id}/review`, { action }),
};
