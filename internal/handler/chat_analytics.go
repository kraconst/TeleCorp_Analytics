package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (h *Handler) GetChatAnalytics(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if h.messageUsecase == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "chat analytics requires message storage"})
		return
	}
	a, err := h.messageUsecase.Analytics(c.Request.Context(), companyID, id)
	if err != nil {
		handleUsecaseError(c, err, "chat not found")
		return
	}
	c.JSON(http.StatusOK, a)
}
