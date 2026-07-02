package service

import (
	"context"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

const (
	KBDefaultLLMModelID       = "kb.default_llm_model_id"
	KBDefaultEmbeddingModelID = "kb.default_embedding_model_id"
	KBDefaultStorageProvider  = "kb.default_storage_provider"

	KBDefaultChunkSize          = "kb.default_chunk_size"
	KBDefaultChunkOverlap       = "kb.default_chunk_overlap"
	KBDefaultChunkSeparators    = "kb.default_chunk_separators"
	KBDefaultParentChildEnabled = "kb.default_parent_child_enabled"
	KBDefaultParentChunkSize    = "kb.default_parent_chunk_size"
	KBDefaultChildChunkSize     = "kb.default_child_chunk_size"

	KBDefaultIndexVectorEnabled  = "kb.default_index_vector_enabled"
	KBDefaultIndexKeywordEnabled = "kb.default_index_keyword_enabled"
	KBDefaultIndexWikiEnabled    = "kb.default_index_wiki_enabled"
	KBDefaultIndexGraphEnabled   = "kb.default_index_graph_enabled"

	KBDefaultVLMEnabled = "kb.default_vlm_enabled"
	KBDefaultVLMModelID = "kb.default_vlm_model_id"
	KBDefaultASREnabled = "kb.default_asr_enabled"
	KBDefaultASRModelID = "kb.default_asr_model_id"

	KBDefaultQuestionGenerationEnabled = "kb.default_question_generation_enabled"
	KBDefaultQuestionGenerationCount   = "kb.default_question_generation_count"
)

const (
	kbDefaultLLMModelID                = KBDefaultLLMModelID
	kbDefaultEmbeddingModelID          = KBDefaultEmbeddingModelID
	kbDefaultStorageProvider           = KBDefaultStorageProvider
	kbDefaultChunkSize                 = KBDefaultChunkSize
	kbDefaultChunkOverlap              = KBDefaultChunkOverlap
	kbDefaultChunkSeparators           = KBDefaultChunkSeparators
	kbDefaultParentChildEnabled        = KBDefaultParentChildEnabled
	kbDefaultParentChunkSize           = KBDefaultParentChunkSize
	kbDefaultChildChunkSize            = KBDefaultChildChunkSize
	kbDefaultIndexVectorEnabled        = KBDefaultIndexVectorEnabled
	kbDefaultIndexKeywordEnabled       = KBDefaultIndexKeywordEnabled
	kbDefaultIndexWikiEnabled          = KBDefaultIndexWikiEnabled
	kbDefaultIndexGraphEnabled         = KBDefaultIndexGraphEnabled
	kbDefaultVLMEnabled                = KBDefaultVLMEnabled
	kbDefaultVLMModelID                = KBDefaultVLMModelID
	kbDefaultASREnabled                = KBDefaultASREnabled
	kbDefaultASRModelID                = KBDefaultASRModelID
	kbDefaultQuestionGenerationEnabled = KBDefaultQuestionGenerationEnabled
	kbDefaultQuestionGenerationCount   = KBDefaultQuestionGenerationCount
)

var defaultChunkSeparators = []string{"\n\n", "\n", "。", "！", "？", ";", "；"}

func (s *knowledgeBaseService) applySystemKBDefaults(ctx context.Context, kb *types.KnowledgeBase) {
	if s == nil || s.systemSettingSvc == nil || kb == nil {
		return
	}
	settings := s.systemSettingSvc

	if strings.TrimSpace(kb.SummaryModelID) == "" {
		kb.SummaryModelID = strings.TrimSpace(settings.GetString(ctx, kbDefaultLLMModelID, "WEKNORA_KB_DEFAULT_LLM_MODEL_ID", ""))
	}
	if strings.TrimSpace(kb.EmbeddingModelID) == "" {
		kb.EmbeddingModelID = strings.TrimSpace(settings.GetString(ctx, kbDefaultEmbeddingModelID, "WEKNORA_KB_DEFAULT_EMBEDDING_MODEL_ID", ""))
	}

	if kb.ChunkingConfig.ChunkSize <= 0 {
		if v := settings.GetInt(ctx, kbDefaultChunkSize, "WEKNORA_KB_DEFAULT_CHUNK_SIZE", 512); v > 0 {
			kb.ChunkingConfig.ChunkSize = int(v)
		}
	}
	if kb.ChunkingConfig.ChunkOverlap <= 0 {
		if v := settings.GetInt(ctx, kbDefaultChunkOverlap, "WEKNORA_KB_DEFAULT_CHUNK_OVERLAP", 80); v >= 0 {
			kb.ChunkingConfig.ChunkOverlap = int(v)
		}
	}
	if len(kb.ChunkingConfig.Separators) == 0 {
		if v := settings.GetStringList(ctx, kbDefaultChunkSeparators, "WEKNORA_KB_DEFAULT_CHUNK_SEPARATORS", defaultChunkSeparators); len(v) > 0 {
			kb.ChunkingConfig.Separators = v
		}
	}
	if !kb.ChunkingConfig.EnableParentChild {
		kb.ChunkingConfig.EnableParentChild = settings.GetBool(ctx, kbDefaultParentChildEnabled, "WEKNORA_KB_DEFAULT_PARENT_CHILD_ENABLED", false)
	}
	if kb.ChunkingConfig.ParentChunkSize <= 0 {
		if v := settings.GetInt(ctx, kbDefaultParentChunkSize, "WEKNORA_KB_DEFAULT_PARENT_CHUNK_SIZE", 4096); v > 0 {
			kb.ChunkingConfig.ParentChunkSize = int(v)
		}
	}
	if kb.ChunkingConfig.ChildChunkSize <= 0 {
		if v := settings.GetInt(ctx, kbDefaultChildChunkSize, "WEKNORA_KB_DEFAULT_CHILD_CHUNK_SIZE", 384); v > 0 {
			kb.ChunkingConfig.ChildChunkSize = int(v)
		}
	}

	if kb.IndexingStrategy.IsZero() {
		kb.IndexingStrategy = types.IndexingStrategy{
			VectorEnabled:  settings.GetBool(ctx, kbDefaultIndexVectorEnabled, "WEKNORA_KB_DEFAULT_INDEX_VECTOR_ENABLED", true),
			KeywordEnabled: settings.GetBool(ctx, kbDefaultIndexKeywordEnabled, "WEKNORA_KB_DEFAULT_INDEX_KEYWORD_ENABLED", true),
			WikiEnabled:    settings.GetBool(ctx, kbDefaultIndexWikiEnabled, "WEKNORA_KB_DEFAULT_INDEX_WIKI_ENABLED", false),
			GraphEnabled:   settings.GetBool(ctx, kbDefaultIndexGraphEnabled, "WEKNORA_KB_DEFAULT_INDEX_GRAPH_ENABLED", false),
		}
	}

	if strings.TrimSpace(kb.GetStorageProvider()) == "" {
		if provider := strings.ToLower(strings.TrimSpace(settings.GetString(ctx, kbDefaultStorageProvider, "WEKNORA_KB_DEFAULT_STORAGE_PROVIDER", ""))); provider != "" {
			kb.SetStorageProvider(provider)
		}
	}

	if kb.VLMConfig == (types.VLMConfig{}) && settings.GetBool(ctx, kbDefaultVLMEnabled, "WEKNORA_KB_DEFAULT_VLM_ENABLED", false) {
		if modelID := strings.TrimSpace(settings.GetString(ctx, kbDefaultVLMModelID, "WEKNORA_KB_DEFAULT_VLM_MODEL_ID", "")); modelID != "" {
			kb.VLMConfig = types.VLMConfig{Enabled: true, ModelID: modelID}
		}
	}
	if kb.ASRConfig == (types.ASRConfig{}) && settings.GetBool(ctx, kbDefaultASREnabled, "WEKNORA_KB_DEFAULT_ASR_ENABLED", false) {
		if modelID := strings.TrimSpace(settings.GetString(ctx, kbDefaultASRModelID, "WEKNORA_KB_DEFAULT_ASR_MODEL_ID", "")); modelID != "" {
			kb.ASRConfig = types.ASRConfig{Enabled: true, ModelID: modelID}
		}
	}
	if kb.QuestionGenerationConfig == nil && settings.GetBool(ctx, kbDefaultQuestionGenerationEnabled, "WEKNORA_KB_DEFAULT_QUESTION_GENERATION_ENABLED", false) {
		count := settings.GetInt(ctx, kbDefaultQuestionGenerationCount, "WEKNORA_KB_DEFAULT_QUESTION_GENERATION_COUNT", 3)
		if count <= 0 {
			count = 3
		}
		kb.QuestionGenerationConfig = &types.QuestionGenerationConfig{
			Enabled:       true,
			QuestionCount: int(count),
		}
	}
}
