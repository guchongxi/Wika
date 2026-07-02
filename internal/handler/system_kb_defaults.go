package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	appservice "github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/gin-gonic/gin"
)

type kbDefaultsChunking struct {
	ChunkSize         int      `json:"chunk_size"`
	ChunkOverlap      int      `json:"chunk_overlap"`
	Separators        []string `json:"separators"`
	EnableParentChild bool     `json:"enable_parent_child"`
	ParentChunkSize   int      `json:"parent_chunk_size"`
	ChildChunkSize    int      `json:"child_chunk_size"`
}

type kbDefaultsIndexing struct {
	VectorEnabled  bool `json:"vector_enabled"`
	KeywordEnabled bool `json:"keyword_enabled"`
	WikiEnabled    bool `json:"wiki_enabled"`
	GraphEnabled   bool `json:"graph_enabled"`
}

type kbDefaultsMultimodal struct {
	VLMEnabled bool   `json:"vlm_enabled"`
	VLMModelID string `json:"vlm_model_id"`
	ASREnabled bool   `json:"asr_enabled"`
	ASRModelID string `json:"asr_model_id"`
}

type kbDefaultsQuestionGeneration struct {
	Enabled       bool `json:"enabled"`
	QuestionCount int  `json:"question_count"`
}

type knowledgeBaseDefaultsResponse struct {
	LLMModelID         string                       `json:"llm_model_id"`
	EmbeddingModelID   string                       `json:"embedding_model_id"`
	StorageProvider    string                       `json:"storage_provider"`
	Chunking           kbDefaultsChunking           `json:"chunking"`
	Indexing           kbDefaultsIndexing           `json:"indexing"`
	Multimodal         kbDefaultsMultimodal         `json:"multimodal"`
	QuestionGeneration kbDefaultsQuestionGeneration `json:"question_generation"`
}

type kbDefaultsChunkingRequest struct {
	ChunkSize         *int      `json:"chunk_size"`
	ChunkOverlap      *int      `json:"chunk_overlap"`
	Separators        *[]string `json:"separators"`
	EnableParentChild *bool     `json:"enable_parent_child"`
	ParentChunkSize   *int      `json:"parent_chunk_size"`
	ChildChunkSize    *int      `json:"child_chunk_size"`
}

type kbDefaultsIndexingRequest struct {
	VectorEnabled  *bool `json:"vector_enabled"`
	KeywordEnabled *bool `json:"keyword_enabled"`
	WikiEnabled    *bool `json:"wiki_enabled"`
	GraphEnabled   *bool `json:"graph_enabled"`
}

type kbDefaultsMultimodalRequest struct {
	VLMEnabled *bool   `json:"vlm_enabled"`
	VLMModelID *string `json:"vlm_model_id"`
	ASREnabled *bool   `json:"asr_enabled"`
	ASRModelID *string `json:"asr_model_id"`
}

type kbDefaultsQuestionGenerationRequest struct {
	Enabled       *bool `json:"enabled"`
	QuestionCount *int  `json:"question_count"`
}

type updateKnowledgeBaseDefaultsRequest struct {
	LLMModelID         *string                              `json:"llm_model_id"`
	EmbeddingModelID   *string                              `json:"embedding_model_id"`
	StorageProvider    *string                              `json:"storage_provider"`
	Chunking           *kbDefaultsChunkingRequest           `json:"chunking"`
	Indexing           *kbDefaultsIndexingRequest           `json:"indexing"`
	Multimodal         *kbDefaultsMultimodalRequest         `json:"multimodal"`
	QuestionGeneration *kbDefaultsQuestionGenerationRequest `json:"question_generation"`
}

// GetKnowledgeBaseDefaults returns the SystemAdmin-facing aggregate of KB defaults.
func (h *SystemHandler) GetKnowledgeBaseDefaults(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())
	if h.systemSettingSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "system setting service unavailable"})
		return
	}
	c.JSON(http.StatusOK, h.buildKnowledgeBaseDefaults(ctx))
}

// UpdateKnowledgeBaseDefaults persists the provided subset of KB defaults.
func (h *SystemHandler) UpdateKnowledgeBaseDefaults(c *gin.Context) {
	ctx := logger.CloneContext(c.Request.Context())
	if h.systemSettingSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "system setting service unavailable"})
		return
	}
	var req updateKnowledgeBaseDefaultsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	if err := h.persistKnowledgeBaseDefaults(ctx, req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.buildKnowledgeBaseDefaults(ctx))
}

func (h *SystemHandler) buildKnowledgeBaseDefaults(ctx context.Context) knowledgeBaseDefaultsResponse {
	settings := h.systemSettingSvc
	return knowledgeBaseDefaultsResponse{
		LLMModelID:       settings.GetString(ctx, appservice.KBDefaultLLMModelID, "WEKNORA_KB_DEFAULT_LLM_MODEL_ID", ""),
		EmbeddingModelID: settings.GetString(ctx, appservice.KBDefaultEmbeddingModelID, "WEKNORA_KB_DEFAULT_EMBEDDING_MODEL_ID", ""),
		StorageProvider:  settings.GetString(ctx, appservice.KBDefaultStorageProvider, "WEKNORA_KB_DEFAULT_STORAGE_PROVIDER", "local"),
		Chunking: kbDefaultsChunking{
			ChunkSize:         int(settings.GetInt(ctx, appservice.KBDefaultChunkSize, "WEKNORA_KB_DEFAULT_CHUNK_SIZE", 512)),
			ChunkOverlap:      int(settings.GetInt(ctx, appservice.KBDefaultChunkOverlap, "WEKNORA_KB_DEFAULT_CHUNK_OVERLAP", 80)),
			Separators:        settings.GetStringList(ctx, appservice.KBDefaultChunkSeparators, "WEKNORA_KB_DEFAULT_CHUNK_SEPARATORS", []string{"\n\n", "\n", "。", "！", "？", ";", "；"}),
			EnableParentChild: settings.GetBool(ctx, appservice.KBDefaultParentChildEnabled, "WEKNORA_KB_DEFAULT_PARENT_CHILD_ENABLED", false),
			ParentChunkSize:   int(settings.GetInt(ctx, appservice.KBDefaultParentChunkSize, "WEKNORA_KB_DEFAULT_PARENT_CHUNK_SIZE", 4096)),
			ChildChunkSize:    int(settings.GetInt(ctx, appservice.KBDefaultChildChunkSize, "WEKNORA_KB_DEFAULT_CHILD_CHUNK_SIZE", 384)),
		},
		Indexing: kbDefaultsIndexing{
			VectorEnabled:  settings.GetBool(ctx, appservice.KBDefaultIndexVectorEnabled, "WEKNORA_KB_DEFAULT_INDEX_VECTOR_ENABLED", true),
			KeywordEnabled: settings.GetBool(ctx, appservice.KBDefaultIndexKeywordEnabled, "WEKNORA_KB_DEFAULT_INDEX_KEYWORD_ENABLED", true),
			WikiEnabled:    settings.GetBool(ctx, appservice.KBDefaultIndexWikiEnabled, "WEKNORA_KB_DEFAULT_INDEX_WIKI_ENABLED", false),
			GraphEnabled:   settings.GetBool(ctx, appservice.KBDefaultIndexGraphEnabled, "WEKNORA_KB_DEFAULT_INDEX_GRAPH_ENABLED", false),
		},
		Multimodal: kbDefaultsMultimodal{
			VLMEnabled: settings.GetBool(ctx, appservice.KBDefaultVLMEnabled, "WEKNORA_KB_DEFAULT_VLM_ENABLED", false),
			VLMModelID: settings.GetString(ctx, appservice.KBDefaultVLMModelID, "WEKNORA_KB_DEFAULT_VLM_MODEL_ID", ""),
			ASREnabled: settings.GetBool(ctx, appservice.KBDefaultASREnabled, "WEKNORA_KB_DEFAULT_ASR_ENABLED", false),
			ASRModelID: settings.GetString(ctx, appservice.KBDefaultASRModelID, "WEKNORA_KB_DEFAULT_ASR_MODEL_ID", ""),
		},
		QuestionGeneration: kbDefaultsQuestionGeneration{
			Enabled:       settings.GetBool(ctx, appservice.KBDefaultQuestionGenerationEnabled, "WEKNORA_KB_DEFAULT_QUESTION_GENERATION_ENABLED", false),
			QuestionCount: int(settings.GetInt(ctx, appservice.KBDefaultQuestionGenerationCount, "WEKNORA_KB_DEFAULT_QUESTION_GENERATION_COUNT", 3)),
		},
	}
}

func (h *SystemHandler) persistKnowledgeBaseDefaults(ctx context.Context, req updateKnowledgeBaseDefaultsRequest) error {
	if req.LLMModelID != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultLLMModelID, strings.TrimSpace(*req.LLMModelID)); err != nil {
			return err
		}
	}
	if req.EmbeddingModelID != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultEmbeddingModelID, strings.TrimSpace(*req.EmbeddingModelID)); err != nil {
			return err
		}
	}
	if req.StorageProvider != nil {
		provider := strings.ToLower(strings.TrimSpace(*req.StorageProvider))
		if provider != "" && !isStorageProviderAllowed(provider) {
			return errors.New("storage provider is not allowed by STORAGE_ALLOW_LIST")
		}
		if err := h.updateKBDefault(ctx, appservice.KBDefaultStorageProvider, provider); err != nil {
			return err
		}
	}
	if req.Chunking != nil {
		if err := h.persistKBDefaultChunking(ctx, *req.Chunking); err != nil {
			return err
		}
	}
	if req.Indexing != nil {
		if err := h.persistKBDefaultIndexing(ctx, *req.Indexing); err != nil {
			return err
		}
	}
	if req.Multimodal != nil {
		if err := h.persistKBDefaultMultimodal(ctx, *req.Multimodal); err != nil {
			return err
		}
	}
	if req.QuestionGeneration != nil {
		if err := h.persistKBDefaultQuestionGeneration(ctx, *req.QuestionGeneration); err != nil {
			return err
		}
	}
	return nil
}

func (h *SystemHandler) persistKBDefaultChunking(ctx context.Context, req kbDefaultsChunkingRequest) error {
	if req.ChunkSize != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultChunkSize, int64(*req.ChunkSize)); err != nil {
			return err
		}
	}
	if req.ChunkOverlap != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultChunkOverlap, int64(*req.ChunkOverlap)); err != nil {
			return err
		}
	}
	if req.Separators != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultChunkSeparators, *req.Separators); err != nil {
			return err
		}
	}
	if req.EnableParentChild != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultParentChildEnabled, *req.EnableParentChild); err != nil {
			return err
		}
	}
	if req.ParentChunkSize != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultParentChunkSize, int64(*req.ParentChunkSize)); err != nil {
			return err
		}
	}
	if req.ChildChunkSize != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultChildChunkSize, int64(*req.ChildChunkSize)); err != nil {
			return err
		}
	}
	return nil
}

func (h *SystemHandler) persistKBDefaultIndexing(ctx context.Context, req kbDefaultsIndexingRequest) error {
	if req.VectorEnabled != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultIndexVectorEnabled, *req.VectorEnabled); err != nil {
			return err
		}
	}
	if req.KeywordEnabled != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultIndexKeywordEnabled, *req.KeywordEnabled); err != nil {
			return err
		}
	}
	if req.WikiEnabled != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultIndexWikiEnabled, *req.WikiEnabled); err != nil {
			return err
		}
	}
	if req.GraphEnabled != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultIndexGraphEnabled, *req.GraphEnabled); err != nil {
			return err
		}
	}
	return nil
}

func (h *SystemHandler) persistKBDefaultMultimodal(ctx context.Context, req kbDefaultsMultimodalRequest) error {
	if req.VLMEnabled != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultVLMEnabled, *req.VLMEnabled); err != nil {
			return err
		}
	}
	if req.VLMModelID != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultVLMModelID, strings.TrimSpace(*req.VLMModelID)); err != nil {
			return err
		}
	}
	if req.ASREnabled != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultASREnabled, *req.ASREnabled); err != nil {
			return err
		}
	}
	if req.ASRModelID != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultASRModelID, strings.TrimSpace(*req.ASRModelID)); err != nil {
			return err
		}
	}
	return nil
}

func (h *SystemHandler) persistKBDefaultQuestionGeneration(ctx context.Context, req kbDefaultsQuestionGenerationRequest) error {
	if req.Enabled != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultQuestionGenerationEnabled, *req.Enabled); err != nil {
			return err
		}
	}
	if req.QuestionCount != nil {
		if err := h.updateKBDefault(ctx, appservice.KBDefaultQuestionGenerationCount, int64(*req.QuestionCount)); err != nil {
			return err
		}
	}
	return nil
}

func (h *SystemHandler) updateKBDefault(ctx context.Context, key string, value any) error {
	_, err := h.systemSettingSvc.Update(ctx, key, value)
	return err
}
