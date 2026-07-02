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
	wikaeval "github.com/Tencent/WeKnora/internal/wika/evaluation"
	wikasearch "github.com/Tencent/WeKnora/internal/wika/search"
	"gorm.io/gorm"
)

type stubWikaEvaluationService struct {
	createDatasetInput *wikaeval.CreateDatasetInput
	addQAItemInput     *wikaeval.AddQAItemInput
	runInput           *wikaeval.RunInput
	dryRunInput        *wikaeval.DryRunInput
	exportInput        *wikaeval.ExportDatasetInput
	importInput        *wikaeval.ImportDatasetInput
	trendInput         *wikaeval.TrendInput
	exportErr          error
	importErr          error
	addErr             error
	runErr             error
}

func (s *stubWikaEvaluationService) CreateDataset(_ context.Context, input wikaeval.CreateDatasetInput) (*types.WikaEvalDataset, error) {
	s.createDatasetInput = &input
	return &types.WikaEvalDataset{ID: 11, TenantID: input.TenantID, KBID: input.KBID, Name: input.Name, CreatedBy: input.ActorID}, nil
}

func (s *stubWikaEvaluationService) AddQAItem(_ context.Context, input wikaeval.AddQAItemInput) (*types.WikaEvalQAItem, error) {
	s.addQAItemInput = &input
	if s.addErr != nil {
		return nil, s.addErr
	}
	return &types.WikaEvalQAItem{ID: 21, DatasetID: input.DatasetID, Question: input.Question, ExpectedKnowledgeIDs: types.JSON([]byte(`["k-1"]`))}, nil
}

func (s *stubWikaEvaluationService) RunEvaluation(_ context.Context, input wikaeval.RunInput) (*types.WikaEvalRun, error) {
	s.runInput = &input
	if s.runErr != nil {
		return nil, s.runErr
	}
	return &types.WikaEvalRun{ID: 31, TenantID: input.TenantID, KBID: input.KBID, DatasetID: input.DatasetID, Status: wikaeval.RunStatusCompleted, Total: 1, RecallAt5: 1}, nil
}

func (s *stubWikaEvaluationService) DryRun(_ context.Context, input wikaeval.DryRunInput) (*wikaeval.DryRunResult, error) {
	s.dryRunInput = &input
	return &wikaeval.DryRunResult{
		Question: input.Question,
		Results:  []wikasearch.ResultItem{{KnowledgeID: "k-1", Title: "命中知识"}},
	}, nil
}

func (s *stubWikaEvaluationService) ExportDataset(_ context.Context, input wikaeval.ExportDatasetInput) (*wikaeval.ExportDatasetResult, error) {
	s.exportInput = &input
	if s.exportErr != nil {
		return nil, s.exportErr
	}
	return &wikaeval.ExportDatasetResult{
		Dataset: wikaeval.ExportDatasetMeta{ID: input.DatasetID, KBID: input.KBID, Name: "黄金 QA"},
		Items: []wikaeval.ExportQAItem{
			{ID: 21, Question: "如何排查索引延迟？", ExpectedKnowledgeIDs: []string{"k-1"}, Enabled: true, Version: 1},
		},
	}, nil
}

func (s *stubWikaEvaluationService) ImportDataset(_ context.Context, input wikaeval.ImportDatasetInput) (*wikaeval.ImportDatasetResult, error) {
	s.importInput = &input
	if s.importErr != nil {
		return nil, s.importErr
	}
	return &wikaeval.ImportDatasetResult{
		Imported: len(input.Items),
		Items: []wikaeval.ImportedQAItem{
			{ID: 21, Question: input.Items[0].Question, Enabled: input.Items[0].Enabled, Version: 1},
		},
	}, nil
}

func (s *stubWikaEvaluationService) Trend(_ context.Context, input wikaeval.TrendInput) (*wikaeval.TrendResult, error) {
	s.trendInput = &input
	return &wikaeval.TrendResult{Runs: []wikaeval.TrendRun{{RunID: 31, RecallAt5: 1, MRR: 1, NDCGAt5: 1, Total: 1}}}, nil
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
	r.POST("/api/v1/wika/kb/:id/eval/datasets/:dataset_id/import", h.ImportDataset)
	r.GET("/api/v1/wika/kb/:id/eval/datasets/:dataset_id/export", h.ExportDataset)
	r.POST("/api/v1/wika/kb/:id/eval/dry-run", h.DryRun)
	r.POST("/api/v1/wika/kb/:id/eval/runs", h.RunEvaluation)
	r.GET("/api/v1/wika/kb/:id/eval/trend", h.Trend)
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
		service.addQAItemInput.ActorID != "u-test" ||
		service.addQAItemInput.TenantID != 80 ||
		service.addQAItemInput.KBID != "kb-team" ||
		service.addQAItemInput.DatasetID != 11 ||
		service.addQAItemInput.Question != "如何排查索引延迟？" ||
		len(service.addQAItemInput.ExpectedKnowledgeIDs) != 1 ||
		len(service.addQAItemInput.ExpectedChunkIDs) != 1 {
		t.Fatalf("unexpected add qa input: %+v", service.addQAItemInput)
	}
}

func TestWikaEvaluationAddQAItemMapsMissingDatasetToNotFound(t *testing.T) {
	service := &stubWikaEvaluationService{addErr: gorm.ErrRecordNotFound}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/datasets/11/items", `{"question":"Q1"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
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

func TestWikaEvaluationRunMapsMissingDatasetToNotFound(t *testing.T) {
	service := &stubWikaEvaluationService{runErr: gorm.ErrRecordNotFound}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/runs", `{"dataset_id":11}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaEvaluationDryRunPassesQuestionAndReturnsResults(t *testing.T) {
	service := &stubWikaEvaluationService{}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/dry-run", `{"question":"如何排查默认模型？","limit":3}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.dryRunInput == nil ||
		service.dryRunInput.ActorID != "u-test" ||
		service.dryRunInput.TenantID != 80 ||
		service.dryRunInput.KBID != "kb-team" ||
		service.dryRunInput.Question != "如何排查默认模型？" ||
		service.dryRunInput.Limit != 3 {
		t.Fatalf("unexpected dry-run input: %+v", service.dryRunInput)
	}
	var body wikaeval.DryRunResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Results) != 1 || body.Results[0].KnowledgeID != "k-1" {
		t.Fatalf("unexpected dry-run response: %+v", body)
	}
}

func TestWikaEvaluationExportDatasetPassesScopeAndOmitsExpectedAnswer(t *testing.T) {
	service := &stubWikaEvaluationService{}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodGet, "/api/v1/wika/kb/kb-team/eval/datasets/11/export", ``)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.exportInput == nil ||
		service.exportInput.ActorID != "u-test" ||
		service.exportInput.TenantID != 80 ||
		service.exportInput.KBID != "kb-team" ||
		service.exportInput.DatasetID != 11 {
		t.Fatalf("unexpected export input: %+v", service.exportInput)
	}
	var body struct {
		Items []struct {
			Question       string `json:"question"`
			ExpectedAnswer string `json:"expected_answer"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Question == "" {
		t.Fatalf("unexpected export response: %+v", body)
	}
	if body.Items[0].ExpectedAnswer != "" {
		t.Fatalf("expected answer must be omitted, got %q", body.Items[0].ExpectedAnswer)
	}
}

func TestWikaEvaluationImportDatasetPassesScopeAndItems(t *testing.T) {
	service := &stubWikaEvaluationService{}
	r := newWikaEvaluationTestRouter(service)

	body := `{"items":[{"question":"如何排查默认模型？","expected_answer":"查看系统默认 KB 配置。","expected_knowledge_ids":["k-1"],"expected_chunk_ids":["c-1"],"tags":["p2"],"enabled":true}]}`
	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/datasets/11/import", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.importInput == nil ||
		service.importInput.ActorID != "u-test" ||
		service.importInput.TenantID != 80 ||
		service.importInput.KBID != "kb-team" ||
		service.importInput.DatasetID != 11 ||
		len(service.importInput.Items) != 1 ||
		service.importInput.Items[0].Question != "如何排查默认模型？" {
		t.Fatalf("unexpected import input: %+v", service.importInput)
	}
	var bodyResp wikaeval.ImportDatasetResult
	if err := json.Unmarshal(w.Body.Bytes(), &bodyResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if bodyResp.Imported != 1 || len(bodyResp.Items) != 1 {
		t.Fatalf("unexpected import response: %+v", bodyResp)
	}
}

func TestWikaEvaluationExportDatasetMapsMissingDatasetToNotFound(t *testing.T) {
	service := &stubWikaEvaluationService{exportErr: gorm.ErrRecordNotFound}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodGet, "/api/v1/wika/kb/kb-team/eval/datasets/11/export", ``)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaEvaluationImportDatasetMapsMissingDatasetToNotFound(t *testing.T) {
	service := &stubWikaEvaluationService{importErr: gorm.ErrRecordNotFound}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodPost, "/api/v1/wika/kb/kb-team/eval/datasets/11/import", `{"items":[{"question":"Q1"}]}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaEvaluationTrendPassesKBAndLimit(t *testing.T) {
	service := &stubWikaEvaluationService{}
	r := newWikaEvaluationTestRouter(service)

	w := doWikaEvaluationJSON(t, r, http.MethodGet, "/api/v1/wika/kb/kb-team/eval/trend?limit=7", ``)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.trendInput == nil ||
		service.trendInput.ActorID != "u-test" ||
		service.trendInput.TenantID != 80 ||
		service.trendInput.KBID != "kb-team" ||
		service.trendInput.Limit != 7 {
		t.Fatalf("unexpected trend input: %+v", service.trendInput)
	}
	var body wikaeval.TrendResult
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Runs) != 1 || body.Runs[0].RunID != 31 {
		t.Fatalf("unexpected trend response: %+v", body)
	}
}
