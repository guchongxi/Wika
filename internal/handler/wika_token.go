package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	wikaauth "github.com/Tencent/WeKnora/internal/wika/auth"
)

const maxWikaTokenTTL = 90 * 24 * time.Hour

var allowedWikaTokenScopes = map[string]struct{}{
	"knowledge:push":    {},
	"knowledge:search":  {},
	"knowledge:read":    {},
	"suggestion:create": {},
}

type wikaTokenService interface {
	CreateToken(ctx context.Context, input wikaauth.CreateTokenInput) (*wikaauth.CreatedToken, error)
	ListTokens(ctx context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error)
	RevokeToken(ctx context.Context, userID string, tokenID uint64) error
}

// WikaTokenHandler 暴露日常 MCP 工具使用的用户级 PAT 管理接口。
type WikaTokenHandler struct {
	service wikaTokenService
	now     func() time.Time
}

// NewWikaTokenHandler 创建 Wika token handler。
func NewWikaTokenHandler(service *wikaauth.TokenService) *WikaTokenHandler {
	return &WikaTokenHandler{service: service, now: time.Now}
}

type createWikaTokenRequest struct {
	Name      string    `json:"name"`
	TenantID  *uint64   `json:"tenant_id,omitempty"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

type wikaTokenResponse struct {
	ID          uint64     `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token,omitempty"`
	TokenPrefix string     `json:"token_prefix"`
	Scopes      []string   `json:"scopes"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
}

// CreateToken 创建用户级 PAT，并且只在本次响应返回明文 token。
func (h *WikaTokenHandler) CreateToken(c *gin.Context) {
	userID, tenantID, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	var req createWikaTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("invalid request body").WithDetails(err.Error()))
		return
	}
	name := strings.TrimSpace(req.Name)
	scopes, err := normalizeWikaTokenScopes(req.Scopes)
	if err != nil {
		c.Error(apperrors.NewBadRequestError(err.Error()))
		return
	}
	targetTenantID := tenantID
	if req.TenantID != nil {
		if *req.TenantID == 0 || *req.TenantID != tenantID {
			c.Error(apperrors.NewBadRequestError("tenant_id must match current tenant"))
			return
		}
		targetTenantID = *req.TenantID
	}
	now := h.currentTime()
	if name == "" {
		c.Error(apperrors.NewBadRequestError("name is required"))
		return
	}
	if req.ExpiresAt.IsZero() || !req.ExpiresAt.After(now) {
		c.Error(apperrors.NewBadRequestError("expires_at must be in the future"))
		return
	}
	if req.ExpiresAt.After(now.Add(maxWikaTokenTTL)) {
		c.Error(apperrors.NewBadRequestError("expires_at must be within 90 days"))
		return
	}

	created, err := h.service.CreateToken(c.Request.Context(), wikaauth.CreateTokenInput{
		UserID:      userID,
		TenantID:    targetTenantID,
		Name:        name,
		Scopes:      scopes,
		ExpiresAt:   req.ExpiresAt,
		CreatedByIP: c.ClientIP(),
	})
	if err != nil {
		logger.Error(c.Request.Context(), "failed to create Wika token", err)
		c.Error(apperrors.NewInternalServerError("failed to create token"))
		return
	}
	if created == nil || created.Metadata == nil {
		c.Error(apperrors.NewInternalServerError("failed to create token"))
		return
	}
	resp := buildWikaTokenResponse(created.Metadata)
	resp.Token = created.Plaintext
	c.JSON(http.StatusCreated, resp)
}

// ListTokens 列出当前用户的 PAT 元数据，不返回明文或 hash。
func (h *WikaTokenHandler) ListTokens(c *gin.Context) {
	userID, tenantID, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	tokens, err := h.service.ListTokens(c.Request.Context(), userID, tenantID)
	if err != nil {
		logger.Error(c.Request.Context(), "failed to list Wika tokens", err)
		c.Error(apperrors.NewInternalServerError("failed to list tokens"))
		return
	}
	resp := make([]wikaTokenResponse, 0, len(tokens))
	for _, token := range tokens {
		if token == nil {
			continue
		}
		resp = append(resp, buildWikaTokenResponse(token))
	}
	c.JSON(http.StatusOK, gin.H{"tokens": resp})
}

// RevokeToken 撤销当前用户自己的 PAT。
func (h *WikaTokenHandler) RevokeToken(c *gin.Context) {
	userID, _, ok := wikaTokenContext(c)
	if !ok {
		return
	}
	tokenID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || tokenID == 0 {
		c.Error(apperrors.NewBadRequestError("invalid token id"))
		return
	}
	if err := h.service.RevokeToken(c.Request.Context(), userID, tokenID); err != nil {
		if stderrors.Is(err, wikaauth.ErrTokenInvalid) {
			c.Error(apperrors.NewNotFoundError("token not found"))
			return
		}
		logger.Error(c.Request.Context(), "failed to revoke Wika token", err)
		c.Error(apperrors.NewInternalServerError("failed to revoke token"))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *WikaTokenHandler) currentTime() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now()
}

func wikaTokenContext(c *gin.Context) (string, uint64, bool) {
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		c.Error(apperrors.NewUnauthorizedError("user ID not found"))
		return "", 0, false
	}
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(apperrors.NewUnauthorizedError("tenant ID not found"))
		return "", 0, false
	}
	return userID, tenantID, true
}

func normalizeWikaTokenScopes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, stderrors.New("scopes are required")
	}
	scopes := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, scope := range raw {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil, stderrors.New("scope must not be empty")
		}
		if _, ok := allowedWikaTokenScopes[scope]; !ok {
			return nil, stderrors.New("unsupported scope: " + scope)
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

func buildWikaTokenResponse(token *types.WikaUserToken) wikaTokenResponse {
	return wikaTokenResponse{
		ID:          token.ID,
		Name:        token.Name,
		TokenPrefix: token.TokenPrefix,
		Scopes:      wikaTokenScopesFromJSON(token.Scopes),
		ExpiresAt:   token.ExpiresAt,
		RevokedAt:   token.RevokedAt,
		CreatedAt:   token.CreatedAt,
		LastUsedAt:  token.LastUsedAt,
	}
}

func wikaTokenScopesFromJSON(raw types.JSON) []string {
	var scopes []string
	if err := json.Unmarshal([]byte(raw), &scopes); err != nil {
		return []string{}
	}
	return scopes
}
