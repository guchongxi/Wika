package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/handler/dto"
	"github.com/Tencent/WeKnora/internal/types"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

type modelVisibilityRequest struct {
	UserSelectable bool `json:"user_selectable"`
}

func modelTypeFromQuery(c *gin.Context) types.ModelType {
	return types.ModelType(secutils.SanitizeForLog(c.Query("type")))
}

func modelUsageContextFromQuery(c *gin.Context) types.ModelUsageContext {
	usageContext := types.ModelUsageContext(secutils.SanitizeForLog(c.Query("usage_context")))
	if usageContext == "" {
		return types.ModelUsageContextPersonal
	}
	return usageContext
}

func writeModelScopeError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if err == service.ErrModelNotFound {
		c.Error(errors.NewNotFoundError("Model not found"))
		return
	}
	if appErr, ok := errors.IsAppError(err); ok {
		c.Error(appErr)
		return
	}
	c.Error(errors.NewInternalServerError(err.Error()))
}

func validateModelBaseURL(c *gin.Context, baseURL string) bool {
	if baseURL == "" {
		return true
	}
	if err := secutils.ValidateURLForSSRF(baseURL); err != nil {
		c.Error(errors.NewBadRequestError(secutils.FormatSSRFError("Base URL", baseURL, err)))
		return false
	}
	return true
}

func (h *ModelHandler) ListSelectableModels(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.GetUint64(types.TenantIDContextKey.String())
	if tenantID == 0 {
		c.Error(errors.NewBadRequestError("Tenant ID cannot be empty"))
		return
	}
	userID := c.GetString(types.UserIDContextKey.String())
	usageContext := modelUsageContextFromQuery(c)
	if usageContext != types.ModelUsageContextPersonal && usageContext != types.ModelUsageContextTeam {
		c.Error(errors.NewBadRequestError("unknown model usage context"))
		return
	}
	models, err := h.service.ListSelectableModels(
		ctx,
		userID,
		tenantID,
		usageContext,
		modelTypeFromQuery(c),
	)
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewModelResponses(models)})
}

func (h *ModelHandler) ListMyModels(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		c.Error(errors.NewBadRequestError("User ID cannot be empty"))
		return
	}
	models, err := h.service.ListMyModels(ctx, userID, modelTypeFromQuery(c))
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewModelResponses(models)})
}

func (h *ModelHandler) GetMyModel(c *gin.Context) {
	model, err := h.service.GetModelByID(c.Request.Context(), secutils.SanitizeForLog(c.Param("id")))
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	if model.EffectiveScope() != types.ModelScopeUser || model.OwnerUserID != c.GetString(types.UserIDContextKey.String()) {
		c.Error(errors.NewNotFoundError("Model not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewModelResponse(model)})
}

func (h *ModelHandler) CreateMyModel(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		c.Error(errors.NewBadRequestError("User ID cannot be empty"))
		return
	}
	var req CreateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if !validateModelBaseURL(c, req.Parameters.BaseURL) {
		return
	}
	model := &types.Model{
		Name:        secutils.SanitizeForLog(req.Name),
		DisplayName: secutils.SanitizeForLog(req.DisplayName),
		Type:        types.ModelType(secutils.SanitizeForLog(string(req.Type))),
		Source:      req.Source,
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  req.Parameters,
	}
	if err := h.service.CreateUserModel(ctx, userID, model); err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": dto.NewModelResponse(model)})
}

func (h *ModelHandler) UpdateMyModel(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		c.Error(errors.NewBadRequestError("User ID cannot be empty"))
		return
	}
	id := secutils.SanitizeForLog(c.Param("id"))
	var req UpdateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if !validateModelBaseURL(c, req.Parameters.BaseURL) {
		return
	}
	model := &types.Model{
		ID:          id,
		Name:        secutils.SanitizeForLog(req.Name),
		Type:        req.Type,
		Source:      req.Source,
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  req.Parameters,
	}
	if req.DisplayName != nil {
		model.DisplayName = secutils.SanitizeForLog(*req.DisplayName)
	}
	if err := h.service.UpdateUserModel(ctx, userID, model); err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewModelResponse(model)})
}

func (h *ModelHandler) DeleteMyModel(c *gin.Context) {
	ctx := c.Request.Context()
	userID := c.GetString(types.UserIDContextKey.String())
	if userID == "" {
		c.Error(errors.NewBadRequestError("User ID cannot be empty"))
		return
	}
	if err := h.service.DeleteUserModel(ctx, userID, secutils.SanitizeForLog(c.Param("id"))); err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Model deleted"})
}

func (h *ModelHandler) PutMyModelCredentials(c *gin.Context) {
	model, err := h.service.GetModelByID(c.Request.Context(), secutils.SanitizeForLog(c.Param("id")))
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	if model.EffectiveScope() != types.ModelScopeUser || model.OwnerUserID != c.GetString(types.UserIDContextKey.String()) {
		c.Error(errors.NewNotFoundError("Model not found"))
		return
	}
	var req modelCredentialsPutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	updated, err := h.service.UpdateModelCredentials(c.Request.Context(), model.ID, req.APIKey, req.AppSecret)
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.CredentialsResponse{
		Fields: map[string]dto.CredentialFieldMetadata{
			"api_key":    {Configured: updated.Parameters.APIKey != ""},
			"app_secret": {Configured: updated.Parameters.AppSecret != ""},
		},
	}})
}

func (h *ModelHandler) DeleteMyModelCredential(c *gin.Context) {
	model, err := h.service.GetModelByID(c.Request.Context(), secutils.SanitizeForLog(c.Param("id")))
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	if model.EffectiveScope() != types.ModelScopeUser || model.OwnerUserID != c.GetString(types.UserIDContextKey.String()) {
		c.Error(errors.NewNotFoundError("Model not found"))
		return
	}
	field := c.Param("field")
	if field != "api_key" && field != "app_secret" {
		c.Error(errors.NewBadRequestError("unknown credential field: " + secutils.SanitizeForLog(field)))
		return
	}
	if err := h.service.ClearModelCredential(c.Request.Context(), model.ID, field); err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ModelHandler) ListSystemModels(c *gin.Context) {
	models, err := h.service.ListSystemModels(c.Request.Context(), modelTypeFromQuery(c))
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewSystemModelResponses(models)})
}

func (h *ModelHandler) GetSystemModel(c *gin.Context) {
	model, err := h.service.GetModelByID(c.Request.Context(), secutils.SanitizeForLog(c.Param("id")))
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	if !model.IsSystemModel() {
		c.Error(errors.NewNotFoundError("Model not found"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewSystemModelResponse(model)})
}

func (h *ModelHandler) CreateSystemModel(c *gin.Context) {
	var req CreateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if !validateModelBaseURL(c, req.Parameters.BaseURL) {
		return
	}
	model := &types.Model{
		Name:        secutils.SanitizeForLog(req.Name),
		DisplayName: secutils.SanitizeForLog(req.DisplayName),
		Type:        types.ModelType(secutils.SanitizeForLog(string(req.Type))),
		Source:      req.Source,
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  req.Parameters,
	}
	if err := h.service.CreateSystemModel(c.Request.Context(), model); err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": dto.NewSystemModelResponse(model)})
}

func (h *ModelHandler) UpdateSystemModel(c *gin.Context) {
	var req UpdateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	if !validateModelBaseURL(c, req.Parameters.BaseURL) {
		return
	}
	model := &types.Model{
		ID:          secutils.SanitizeForLog(c.Param("id")),
		Name:        secutils.SanitizeForLog(req.Name),
		Type:        req.Type,
		Source:      req.Source,
		Description: secutils.SanitizeForLog(req.Description),
		Parameters:  req.Parameters,
	}
	if req.DisplayName != nil {
		model.DisplayName = secutils.SanitizeForLog(*req.DisplayName)
	}
	if err := h.service.UpdateSystemModel(c.Request.Context(), model); err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewSystemModelResponse(model)})
}

func (h *ModelHandler) DeleteSystemModel(c *gin.Context) {
	if err := h.service.DeleteSystemModel(c.Request.Context(), secutils.SanitizeForLog(c.Param("id"))); err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Model deleted"})
}

func (h *ModelHandler) SetSystemModelVisibility(c *gin.Context) {
	var req modelVisibilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	model, err := h.service.SetSystemModelSelectable(
		c.Request.Context(),
		secutils.SanitizeForLog(c.Param("id")),
		req.UserSelectable,
	)
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewSystemModelResponse(model)})
}

func (h *ModelHandler) SetSystemDefaultModel(c *gin.Context) {
	model, err := h.service.SetSystemDefaultModel(
		c.Request.Context(),
		secutils.SanitizeForLog(c.Param("id")),
	)
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewSystemModelResponse(model)})
}

func (h *ModelHandler) UnsetSystemDefaultModel(c *gin.Context) {
	model, err := h.service.UnsetSystemDefaultModel(
		c.Request.Context(),
		secutils.SanitizeForLog(c.Param("id")),
	)
	if err != nil {
		writeModelScopeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.NewSystemModelResponse(model)})
}
