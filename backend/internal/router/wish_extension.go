package router

import (
	"github.com/gin-gonic/gin"

	"github.com/wishwall/wishwall/internal/handler"
)

// RegisterWishExtensionRoutes 注册心愿延期申请路由。
func RegisterWishExtensionRoutes(rg *gin.RouterGroup, h *handler.WishExtensionHandler, auth gin.HandlerFunc) {
	rg.POST("/wishes/:id/extensions", auth, h.Submit)
	rg.GET("/wishes/:id/extensions", h.ListByWish)
	rg.POST("/extensions/:id/review", auth, h.Review)
}
