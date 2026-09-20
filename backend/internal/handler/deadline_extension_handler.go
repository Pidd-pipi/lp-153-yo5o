package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/wishwall/wishwall/internal/constants"
	"github.com/wishwall/wishwall/internal/dto"
	"github.com/wishwall/wishwall/internal/middleware"
	"github.com/wishwall/wishwall/internal/service"
)

// DeadlineExtensionHandler 心愿延期申请处理器。
type DeadlineExtensionHandler struct {
	extension service.DeadlineExtensionService
}

// NewDeadlineExtensionHandler 构造延期申请处理器。
func NewDeadlineExtensionHandler(extension service.DeadlineExtensionService) *DeadlineExtensionHandler {
	return &DeadlineExtensionHandler{extension: extension}
}

// Submit POST /api/v1/wishes/:id/extensions
func (h *DeadlineExtensionHandler) Submit(c *gin.Context) {
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
	ext, err := h.extension.Submit(c.Request.Context(), userID, wishID, req, c.ClientIP(), middleware.GetRequestID(c))
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "延期申请已提交，等待心愿发布者审核", "data": dto.ToDeadlineExtensionResponse(ext, "", constants.ExtensionStatusText(ext.Status))})
}

// Approve POST /api/v1/extensions/:id/approve
func (h *DeadlineExtensionHandler) Approve(c *gin.Context) {
	h.review(c, true)
}

// Reject POST /api/v1/extensions/:id/reject
func (h *DeadlineExtensionHandler) Reject(c *gin.Context) {
	h.review(c, false)
}

func (h *DeadlineExtensionHandler) review(c *gin.Context, approve bool) {
	extensionID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "延期申请 id 参数非法")
		return
	}
	var req dto.ReviewExtensionRequest
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	userID := middleware.CurrentUserID(c)
	if approve {
		ext, aerr := h.extension.Approve(c.Request.Context(), userID, extensionID, req, c.ClientIP(), middleware.GetRequestID(c))
		if aerr != nil {
			handleError(c, aerr)
			return
		}
		c.JSON(200, gin.H{"code": 0, "message": constants.MsgExtensionApproved, "data": dto.ToDeadlineExtensionResponse(ext, "", constants.ExtensionStatusText(ext.Status))})
		return
	}
	ext, rerr := h.extension.Reject(c.Request.Context(), userID, extensionID, req, c.ClientIP(), middleware.GetRequestID(c))
	if rerr != nil {
		handleError(c, rerr)
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": constants.MsgExtensionRejected, "data": dto.ToDeadlineExtensionResponse(ext, "", constants.ExtensionStatusText(ext.Status))})
}

// ListByWish GET /api/v1/wishes/:id/extensions
func (h *DeadlineExtensionHandler) ListByWish(c *gin.Context) {
	wishID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		responseError(c, 400, constants.CodeBadRequest, "心愿 id 参数非法")
		return
	}
	var q dto.PageQuery
	if !bindQuery(c, &q) {
		return
	}
	userID := middleware.CurrentUserID(c)
	result, err := h.extension.ListByWish(userID, wishID, q)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": result})
}
