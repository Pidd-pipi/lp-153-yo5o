import {
  WISH_STATUS_STYLE,
  WISH_STATUS_TEXT,
  CAPSULE_STATUS_TEXT,
  EXTENSION_STATUS_TEXT,
  EXTENSION_STATUS_STYLE,
} from "@/constants";

interface StatusBadgeProps {
  status: string;
  kind?: "wish" | "capsule" | "extension";
}

// 状态徽标：心愿状态 / 胶囊状态 / 延期申请状态跨页复用。
export default function StatusBadge({ status, kind = "wish" }: StatusBadgeProps) {
  let text: string;
  let style: string;
  if (kind === "capsule") {
    text = CAPSULE_STATUS_TEXT[status] || status;
    style = status === "unlocked" ? "bg-emerald-100 text-emerald-700" : "bg-gray-100 text-gray-600";
  } else if (kind === "extension") {
    text = EXTENSION_STATUS_TEXT[status] || status;
    style = EXTENSION_STATUS_STYLE[status] || "bg-gray-100 text-gray-600";
  } else {
    text = WISH_STATUS_TEXT[status] || status;
    style = WISH_STATUS_STYLE[status] || "bg-gray-100 text-gray-600";
  }
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${style}`}>
      {text}
    </span>
  );
}
