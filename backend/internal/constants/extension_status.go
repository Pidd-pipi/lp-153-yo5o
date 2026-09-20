package constants

// 延期申请状态机：pending -> approved / rejected（驳回后可重新提交）。
const (
	ExtensionStatusPending  = "pending"
	ExtensionStatusApproved = "approved"
	ExtensionStatusRejected = "rejected"
)

// 延期申请审核动作。
const (
	ExtensionActionApprove = "approve"
	ExtensionActionReject  = "reject"
)

// MaxExtensionDays 延期上限：新截止日不得超过提交日起九十天。
const MaxExtensionDays = 90

// ValidExtensionStatuses 状态白名单。
var ValidExtensionStatuses = []string{
	ExtensionStatusPending,
	ExtensionStatusApproved,
	ExtensionStatusRejected,
}

// ExtensionStatusText 状态文本（formatters 亦引用）。
func ExtensionStatusText(status string) string {
	switch status {
	case ExtensionStatusPending:
		return "待审核"
	case ExtensionStatusApproved:
		return "已批准"
	case ExtensionStatusRejected:
		return "已驳回"
	default:
		return "未知"
	}
}
