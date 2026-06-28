package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeTokenStore struct {
	saved  *types.WikaUserToken
	byHash map[string]*types.WikaUserToken
	byID   map[uint64]*types.WikaUserToken
}

func (f *fakeTokenStore) SaveToken(ctx context.Context, token *types.WikaUserToken) error {
	f.saved = token
	if f.byHash == nil {
		f.byHash = map[string]*types.WikaUserToken{}
	}
	if f.byID == nil {
		f.byID = map[uint64]*types.WikaUserToken{}
	}
	if token.ID == 0 {
		token.ID = uint64(len(f.byID) + 1)
	}
	f.byHash[token.TokenHash] = token
	f.byID[token.ID] = token
	return nil
}

func (f *fakeTokenStore) FindTokenByHash(ctx context.Context, tokenHash string) (*types.WikaUserToken, error) {
	if token, ok := f.byHash[tokenHash]; ok {
		return token, nil
	}
	return nil, ErrTokenInvalid
}

func (f *fakeTokenStore) ListTokens(ctx context.Context, userID string, tenantID uint64) ([]*types.WikaUserToken, error) {
	var out []*types.WikaUserToken
	for _, token := range f.byID {
		if token.UserID == userID && token.TenantID == tenantID {
			out = append(out, token)
		}
	}
	return out, nil
}

func (f *fakeTokenStore) RevokeToken(ctx context.Context, userID string, tokenID uint64, revokedAt time.Time) error {
	token, ok := f.byID[tokenID]
	if !ok || token.UserID != userID {
		return ErrTokenInvalid
	}
	token.RevokedAt = &revokedAt
	return nil
}

func TestCreateTokenStoresOnlyHashAndReturnsPlaintextOnce(t *testing.T) {
	store := &fakeTokenStore{}
	service := NewTokenService(store, "pepper")
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }
	service.randomToken = func() (string, error) { return "wika_pat_plaintext", nil }

	created, err := service.CreateToken(context.Background(), CreateTokenInput{
		UserID:    "user-1",
		TenantID:  7,
		Name:      "Claude Code",
		Scopes:    []string{"knowledge:push", "knowledge:search"},
		ExpiresAt: time.Unix(200, 0).UTC(),
	})

	if err != nil {
		t.Fatalf("expected token creation, got error: %v", err)
	}
	if created.Plaintext != "wika_pat_plaintext" {
		t.Fatalf("expected plaintext token returned once, got %q", created.Plaintext)
	}
	if store.saved == nil {
		t.Fatal("expected token metadata to be stored")
	}
	if store.saved.TokenHash == "" || store.saved.TokenHash == created.Plaintext {
		t.Fatalf("expected stored hash, got %q", store.saved.TokenHash)
	}
	if store.saved.TokenPrefix != "wika_pat_plainte" {
		t.Fatalf("expected stable token prefix, got %q", store.saved.TokenPrefix)
	}
	var scopes []string
	if err := json.Unmarshal([]byte(store.saved.Scopes), &scopes); err != nil {
		t.Fatalf("expected scopes JSON: %v", err)
	}
	if len(scopes) != 2 || scopes[0] != "knowledge:push" || scopes[1] != "knowledge:search" {
		t.Fatalf("unexpected scopes: %#v", scopes)
	}
}

func TestVerifyTokenRequiresActiveUnexpiredScope(t *testing.T) {
	store := &fakeTokenStore{byHash: map[string]*types.WikaUserToken{}}
	service := NewTokenService(store, "pepper")
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }

	hash := service.hashToken("wika_pat_plaintext")
	store.byHash[hash] = &types.WikaUserToken{
		UserID:    "user-1",
		TenantID:  7,
		TokenHash: hash,
		Scopes:    types.JSON([]byte(`["knowledge:search"]`)),
		ExpiresAt: time.Unix(200, 0).UTC(),
	}

	token, err := service.VerifyToken(context.Background(), "wika_pat_plaintext", "knowledge:search")

	if err != nil {
		t.Fatalf("expected token to verify, got %v", err)
	}
	if token.UserID != "user-1" || token.TenantID != 7 {
		t.Fatalf("unexpected verified token: %+v", token)
	}

	_, err = service.VerifyToken(context.Background(), "wika_pat_plaintext", "knowledge:push")
	if !errors.Is(err, ErrTokenScopeDenied) {
		t.Fatalf("expected scope denial, got %v", err)
	}
}

func TestVerifyTokenRejectsExpiredOrRevoked(t *testing.T) {
	store := &fakeTokenStore{byHash: map[string]*types.WikaUserToken{}}
	service := NewTokenService(store, "pepper")
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }

	expiredHash := service.hashToken("expired")
	store.byHash[expiredHash] = &types.WikaUserToken{
		TokenHash: expiredHash,
		Scopes:    types.JSON([]byte(`["knowledge:search"]`)),
		ExpiresAt: time.Unix(99, 0).UTC(),
	}
	_, err := service.VerifyToken(context.Background(), "expired", "knowledge:search")
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected expired token rejection, got %v", err)
	}

	revokedAt := time.Unix(90, 0).UTC()
	revokedHash := service.hashToken("revoked")
	store.byHash[revokedHash] = &types.WikaUserToken{
		TokenHash: revokedHash,
		Scopes:    types.JSON([]byte(`["knowledge:search"]`)),
		ExpiresAt: time.Unix(200, 0).UTC(),
		RevokedAt: &revokedAt,
	}
	_, err = service.VerifyToken(context.Background(), "revoked", "knowledge:search")
	if !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("expected revoked token rejection, got %v", err)
	}
}

func TestListTokensDoesNotExposeHashOrPlaintext(t *testing.T) {
	store := &fakeTokenStore{}
	service := NewTokenService(store, "pepper")
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }
	service.randomToken = func() (string, error) { return "wika_pat_plaintext", nil }
	_, err := service.CreateToken(context.Background(), CreateTokenInput{
		UserID:    "user-1",
		TenantID:  7,
		Name:      "Claude Code",
		Scopes:    []string{"knowledge:search"},
		ExpiresAt: time.Unix(200, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("expected token creation, got %v", err)
	}

	tokens, err := service.ListTokens(context.Background(), "user-1", 7)

	if err != nil {
		t.Fatalf("expected token list, got %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("expected one listed token, got %d", len(tokens))
	}
	if tokens[0].TokenHash != "" {
		t.Fatalf("listed token must redact hash, got %q", tokens[0].TokenHash)
	}
	if tokens[0].TokenPrefix == "" {
		t.Fatal("listed token should include prefix for user display")
	}
	if store.byID[tokens[0].ID].TokenHash == "" {
		t.Fatal("listing tokens must not clear stored token hash")
	}

	_, err = service.VerifyToken(context.Background(), "wika_pat_plaintext", "knowledge:search")
	if err != nil {
		t.Fatalf("listing tokens must not corrupt stored hash, verify got %v", err)
	}
}

func TestRevokeTokenMarksTokenRevoked(t *testing.T) {
	store := &fakeTokenStore{}
	service := NewTokenService(store, "pepper")
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }
	service.randomToken = func() (string, error) { return "wika_pat_plaintext", nil }
	created, err := service.CreateToken(context.Background(), CreateTokenInput{
		UserID:    "user-1",
		TenantID:  7,
		Name:      "Claude Code",
		Scopes:    []string{"knowledge:search"},
		ExpiresAt: time.Unix(200, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("expected token creation, got %v", err)
	}

	err = service.RevokeToken(context.Background(), "user-1", created.Metadata.ID)

	if err != nil {
		t.Fatalf("expected revoke, got %v", err)
	}
	if store.byID[created.Metadata.ID].RevokedAt == nil {
		t.Fatal("expected token revoked_at to be set")
	}
}
