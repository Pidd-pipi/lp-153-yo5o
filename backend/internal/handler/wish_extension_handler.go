package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/middleware"
	"github.com/wishwall/wishwall/internal/service"
)

// WishExtensionHandler 心愿延期申请处理器。
type WishExtensionHandler struct {
	ext service.WishExtensionService
}

// NewWishExtensionHandler 构造延期申请处理器。
func NewWishExtensionHandler(ext service.WishExtensionService) *WishExtensionHandler {
	return &WishExtensionHandler{ext: ext}
}

// Submit POST /api/v1/wishes/:id/extensions
func (h *WishExtensionHandler) Submit(c *gin.Context) {
	wishID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "心愿 id 参数非法")
		return
	}
	var req dto.CreateExtensionRequest
	if !bindJSON(c, &req) {
		return
	}
	userID := middleware.CurrentUserID(c)
	ext, err := h.ext.Submit(c.Request.Context(), userID, wishID, req, c.ClientIP(), middleware.GetRequestID(c))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": constants.MsgExtensionSubmitted, "data": dto.ToWishExtensionResponse(ext, "", "")})
}

// Review POST /api/v1/extensions/:id/review
func (h *WishExtensionHandler) Review(c *gin.Context) {
	extensionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "延期申请 id 参数非法")
		return
	}
	var req dto.ReviewExtensionRequest
	if !bindJSON(c, &req) {
		return
	}
	userID := middleware.CurrentUserID(c)
	ext, err := h.ext.Review(c.Request.Context(), userID, extensionID, req, c.ClientIP(), middleware.GetRequestID(c))
	if err != nil {
		handleError(c, err)
		return
	}
	msg := constants.MsgExtensionRejected
	if req.Action == constants.ExtensionActionApprove {
		msg = constants.MsgExtensionApproved
	}
	c.JSON(200, gin.H{"code": 0, "message": msg, "data": dto.ToWishExtensionResponse(ext, "", "")})
}

// ListByWish GET /api/v1/wishes/:id/extensions
func (h *WishExtensionHandler) ListByWish(c *gin.Context) {
	wishID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "心愿 id 参数非法")
		return
	}
	items, err := h.ext.ListByWish(wishID)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": items})
}
