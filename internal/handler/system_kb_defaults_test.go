package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type kbDefaultsSettingService struct {
	interfaces.SystemSettingService
	strings     map[string]string
	ints        map[string]int64
	bools       map[string]bool
	stringLists map[string][]string
	updates     map[string]any
}

func (s *kbDefaultsSettingService) GetString(_ context.Context, key string, _ string, def string) string {
	if v, ok := s.strings[key]; ok {
		return v
	}
	return def
}

func (s *kbDefaultsSettingService) GetInt(_ context.Context, key string, _ string, def int64) int64 {
	if v, ok := s.ints[key]; ok {
		return v
	}
	return def
}

func (s *kbDefaultsSettingService) GetBool(_ context.Context, key string, _ string, def bool) bool {
	if v, ok := s.bools[key]; ok {
		return v
	}
	return def
}

func (s *kbDefaultsSettingService) GetStringList(_ context.Context, key string, _ string, def []string) []string {
	if v, ok := s.stringLists[key]; ok {
		return v
	}
	return def
}

func (s *kbDefaultsSettingService) Update(_ context.Context, key string, value any) (*types.SystemSetting, error) {
	if s.updates == nil {
		s.updates = map[string]any{}
	}
	s.updates[key] = value
	raw, _ := json.Marshal(value)
	return &types.SystemSetting{Key: key, Value: types.JSON(raw)}, nil
}

func newKBDefaultsRouter(settings *kbDefaultsSettingService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-system")
		c.Next()
	})
	h := &SystemHandler{systemSettingSvc: settings}
	r.GET("/api/v1/system/admin/kb-defaults", h.GetKnowledgeBaseDefaults)
	r.PUT("/api/v1/system/admin/kb-defaults", h.UpdateKnowledgeBaseDefaults)
	return r
}

func TestGetKnowledgeBaseDefaultsReturnsAggregatedSystemSettings(t *testing.T) {
	settings := &kbDefaultsSettingService{
		strings: map[string]string{
			"kb.default_llm_model_id":       "builtin-llm",
			"kb.default_embedding_model_id": "builtin-embedding",
			"kb.default_storage_provider":   "minio",
			"kb.default_vlm_model_id":       "builtin-vlm",
			"kb.default_asr_model_id":       "builtin-asr",
		},
		ints: map[string]int64{
			"kb.default_chunk_size":                768,
			"kb.default_chunk_overlap":             96,
			"kb.default_question_generation_count": 4,
		},
		bools: map[string]bool{
			"kb.default_index_vector_enabled":        true,
			"kb.default_index_keyword_enabled":       false,
			"kb.default_index_wiki_enabled":          true,
			"kb.default_index_graph_enabled":         false,
			"kb.default_vlm_enabled":                 true,
			"kb.default_asr_enabled":                 true,
			"kb.default_question_generation_enabled": true,
		},
		stringLists: map[string][]string{
			"kb.default_chunk_separators": {"\n\n", "\n", "。"},
		},
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/kb-defaults", nil)
	newKBDefaultsRouter(settings).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var got knowledgeBaseDefaultsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}
	if got.LLMModelID != "builtin-llm" ||
		got.EmbeddingModelID != "builtin-embedding" ||
		got.StorageProvider != "minio" ||
		got.Chunking.ChunkSize != 768 ||
		got.Chunking.ChunkOverlap != 96 ||
		len(got.Chunking.Separators) != 3 ||
		!got.Indexing.VectorEnabled ||
		got.Indexing.KeywordEnabled ||
		!got.Indexing.WikiEnabled ||
		!got.Multimodal.VLMEnabled ||
		got.Multimodal.VLMModelID != "builtin-vlm" ||
		!got.Multimodal.ASREnabled ||
		got.Multimodal.ASRModelID != "builtin-asr" ||
		!got.QuestionGeneration.Enabled ||
		got.QuestionGeneration.QuestionCount != 4 {
		t.Fatalf("unexpected defaults response: %+v", got)
	}
}

func TestUpdateKnowledgeBaseDefaultsPersistsAllProvidedFields(t *testing.T) {
	settings := &kbDefaultsSettingService{
		strings:     map[string]string{},
		ints:        map[string]int64{},
		bools:       map[string]bool{},
		stringLists: map[string][]string{},
	}
	body := []byte(`{
		"llm_model_id":"builtin-llm",
		"embedding_model_id":"builtin-embedding",
		"storage_provider":"local",
		"chunking":{"chunk_size":768,"chunk_overlap":96,"separators":["\n\n","\n","。"]},
		"indexing":{"vector_enabled":true,"keyword_enabled":false,"wiki_enabled":true,"graph_enabled":false},
		"multimodal":{"vlm_enabled":true,"vlm_model_id":"builtin-vlm","asr_enabled":true,"asr_model_id":"builtin-asr"},
		"question_generation":{"enabled":true,"question_count":4}
	}`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/admin/kb-defaults", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	newKBDefaultsRouter(settings).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	expected := map[string]any{
		"kb.default_llm_model_id":                "builtin-llm",
		"kb.default_embedding_model_id":          "builtin-embedding",
		"kb.default_storage_provider":            "local",
		"kb.default_chunk_size":                  int64(768),
		"kb.default_chunk_overlap":               int64(96),
		"kb.default_chunk_separators":            []string{"\n\n", "\n", "。"},
		"kb.default_index_vector_enabled":        true,
		"kb.default_index_keyword_enabled":       false,
		"kb.default_index_wiki_enabled":          true,
		"kb.default_index_graph_enabled":         false,
		"kb.default_vlm_enabled":                 true,
		"kb.default_vlm_model_id":                "builtin-vlm",
		"kb.default_asr_enabled":                 true,
		"kb.default_asr_model_id":                "builtin-asr",
		"kb.default_question_generation_enabled": true,
		"kb.default_question_generation_count":   int64(4),
	}
	for key, want := range expected {
		got, ok := settings.updates[key]
		if !ok {
			t.Fatalf("expected update for %s, got updates=%v", key, settings.updates)
		}
		if !jsonEqualForTest(got, want) {
			t.Fatalf("unexpected update for %s: got %#v want %#v", key, got, want)
		}
	}
}

func jsonEqualForTest(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return bytes.Equal(aj, bj)
}
