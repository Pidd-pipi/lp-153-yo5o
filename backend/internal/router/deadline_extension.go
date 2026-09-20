package router

import (
	"github.com/gin-gonic/gin"

	"github.com/wishwall/wishwall/internal/handler"
)

// RegisterDeadlineExtensionRoutes 注册心愿延期申请路由。
func RegisterDeadlineExtensionRoutes(rg *gin.RouterGroup, h *handler.DeadlineExtensionHandler, auth gin.HandlerFunc) {
	rg.POST("/wishes/:id/extensions", auth, h.Submit)
	rg.GET("/wishes/:id/extensions", auth, h.ListByWish)
	rg.POST("/extensions/:id/approve", auth, h.Approve)
	rg.POST("/extensions/:id/reject", auth, h.Reject)
}
