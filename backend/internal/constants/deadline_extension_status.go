package constants

// 延期申请状态机：pending -> approved / rejected。
const (
	ExtensionStatusPending  = "pending"
	ExtensionStatusApproved = "approved"
	ExtensionStatusRejected = "rejected"
)

// ValidExtensionStatuses 状态白名单。
var ValidExtensionStatuses = []string{
	ExtensionStatusPending,
	ExtensionStatusApproved,
	ExtensionStatusRejected,
}

// ExtensionMaxDays 新完成日期距提交日的最大天数。
const ExtensionMaxDays = 90

// ExtensionStatusText 延期申请状态文本（service 状态机、formatters、前端徽标共同引用）。
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
