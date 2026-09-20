package router

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/wishwall/wishwall/internal/handler"
)

// TestRegisterExtensionRoutes 新路由与既有 /wishes/:id/claim、/wishes/:id/like 同参数层级共存，注册期不得 panic。
func TestRegisterExtensionRoutes(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	noop := func(c *gin.Context) {}
	_ = noop
	// 注册期仅取方法值，不会解引用 handler，故可传零值处理器。
	RegisterWishClaimRoutes(api, (*handler.WishClaimHandler)(nil), func(c *gin.Context) {})
	RegisterDeadlineExtensionRoutes(api, (*handler.DeadlineExtensionHandler)(nil), func(c *gin.Context) {})

	routes := map[string]bool{}
	for _, ri := range r.Routes() {
		routes[ri.Method+" "+ri.Path] = true
	}
	for _, want := range []string{
		"POST /api/v1/wishes/:id/extensions",
		"GET /api/v1/wishes/:id/extensions",
		"POST /api/v1/extensions/:id/approve",
		"POST /api/v1/extensions/:id/reject",
		"POST /api/v1/wishes/:id/claim",
	} {
		if !routes[want] {
			t.Fatalf("route %s not registered", want)
		}
	}
}
