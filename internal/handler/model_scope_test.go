package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type modelScopeFakeService struct {
	interfaces.ModelService
	selectableUsage types.ModelUsageContext
	selectableType  types.ModelType
	myUserID        string
	systemVisible   bool
}

func (s *modelScopeFakeService) ListSelectableModels(
	_ context.Context,
	_ string,
	_ uint64,
	usageContext types.ModelUsageContext,
	modelType types.ModelType,
) ([]*types.Model, error) {
	s.selectableUsage = usageContext
	s.selectableType = modelType
	return []*types.Model{{
		ID: "system-visible", Name: "visible", Type: modelType, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeSystem, IsBuiltin: true, UserSelectable: true, Status: types.ModelStatusActive,
	}}, nil
}

func (s *modelScopeFakeService) ListMyModels(_ context.Context, userID string, _ types.ModelType) ([]*types.Model, error) {
	s.myUserID = userID
	return []*types.Model{{
		ID: "mine", Name: "mine", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeUser, OwnerUserID: userID, Status: types.ModelStatusActive,
	}}, nil
}

func (s *modelScopeFakeService) CreateUserModel(_ context.Context, userID string, model *types.Model) error {
	s.myUserID = userID
	model.ID = "created-private"
	model.Scope = types.ModelScopeUser
	model.OwnerUserID = userID
	model.Status = types.ModelStatusActive
	return nil
}

func (s *modelScopeFakeService) SetSystemModelSelectable(_ context.Context, id string, selectable bool) (*types.Model, error) {
	if id == "" {
		return nil, errors.NewBadRequestError("id required")
	}
	s.systemVisible = selectable
	return &types.Model{
		ID: id, Name: "system", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeSystem, IsBuiltin: true, UserSelectable: selectable, Status: types.ModelStatusActive,
	}, nil
}

func (s *modelScopeFakeService) SetSystemDefaultModel(_ context.Context, id string) (*types.Model, error) {
	return &types.Model{
		ID: id, Name: "system", Type: types.ModelTypeKnowledgeQA, Source: types.ModelSourceRemote,
		Scope: types.ModelScopeSystem, IsBuiltin: true, IsDefault: true, UserSelectable: true, Status: types.ModelStatusActive,
	}, nil
}

func newModelScopeRouter(svc *modelScopeFakeService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), uint64(7))
		c.Set(types.UserIDContextKey.String(), "u-a")
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(7))
		ctx = context.WithValue(ctx, types.UserIDContextKey, "u-a")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := NewModelHandler(svc)
	r.GET("/api/v1/models/selectable", h.ListSelectableModels)
	r.GET("/api/v1/me/models", h.ListMyModels)
	r.POST("/api/v1/me/models", h.CreateMyModel)
	r.PATCH("/api/v1/system/admin/models/:id/visibility", h.SetSystemModelVisibility)
	r.PUT("/api/v1/system/admin/models/:id/default", h.SetSystemDefaultModel)
	return r
}

func TestListSelectableModelsPassesUsageContextAndType(t *testing.T) {
	svc := &modelScopeFakeService{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/selectable?type=Embedding&usage_context=team", nil)

	newModelScopeRouter(svc).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, types.ModelUsageContextTeam, svc.selectableUsage)
	assert.Equal(t, types.ModelTypeEmbedding, svc.selectableType)
	assert.Contains(t, w.Body.String(), `"scope":"system"`)
}

func TestMyModelsUseCurrentUser(t *testing.T) {
	svc := &modelScopeFakeService{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/models", nil)

	newModelScopeRouter(svc).ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "u-a", svc.myUserID)
	assert.Contains(t, w.Body.String(), `"owner_user_id":"u-a"`)
}

func TestCreateMyModelUsesCurrentUser(t *testing.T) {
	svc := &modelScopeFakeService{}
	body := []byte(`{"name":"mine","type":"KnowledgeQA","source":"remote","parameters":{}}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	newModelScopeRouter(svc).ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "u-a", svc.myUserID)
	assert.Contains(t, w.Body.String(), `"id":"created-private"`)
}

func TestSystemModelVisibilityAndDefaultResponsesExposeSystemFields(t *testing.T) {
	svc := &modelScopeFakeService{}
	r := newModelScopeRouter(svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/system/admin/models/system-1/visibility", bytes.NewReader([]byte(`{"user_selectable":false}`)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, svc.systemVisible)
	assert.Contains(t, w.Body.String(), `"user_selectable":false`)

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/system/admin/models/system-1/default", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Contains(t, w.Body.String(), `"is_default":true`)
	assert.Contains(t, w.Body.String(), `"user_selectable":true`)
}
