package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type AccountSyncService interface {
	SyncAccount(context.Context, int64, int64) (usecase.SyncResult, error)
}

// SetAccountSync installs the optional service before InitRoutes is served.
func (h *Handler) SetAccountSync(service AccountSyncService) {
	h.accountSync = service
}

func (h *Handler) SyncAccount(c *gin.Context) {
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if h.accountSync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Telegram sync is not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), usecase.SyncTimeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	// This synchronous endpoint can outlive the ordinary API write deadline.
	err = http.NewResponseController(c.Writer).SetWriteDeadline(deadline.Add(5 * time.Second))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not prepare sync response"})
		return
	}

	result, err := h.accountSync.SyncAccount(ctx, companyID, accountID)
	if err == nil {
		c.JSON(http.StatusOK, result)
		return
	}
	if errors.Is(err, usecase.ErrTelegramUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Telegram sync is not configured"})
		return
	}
	if errors.Is(err, usecase.ErrTelegramSessionInvalid) {
		c.JSON(http.StatusConflict, gin.H{"error": "reconnect the Telegram account", "result": result})
		return
	}
	if errors.Is(err, domain.ErrConflict) {
		if result.ChatsProcessed > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "account is inactive or already syncing", "result": result})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "account is inactive or already syncing"})
		return
	}
	if result.ChatsProcessed > 0 {
		_ = c.Error(err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Telegram sync ended after partial progress", "result": result})
		return
	}
	handleUsecaseError(c, err, "account not found")
}
