package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	wikaeval "github.com/Tencent/WeKnora/internal/wika/evaluation"
)

type stubWikaEvaluationService struct {
	createDatasetInput *wikaeval.CreateDatasetInput
	addQAItemInput     *wikaeval.AddQAItemInput
	runInput           *wikaeval.RunInput
}

func (s *stubWikaEvaluationService) CreateDataset(_ context.Context, input wikaeval.CreateDatasetInput) (*types.WikaEvalDataset, error) {
	s.createDatasetInput = &input
	return &types.WikaEvalDataset{ID: 11, TenantID: input.TenantID, KBID: input.KBID, Name: input.Name, CreatedBy: input.ActorID}, nil
}

func (s *stubWikaEvaluationService) AddQAItem(_ context.Context, input wikaeval.AddQAItemInput) (*types.WikaEvalQAItem, error) {
	s.addQAItemInput = &input
	return &types.WikaEvalQAItem{ID: 21, DatasetID: input.DatasetID, Question: input.Question, ExpectedKnowledgeIDs: types.JSON([]byte(`["k-1"]`))}, nil
}

func (s *stubWikaEvaluationService) RunEvaluation(_ context.Context, input wikaeval.RunInput) (*types.WikaEvalRun, error) {
	s.runInput = &input
	return &types.WikaEvalRun{ID: 31, TenantID: input.TenantID, KBID: input.KBID, DatasetID: input.DatasetID, Status: wikaeval.RunStatusCompleted, Total: 1, RecallAt5: 1}, nil
}

func newWikaEvaluationTestRouter(service *stubWikaEvaluationService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	h := &WikaEvaluationHandler{service: service}
	r.POST("/api/v1/wika/kb/:id/eval/datasets", h.CreateDataset)
	r.POST("/api/v1/wika/kb/:id/eval/datasets/:dataset_id/items", h.AddQAItem)
	r.POST("/api/v1/wika/kb/:id/eval/runs", h.RunEvaluation)
	return r
}

func doWikaEvaluationJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestWikaEvaluationCreateDatasetPassesTenantKBAndActor(t *testing.T) {
	service := &stubWikaEvaluationService{}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/datasets", `{"name":"团队检索黄金 QA","description":"P2 smoke"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.createDatasetInput == nil ||
		service.createDatasetInput.ActorID != "u-test" ||
		service.createDatasetInput.TenantID != 80 ||
		service.createDatasetInput.KBID != "kb-team" ||
		service.createDatasetInput.Name != "团队检索黄金 QA" {
		t.Fatalf("unexpected create dataset input: %+v", service.createDatasetInput)
	}
}

func TestWikaEvaluationAddQAItemPassesExpectedIDs(t *testing.T) {
	service := &stubWikaEvaluationService{}
	r := newWikaEvaluationTestRouter(service)

	body := `{
		"question":"如何排查索引延迟？",
		"expected_answer":"查看队列、embedding 任务和索引状态。",
		"expected_knowledge_ids":["k-1"],
		"expected_chunk_ids":["c-1"],
		"tags":["search"]
	}`
	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/datasets/11/items", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.addQAItemInput == nil ||
		service.addQAItemInput.DatasetID != 11 ||
		service.addQAItemInput.Question != "如何排查索引延迟？" ||
		len(service.addQAItemInput.ExpectedKnowledgeIDs) != 1 ||
		len(service.addQAItemInput.ExpectedChunkIDs) != 1 {
		t.Fatalf("unexpected add qa input: %+v", service.addQAItemInput)
	}
}

func TestWikaEvaluationRunPassesDatasetAndActorToService(t *testing.T) {
	service := &stubWikaEvaluationService{}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/runs", `{"dataset_id":11}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.runInput == nil ||
		service.runInput.ActorID != "u-test" ||
		service.runInput.TenantID != 80 ||
		service.runInput.KBID != "kb-team" ||
		service.runInput.DatasetID != 11 {
		t.Fatalf("unexpected run input: %+v", service.runInput)
	}
}
