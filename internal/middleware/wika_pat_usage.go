package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	wikaauth "github.com/Tencent/WeKnora/internal/wika/auth"
)

// WikaPATUsageRecorderService 是 Wika PAT 调用统计写入接口。
type WikaPATUsageRecorderService interface {
	RecordUsage(ctx context.Context, record wikaauth.TokenUsageRecord) error
}

// WikaPATUsageRecorder 记录已通过 Wika PAT 鉴权的 daily route 调用。
func WikaPATUsageRecorder(recorder WikaPATUsageRecorderService) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := c.Get(types.WikaPATUsageContextKey.String())
		if !ok {
			c.Next()
			return
		}
		usage, ok := raw.(types.WikaPATUsageContext)
		if !ok {
			c.Next()
			return
		}

		startedAt := time.Now()
		c.Next()

		recordWikaPATUsage(c, recorder, usage, startedAt)
	}
}

func recordWikaPATUsage(c *gin.Context, recorder WikaPATUsageRecorderService, usage types.WikaPATUsageContext, startedAt time.Time) {
	if recorder == nil {
		return
	}
	statusCode := c.Writer.Status()
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	record := wikaauth.TokenUsageRecord{
		TenantID:    usage.TenantID,
		UserID:      usage.UserID,
		TokenID:     usage.TokenID,
		ToolName:    usage.ToolName,
		APIMethod:   usage.APIMethod,
		APIPath:     usage.APIPath,
		StatusCode:  statusCode,
		Success:     statusCode < http.StatusBadRequest,
		ErrorCode:   wikaUsageErrorCode(c, statusCode),
		LatencyMS:   int(time.Since(startedAt).Milliseconds()),
		KnowledgeID: c.GetString(types.WikaKnowledgeIDContextKey.String()),
		OccurredAt:  startedAt,
	}
	if err := recorder.RecordUsage(c.Request.Context(), record); err != nil {
		logger.Warnf(c.Request.Context(), "failed to record Wika PAT usage token_id=%d tool=%s: %v", usage.TokenID, usage.ToolName, err)
		return
	}
	c.Set(types.WikaPATUsageRecordedContextKey.String(), true)
}

func wikaUsageErrorCode(c *gin.Context, statusCode int) string {
	if statusCode < http.StatusBadRequest {
		return ""
	}
	if len(c.Errors) > 0 {
		return strconv.Itoa(statusCode)
	}
	return strconv.Itoa(statusCode)
}
