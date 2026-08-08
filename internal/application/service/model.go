package service

import (
	"context"
	"errors"
	"fmt"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/models/provider"
	"github.com/Tencent/WeKnora/internal/models/rerank"
	"github.com/Tencent/WeKnora/internal/models/utils/ollama"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

// ErrModelNotFound is returned when a model cannot be found in the repository
var ErrModelNotFound = errors.New("model not found")

// modelService implements the model service interface
type modelService struct {
	repo          interfaces.ModelRepository
	kbRepo        interfaces.KnowledgeBaseRepository
	agentRepo     interfaces.CustomAgentRepository
	ollamaService *ollama.OllamaService
	pooler        embedding.EmbedderPooler
	tenantService interfaces.TenantService
}

// NewModelService creates a new model service instance
func NewModelService(repo interfaces.ModelRepository,
	kbRepo interfaces.KnowledgeBaseRepository,
	agentRepo interfaces.CustomAgentRepository,
	ollamaService *ollama.OllamaService,
	pooler embedding.EmbedderPooler,
	tenantService interfaces.TenantService,
) interfaces.ModelService {
	return &modelService{
		repo:          repo,
		kbRepo:        kbRepo,
		agentRepo:     agentRepo,
		ollamaService: ollamaService,
		pooler:        pooler,
		tenantService: tenantService,
	}
}

// decryptAppSecret 解密 AppSecret（如果为空或 cryptoSvc 为空则原样返回）
func (s *modelService) decryptAppSecret(encrypted string) string {
	if encrypted == "" {
		return encrypted
	}
	if key := utils.GetAESKey(); key != nil {
		if encrypted, err := utils.DecryptAESGCM(encrypted, key); err == nil {
			return encrypted
		}
	}
	return encrypted
}

// resolveWeKnoraCloudCredentials 为 WeKnoraCloud 厂商模型补全 AppID/AppSecret。
// 当模型自身参数中未存储凭证时，自动从租户配置中获取（SaveCredentials 保存的凭证）。
func (s *modelService) resolveWeKnoraCloudCredentials(ctx context.Context, params *types.ModelParameters) (appID, appSecret string) {
	appID = params.AppID
	appSecret = s.decryptAppSecret(params.AppSecret)

	if provider.ProviderName(params.Provider) != provider.ProviderWeKnoraCloud {
		return
	}
	if appID != "" && appSecret != "" {
		return
	}

	if s.tenantService == nil {
		return
	}
	creds := s.tenantService.GetWeKnoraCloudCredentials(ctx)
	if creds == nil {
		return
	}
	if appID == "" {
		appID = creds.AppID
	}
	if appSecret == "" {
		appSecret = creds.AppSecret
	}
	return
}

func normalizeModelScopeForCreate(model *types.Model) {
	if model.Scope == "" {
		if model.IsBuiltin {
			model.Scope = types.ModelScopeSystem
		} else {
			model.Scope = types.ModelScopeTenant
		}
	}
	switch model.Scope {
	case types.ModelScopeSystem:
		model.IsBuiltin = true
		model.OwnerUserID = ""
	case types.ModelScopeUser:
		model.IsBuiltin = false
		model.IsDefault = false
		model.UserSelectable = false
	default:
		model.Scope = types.ModelScopeTenant
		model.IsBuiltin = false
		model.OwnerUserID = ""
		model.UserSelectable = false
	}
}

func (s *modelService) getModelForCaller(ctx context.Context, tenantID uint64, id string) (*types.Model, error) {
	if userID, ok := types.UserIDFromContext(ctx); ok {
		return s.repo.GetByIDForUser(ctx, tenantID, userID, id)
	}
	return s.repo.GetByID(ctx, tenantID, id)
}

func prepareScopedModelUpdate(model, existing *types.Model) {
	if model.Name == "" {
		model.Name = existing.Name
	}
	if model.Type == "" {
		model.Type = existing.Type
	}
	if model.Source == "" {
		model.Source = existing.Source
	}
	model.CreatedAt = existing.CreatedAt
	model.UpdatedAt = existing.UpdatedAt
	model.DeletedAt = existing.DeletedAt
	if model.Parameters.APIKey == "" {
		model.Parameters.APIKey = existing.Parameters.APIKey
	}
	if model.Parameters.AppSecret == "" {
		model.Parameters.AppSecret = existing.Parameters.AppSecret
	}
	if model.Parameters.ParameterSize == "" {
		model.Parameters.ParameterSize = existing.Parameters.ParameterSize
	}
	if model.Parameters.ExtraConfig == nil {
		model.Parameters.ExtraConfig = existing.Parameters.ExtraConfig
	}
	if model.Parameters.CustomHeaders == nil {
		model.Parameters.CustomHeaders = existing.Parameters.CustomHeaders
	}
}

func (s *modelService) ensureModelNotReferenced(ctx context.Context, tenantID uint64, id string) error {
	var kbCount int64
	if s.kbRepo != nil {
		count, err := s.kbRepo.CountByModelID(ctx, tenantID, id)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": id})
			return err
		}
		kbCount = count
	}
	var agentCount int64
	if s.agentRepo != nil {
		count, err := s.agentRepo.CountByModelID(ctx, tenantID, id)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{"model_id": id})
			return err
		}
		agentCount = count
	}
	if kbCount > 0 || agentCount > 0 {
		logger.Warnf(ctx, "Model %s is in use: kb=%d agent=%d", id, kbCount, agentCount)
		return apperrors.NewBadRequestError(formatModelInUseMessage(kbCount, agentCount))
	}
	return nil
}

// CreateModel creates a new model in the repository
// For local models, it initiates an asynchronous download process
// Remote models are immediately set to active status
func (s *modelService) CreateModel(ctx context.Context, model *types.Model) error {
	logger.Infof(ctx, "Creating model: %s, type: %s, source: %s", model.Name, model.Type, model.Source)
	normalizeModelScopeForCreate(model)

	// Handle remote models (e.g., OpenAI, Azure)
	if model.Source == types.ModelSourceRemote {
		logger.Info(ctx, "Remote model detected, setting status to active")
		model.Status = types.ModelStatusActive

		logger.Info(ctx, "Saving remote model to repository")
		err := s.repo.Create(ctx, model)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"model_name": model.Name,
				"model_type": model.Type,
			})
			return err
		}

		logger.Infof(ctx, "Remote model created successfully: %s", model.ID)
		return nil
	}

	// Handle local models (e.g., Ollama)
	logger.Info(ctx, "Local model detected, setting status to downloading")
	model.Status = types.ModelStatusDownloading

	logger.Info(ctx, "Saving local model to repository")
	err := s.repo.Create(ctx, model)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_name": model.Name,
			"model_type": model.Type,
		})
		return err
	}

	// Start asynchronous model download
	logger.Infof(ctx, "Starting background download for model: %s", model.Name)
	newCtx := logger.CloneContext(ctx)
	go func() {
		logger.Info(newCtx, "Background download started")
		err := s.ollamaService.PullModel(newCtx, model.Name)
		if err != nil {
			logger.ErrorWithFields(newCtx, err, map[string]interface{}{
				"model_name": model.Name,
			})
			model.Status = types.ModelStatusDownloadFailed
		} else {
			logger.Infof(newCtx, "Model download completed successfully: %s", model.Name)
			model.Status = types.ModelStatusActive
		}
		logger.Infof(newCtx, "Updating model status to: %s", model.Status)
		s.repo.Update(newCtx, model)
	}()

	logger.Infof(ctx, "Model creation initiated successfully: %s", model.ID)
	return nil
}

// GetModelByID retrieves a model by its ID
// Returns an error if the model is not found or is in a non-active state
func (s *modelService) GetModelByID(ctx context.Context, id string) (*types.Model, error) {
	// Check if ID is empty
	if id == "" {
		logger.Error(ctx, "Model ID is empty")
		return nil, errors.New("model ID cannot be empty")
	}

	tenantID := types.MustTenantIDFromContext(ctx)

	// Fetch model from repository
	model, err := s.getModelForCaller(ctx, tenantID, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":  id,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	// Check if model exists
	if model == nil {
		logger.Error(ctx, "Model not found")
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Model found, name: %s, status: %s", model.Name, model.Status)

	// Check model status
	if model.Status == types.ModelStatusActive {
		return model, nil
	}

	if model.Status == types.ModelStatusDownloading {
		logger.Warn(ctx, "Model is currently downloading")
		return nil, errors.New("model is currently downloading")
	}

	if model.Status == types.ModelStatusDownloadFailed {
		logger.Error(ctx, "Model download failed")
		return nil, errors.New("model download failed")
	}

	logger.Error(ctx, "Model status is abnormal")
	return nil, errors.New("abnormal model status")
}

// ListModels returns all models belonging to the tenant
func (s *modelService) ListModels(ctx context.Context) ([]*types.Model, error) {
	logger.Info(ctx, "Start listing models")

	tenantID := types.MustTenantIDFromContext(ctx)
	logger.Infof(ctx, "Listing models for tenant ID: %d", tenantID)

	// List models from repository with no additional filters
	models, err := s.repo.List(ctx, tenantID, "", "")
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Retrieved %d models successfully", len(models))
	return models, nil
}

func (s *modelService) ListSystemModels(ctx context.Context, modelType types.ModelType) ([]*types.Model, error) {
	return s.repo.ListSystem(ctx, modelType)
}

func (s *modelService) CreateSystemModel(ctx context.Context, model *types.Model) error {
	if model.TenantID == 0 {
		model.TenantID = types.DefaultBuiltinModelTenantID
	}
	model.Scope = types.ModelScopeSystem
	model.IsBuiltin = true
	model.OwnerUserID = ""
	return s.CreateModel(ctx, model)
}

func (s *modelService) UpdateSystemModel(ctx context.Context, model *types.Model) error {
	existing, err := s.repo.GetSystemByID(ctx, model.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrModelNotFound
	}
	model.TenantID = existing.TenantID
	model.Scope = types.ModelScopeSystem
	model.IsBuiltin = true
	model.OwnerUserID = ""
	model.IsDefault = existing.IsDefault
	model.UserSelectable = existing.UserSelectable
	model.Status = existing.Status
	prepareScopedModelUpdate(model, existing)
	if err := s.repo.Update(ctx, model); err != nil {
		return err
	}
	return nil
}

func (s *modelService) DeleteSystemModel(ctx context.Context, id string) error {
	existing, err := s.repo.GetSystemByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrModelNotFound
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		tenantID = existing.TenantID
	}
	if err := s.ensureModelNotReferenced(ctx, tenantID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, existing.TenantID, id)
}

func (s *modelService) SetSystemModelSelectable(
	ctx context.Context, id string, selectable bool,
) (*types.Model, error) {
	existing, err := s.repo.GetSystemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrModelNotFound
	}
	if existing.IsDefault && !selectable {
		return nil, errors.New("default system model must remain selectable")
	}
	existing.Scope = types.ModelScopeSystem
	existing.IsBuiltin = true
	existing.UserSelectable = selectable
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *modelService) SetSystemDefaultModel(ctx context.Context, id string) (*types.Model, error) {
	existing, err := s.repo.GetSystemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrModelNotFound
	}
	if err := s.repo.ClearSystemDefaultByType(ctx, existing.Type, existing.ID); err != nil {
		return nil, err
	}
	existing.Scope = types.ModelScopeSystem
	existing.IsBuiltin = true
	existing.IsDefault = true
	existing.UserSelectable = true
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *modelService) UnsetSystemDefaultModel(ctx context.Context, id string) (*types.Model, error) {
	existing, err := s.repo.GetSystemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrModelNotFound
	}
	existing.IsDefault = false
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *modelService) ListMyModels(
	ctx context.Context, userID string, modelType types.ModelType,
) ([]*types.Model, error) {
	return s.repo.ListByOwner(ctx, userID, modelType)
}

func (s *modelService) CreateUserModel(ctx context.Context, userID string, model *types.Model) error {
	tenantID := types.MustTenantIDFromContext(ctx)
	model.TenantID = tenantID
	model.Scope = types.ModelScopeUser
	model.OwnerUserID = userID
	model.IsBuiltin = false
	model.IsDefault = false
	model.UserSelectable = false
	return s.CreateModel(ctx, model)
}

func (s *modelService) UpdateUserModel(ctx context.Context, userID string, model *types.Model) error {
	existing, err := s.repo.GetByIDForUser(ctx, types.MustTenantIDFromContext(ctx), userID, model.ID)
	if err != nil {
		return err
	}
	if existing == nil || existing.EffectiveScope() != types.ModelScopeUser || existing.OwnerUserID != userID {
		return ErrModelNotFound
	}
	model.TenantID = existing.TenantID
	model.Scope = types.ModelScopeUser
	model.OwnerUserID = userID
	model.IsBuiltin = false
	model.IsDefault = false
	model.UserSelectable = false
	model.Status = existing.Status
	prepareScopedModelUpdate(model, existing)
	return s.repo.Update(ctx, model)
}

func (s *modelService) DeleteUserModel(ctx context.Context, userID, id string) error {
	tenantID := types.MustTenantIDFromContext(ctx)
	existing, err := s.repo.GetByIDForUser(ctx, tenantID, userID, id)
	if err != nil {
		return err
	}
	if existing == nil || existing.EffectiveScope() != types.ModelScopeUser || existing.OwnerUserID != userID {
		return ErrModelNotFound
	}
	if err := s.ensureModelNotReferenced(ctx, tenantID, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, existing.TenantID, id)
}

func (s *modelService) ListSelectableModels(
	ctx context.Context,
	userID string,
	tenantID uint64,
	usageContext types.ModelUsageContext,
	modelType types.ModelType,
) ([]*types.Model, error) {
	if usageContext == "" {
		usageContext = types.ModelUsageContextPersonal
	}
	if usageContext != types.ModelUsageContextPersonal && usageContext != types.ModelUsageContextTeam {
		return nil, errors.New("unknown model usage context")
	}
	return s.repo.ListSelectable(ctx, tenantID, userID, usageContext, modelType)
}

// UpdateModel updates an existing model in the repository
func (s *modelService) UpdateModel(ctx context.Context, model *types.Model) error {
	logger.Info(ctx, "Start updating model")
	logger.Infof(ctx, "Updating model ID: %s, name: %s", model.ID, model.Name)

	// Check if the model is builtin - builtin models cannot be updated
	tenantID := types.MustTenantIDFromContext(ctx)
	existingModel, err := s.repo.GetByID(ctx, tenantID, model.ID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": model.ID,
		})
		return err
	}
	if existingModel != nil && existingModel.IsSystemModel() {
		logger.Warnf(ctx, "Attempted to update system model through tenant API: %s", model.ID)
		return errors.New("system models cannot be updated through tenant model API")
	}

	// Update model in repository
	err = s.repo.Update(ctx, model)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return err
	}

	logger.Infof(ctx, "Model updated successfully: %s", model.ID)
	return nil
}

// UpdateModelCredentials writes one or more credential fields on the model's
// Parameters jsonb. Models are not pooled per-instance the way MCP clients
// are (each call to GetEmbeddingModel/GetChatModel rebuilds the client from
// the current Parameters), so no explicit cache invalidation is required —
// the next call will pick up the new credential automatically.
func (s *modelService) UpdateModelCredentials(
	ctx context.Context, id string, apiKey, appSecret *string,
) (*types.Model, error) {
	tenantID := types.MustTenantIDFromContext(ctx)
	existing, err := s.getModelForCaller(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrModelNotFound
	}
	if existing.IsSystemModel() && !types.IsSystemAdminFromContext(ctx) {
		return nil, errors.New("system models cannot have credentials modified through tenant model API")
	}

	changed := false
	if apiKey != nil && *apiKey != "" && *apiKey != existing.Parameters.APIKey {
		existing.Parameters.APIKey = *apiKey
		changed = true
	}
	if appSecret != nil && *appSecret != "" && *appSecret != existing.Parameters.AppSecret {
		existing.Parameters.AppSecret = *appSecret
		changed = true
	}
	if !changed {
		return existing, nil
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	logger.Infof(ctx, "Model credentials updated: id=%s", id)
	return existing, nil
}

// ClearModelCredential removes a single credential field. Idempotent.
func (s *modelService) ClearModelCredential(ctx context.Context, id, field string) error {
	tenantID := types.MustTenantIDFromContext(ctx)
	existing, err := s.getModelForCaller(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrModelNotFound
	}
	if existing.IsSystemModel() && !types.IsSystemAdminFromContext(ctx) {
		return errors.New("system models cannot have credentials modified through tenant model API")
	}

	changed := false
	switch field {
	case "api_key":
		if existing.Parameters.APIKey != "" {
			existing.Parameters.APIKey = ""
			changed = true
		}
	case "app_secret":
		if existing.Parameters.AppSecret != "" {
			existing.Parameters.AppSecret = ""
			changed = true
		}
	default:
		return errors.New("unknown credential field: " + field)
	}
	if !changed {
		return nil
	}
	if err := s.repo.Update(ctx, existing); err != nil {
		return err
	}
	logger.Infof(ctx, "Model credential cleared by user: id=%s field=%s", id, field)
	return nil
}

// DeleteModel removes a model from the repository
func (s *modelService) DeleteModel(ctx context.Context, id string) error {
	logger.Info(ctx, "Start deleting model")
	logger.Infof(ctx, "Deleting model ID: %s", id)

	tenantID := types.MustTenantIDFromContext(ctx)
	logger.Infof(ctx, "Tenant ID: %d", tenantID)

	// Check if the model is builtin - builtin models cannot be deleted
	existingModel, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": id,
		})
		return err
	}
	if existingModel == nil {
		return ErrModelNotFound
	}
	if existingModel.IsSystemModel() {
		logger.Warnf(ctx, "Attempted to delete system model through tenant API: %s", id)
		return apperrors.NewBadRequestError("system models cannot be deleted through tenant model API")
	}

	if err := s.ensureModelNotReferenced(ctx, tenantID, id); err != nil {
		return err
	}

	// Delete model from repository
	err = s.repo.Delete(ctx, tenantID, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":  id,
			"tenant_id": tenantID,
		})
		return err
	}

	logger.Infof(ctx, "Model deleted successfully: %s", id)
	return nil
}

// GetEmbeddingModel retrieves and initializes an embedding model instance
// Takes a model ID and returns an Embedder interface implementation
func (s *modelService) GetEmbeddingModel(ctx context.Context, modelId string) (embedding.Embedder, error) {
	// Get the model details
	model, err := s.GetModelByID(ctx, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": modelId,
		})
		return nil, err
	}

	logger.Infof(ctx, "Getting embedding model: %s, source: %s", model.Name, model.Source)

	appID, appSecret := s.resolveWeKnoraCloudCredentials(ctx, &model.Parameters)

	embedder, err := embedding.NewEmbedder(embedding.ConfigFromModel(model, appID, appSecret), s.pooler, s.ollamaService)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	logger.Info(ctx, "Embedding model initialized successfully")
	return embedder, nil
}

// GetEmbeddingModelForTenant retrieves and initializes an embedding model for a specific tenant
// This is used for cross-tenant knowledge base sharing where the embedding model from
// the source tenant must be used to ensure vector compatibility
func (s *modelService) GetEmbeddingModelForTenant(ctx context.Context, modelId string, tenantID uint64) (embedding.Embedder, error) {
	// Check if model ID is empty
	if modelId == "" {
		logger.Error(ctx, "Model ID is empty")
		return nil, errors.New("model ID cannot be empty")
	}

	// Fetch model from repository using the specified tenant ID
	model, err := s.repo.GetByID(ctx, tenantID, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":  modelId,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	if model == nil {
		logger.Error(ctx, "Model not found for specified tenant")
		return nil, ErrModelNotFound
	}

	if model.Status != types.ModelStatusActive {
		logger.Errorf(ctx, "Model is not active, status: %s", model.Status)
		return nil, errors.New("model is not active")
	}

	logger.Infof(ctx, "Getting cross-tenant embedding model: %s, source: %s, tenant: %d", model.Name, model.Source, tenantID)

	appID, appSecret := s.resolveWeKnoraCloudCredentials(ctx, &model.Parameters)

	embedder, err := embedding.NewEmbedder(embedding.ConfigFromModel(model, appID, appSecret), s.pooler, s.ollamaService)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
			"tenant_id":  tenantID,
		})
		return nil, err
	}

	logger.Info(ctx, "Cross-tenant embedding model initialized successfully")
	return embedder, nil
}

// GetRerankModel retrieves and initializes a reranking model instance
// Takes a model ID and returns a Reranker interface implementation
func (s *modelService) GetRerankModel(ctx context.Context, modelId string) (rerank.Reranker, error) {
	// Get the model details
	model, err := s.GetModelByID(ctx, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id": modelId,
		})
		return nil, err
	}

	logger.Infof(ctx, "Getting rerank model: %s, source: %s", model.Name, model.Source)

	appID, appSecret := s.resolveWeKnoraCloudCredentials(ctx, &model.Parameters)

	reranker, err := rerank.NewReranker(rerank.ConfigFromModel(model, appID, appSecret))
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	logger.Info(ctx, "Rerank model initialized successfully")
	return reranker, nil
}

// GetChatModel retrieves and initializes a chat model instance
// Takes a model ID and returns a Chat interface implementation
func (s *modelService) GetChatModel(ctx context.Context, modelId string) (chat.Chat, error) {
	// Check if model ID is empty
	if modelId == "" {
		logger.Error(ctx, "Model ID is empty")
		return nil, errors.New("model ID cannot be empty")
	}

	tenantID := types.MustTenantIDFromContext(ctx)

	// Get the model directly from repository to avoid status checks
	model, err := s.getModelForCaller(ctx, tenantID, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":  modelId,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	if model == nil {
		logger.Error(ctx, "Chat model not found")
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Getting chat model: %s, source: %s", model.Name, model.Source)

	appID, appSecret := s.resolveWeKnoraCloudCredentials(ctx, &model.Parameters)

	chatModel, err := chat.NewChat(chat.ConfigFromModel(model, appID, appSecret), s.ollamaService)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	return chatModel, nil
}

// GetVLMModel retrieves and initializes a vision language model instance.
func (s *modelService) GetVLMModel(ctx context.Context, modelId string) (vlm.VLM, error) {
	if modelId == "" {
		return nil, errors.New("model ID cannot be empty")
	}

	tenantID := types.MustTenantIDFromContext(ctx)

	model, err := s.getModelForCaller(ctx, tenantID, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":  modelId,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	if model == nil {
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Getting VLM model: %s, source: %s", model.Name, model.Source)

	appID, appSecret := s.resolveWeKnoraCloudCredentials(ctx, &model.Parameters)

	vlmModel, err := vlm.NewVLM(vlm.ConfigFromModel(model, appID, appSecret), s.ollamaService)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	return vlmModel, nil
}

// Note: default model selection logic has been removed; models no longer
// maintain a per-type default flag at the service layer.

// GetASRModel retrieves and initializes an automatic speech recognition model instance.
func (s *modelService) GetASRModel(ctx context.Context, modelId string) (asr.ASR, error) {
	if modelId == "" {
		return nil, errors.New("model ID cannot be empty")
	}

	tenantID := types.MustTenantIDFromContext(ctx)

	model, err := s.getModelForCaller(ctx, tenantID, modelId)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":  modelId,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	if model == nil {
		return nil, ErrModelNotFound
	}

	logger.Infof(ctx, "Getting ASR model: %s, source: %s", model.Name, model.Source)

	sttModel, err := asr.NewASR(asr.ConfigFromModel(model))
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"model_id":   model.ID,
			"model_name": model.Name,
		})
		return nil, err
	}

	return sttModel, nil
}

func formatModelInUseMessage(kbCount, agentCount int64) string {
	switch {
	case kbCount > 0 && agentCount > 0:
		return fmt.Sprintf(
			"model is used by %d knowledge base(s) and %d agent(s); "+
				"reconfigure or remove those references before deleting",
			kbCount, agentCount,
		)
	case kbCount > 0:
		return fmt.Sprintf(
			"model is used by %d knowledge base(s); "+
				"reconfigure or remove those references before deleting",
			kbCount,
		)
	default:
		return fmt.Sprintf(
			"model is used by %d agent(s); "+
				"reconfigure or remove those references before deleting",
			agentCount,
		)
	}
}
