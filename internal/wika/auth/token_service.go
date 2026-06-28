package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	// ErrTokenInvalid 表示 token 不存在或格式无效。
	ErrTokenInvalid = errors.New("token invalid")
	// ErrTokenExpired 表示 token 已过期。
	ErrTokenExpired = errors.New("token expired")
	// ErrTokenRevoked 表示 token 已撤销。
	ErrTokenRevoked = errors.New("token revoked")
	// ErrTokenScopeDenied 表示 token 缺少所需 scope。
	ErrTokenScopeDenied = errors.New("token scope denied")
)

// TokenStore 隔离用户级 PAT 的持久化操作。
type TokenStore interface {
	SaveToken(ctx context.Context, token *types.WikaUserToken) error
	FindTokenByHash(ctx context.Context, tokenHash string) (*types.WikaUserToken, error)
	ListTokens(ctx context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error)
	RevokeToken(ctx context.Context, userID string, tokenID uint64, revokedAt time.Time) error
}

// CreateTokenInput 是创建用户级 MCP token 的输入。
type CreateTokenInput struct {
	UserID      string
	TenantID    uint64
	Name        string
	Scopes      []string
	ExpiresAt   time.Time
	CreatedByIP string
}

// CreatedToken 是创建成功后唯一一次携带明文 token 的响应。
type CreatedToken struct {
	Plaintext string
	Metadata  *types.WikaUserToken
}

// TokenService 提供用户级 MCP token 创建与校验。
type TokenService struct {
	store       TokenStore
	pepper      string
	now         func() time.Time
	randomToken func() (string, error)
}

// NewTokenService 创建 token 服务。
func NewTokenService(store TokenStore, pepper string) *TokenService {
	return &TokenService{
		store:       store,
		pepper:      pepper,
		now:         time.Now,
		randomToken: generatePlaintextToken,
	}
}

// CreateToken 生成明文 token，并只持久化 prefix、hash 和元数据。
func (s *TokenService) CreateToken(ctx context.Context, input CreateTokenInput) (*CreatedToken, error) {
	plaintext, err := s.randomToken()
	if err != nil {
		return nil, err
	}
	scopes, err := json.Marshal(input.Scopes)
	if err != nil {
		return nil, err
	}

	token := &types.WikaUserToken{
		UserID:      input.UserID,
		TenantID:    input.TenantID,
		Name:        input.Name,
		TokenPrefix: tokenPrefix(plaintext),
		TokenHash:   s.hashToken(plaintext),
		HashAlg:     "sha256_pepper",
		Scopes:      types.JSON(scopes),
		ExpiresAt:   input.ExpiresAt,
		CreatedAt:   s.now(),
		CreatedByIP: input.CreatedByIP,
	}
	if err := s.store.SaveToken(ctx, token); err != nil {
		return nil, err
	}
	return &CreatedToken{Plaintext: plaintext, Metadata: token}, nil
}

// VerifyToken 校验 token 是否有效并包含 requiredScope。
func (s *TokenService) VerifyToken(ctx context.Context, plaintext, requiredScope string) (*types.WikaUserToken, error) {
	if plaintext == "" {
		return nil, ErrTokenInvalid
	}
	token, err := s.store.FindTokenByHash(ctx, s.hashToken(plaintext))
	if err != nil {
		return nil, err
	}
	if token.RevokedAt != nil {
		return nil, ErrTokenRevoked
	}
	if !token.ExpiresAt.IsZero() && !s.now().Before(token.ExpiresAt) {
		return nil, ErrTokenExpired
	}
	if !tokenHasScope(token, requiredScope) {
		return nil, ErrTokenScopeDenied
	}
	return token, nil
}

// ListTokens 返回用户 token 元数据，主动脱敏 hash。
func (s *TokenService) ListTokens(ctx context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error) {
	tokens, err := s.store.ListTokens(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]*types.WikaUserToken, 0, len(tokens))
	for _, token := range tokens {
		if token != nil {
			cp := *token
			cp.TokenHash = ""
			out = append(out, &cp)
		}
	}
	return out, nil
}

// RevokeToken 撤销用户自己的 token。
func (s *TokenService) RevokeToken(ctx context.Context, userID string, tokenID uint64) error {
	return s.store.RevokeToken(ctx, userID, tokenID, s.now())
}

func (s *TokenService) hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext + "\x00" + s.pepper))
	return hex.EncodeToString(sum[:])
}

func tokenPrefix(plaintext string) string {
	const maxPrefix = 16
	if len(plaintext) <= maxPrefix {
		return plaintext
	}
	return plaintext[:maxPrefix]
}

func tokenHasScope(token *types.WikaUserToken, requiredScope string) bool {
	if requiredScope == "" {
		return true
	}
	var scopes []string
	if err := json.Unmarshal([]byte(token.Scopes), &scopes); err != nil {
		return false
	}
	for _, scope := range scopes {
		if scope == requiredScope {
			return true
		}
	}
	return false
}

func generatePlaintextToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "wika_pat_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
