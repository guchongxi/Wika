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
	wikaurlrefresh "github.com/Tencent/WeKnora/internal/wika/governance/urlrefresh"
)

type stubWikaURLRefreshService struct {
	createInput   *wikaurlrefresh.CreateJobInput
	listInput     *wikaurlrefresh.ListInput
	scheduleInput *wikaurlrefresh.CreateOrUpdateScheduleInput
	updateInput   *wikaurlrefresh.UpdateScheduleInput
	disableInput  *wikaurlrefresh.DisableScheduleInput
	reviewInput   *wikaurlrefresh.ReviewJobInput
	createErr     error
	scheduleErr   error
	reviewErr     error
}

type stubWikaURLRefreshKnowledgeReader struct {
	knowledge *types.Knowledge
	err       error
}

func (s *stubWikaURLRefreshKnowledgeReader) GetKnowledgeByID(_ context.Context, id string) (*types.Knowledge, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.knowledge != nil {
		return s.knowledge, nil
	}
	return &types.Knowledge{ID: id, KnowledgeBaseID: "kb-url"}, nil
}

func (s *stubWikaURLRefreshService) CreateJob(_ context.Context, input wikaurlrefresh.CreateJobInput) (*types.WikaURLRefreshJob, error) {
	s.createInput = &input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &types.WikaURLRefreshJob{ID: 11, TenantID: input.TenantID, KBID: input.KBID, KnowledgeID: input.KnowledgeID, SourceURL: input.SourceURL, Status: wikaurlrefresh.JobStatusPending}, nil
}

func (s *stubWikaURLRefreshService) List(_ context.Context, input wikaurlrefresh.ListInput) (*wikaurlrefresh.ListResult, error) {
	s.listInput = &input
	return &wikaurlrefresh.ListResult{
		Jobs: []*types.WikaURLRefreshJob{{
			ID:             11,
			TenantID:       input.TenantID,
			KBID:           input.KBID,
			KnowledgeID:    input.KnowledgeID,
			Status:         wikaurlrefresh.JobStatusPendingReview,
			FetchedContent: "待确认正文不应在列表泄露",
		}},
		Schedules: []*types.WikaURLRefreshSchedule{{
			ID:          21,
			TenantID:    input.TenantID,
			KBID:        input.KBID,
			KnowledgeID: input.KnowledgeID,
			Enabled:     true,
		}},
	}, nil
}

func (s *stubWikaURLRefreshService) ReviewJob(_ context.Context, input wikaurlrefresh.ReviewJobInput) (*wikaurlrefresh.ReviewJobResult, error) {
	s.reviewInput = &input
	if s.reviewErr != nil {
		return nil, s.reviewErr
	}
	status := wikaurlrefresh.JobStatusApplied
	if input.Decision == wikaurlrefresh.ReviewDecisionReject {
		status = wikaurlrefresh.JobStatusRejected
	}
	return &wikaurlrefresh.ReviewJobResult{JobID: input.JobID, Status: status, VersionID: 700}, nil
}

func (s *stubWikaURLRefreshService) CreateOrUpdateSchedule(_ context.Context, input wikaurlrefresh.CreateOrUpdateScheduleInput) (*types.WikaURLRefreshSchedule, error) {
	s.scheduleInput = &input
	if s.scheduleErr != nil {
		return nil, s.scheduleErr
	}
	return &types.WikaURLRefreshSchedule{ID: 21, TenantID: input.TenantID, KBID: input.KBID, KnowledgeID: input.KnowledgeID, SourceURL: input.SourceURL, Enabled: input.Enabled, CronExpr: input.CronExpr, CreatedBy: input.ActorID}, nil
}

func (s *stubWikaURLRefreshService) UpdateSchedule(_ context.Context, input wikaurlrefresh.UpdateScheduleInput) (*types.WikaURLRefreshSchedule, error) {
	s.updateInput = &input
	return &types.WikaURLRefreshSchedule{ID: input.ScheduleID, Enabled: input.Enabled, CronExpr: input.CronExpr}, nil
}

func (s *stubWikaURLRefreshService) DisableSchedule(_ context.Context, input wikaurlrefresh.DisableScheduleInput) (*types.WikaURLRefreshSchedule, error) {
	s.disableInput = &input
	return &types.WikaURLRefreshSchedule{ID: input.ScheduleID, Enabled: false}, nil
}

func newWikaURLRefreshTestRouter(service *stubWikaURLRefreshService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-reviewer")
		c.Set(types.TenantIDContextKey.String(), uint64(90))
		c.Next()
	})
	h := &WikaURLRefreshHandler{service: service, knowledge: &stubWikaURLRefreshKnowledgeReader{}}
	r.GET("/api/v1/wika/knowledge/:id/url-refresh", h.List)
	r.POST("/api/v1/wika/knowledge/:id/url-refresh", h.CreateJob)
	r.PUT("/api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id", h.UpdateSchedule)
	r.DELETE("/api/v1/wika/knowledge/:id/url-refresh/schedules/:schedule_id", h.DisableSchedule)
	r.PUT("/api/v1/wika/url-refresh/:id/review", h.ReviewJob)
	return r
}

func TestWikaURLRefreshListPassesActorTenantKnowledgeAndFilters(t *testing.T) {
	service := &stubWikaURLRefreshService{}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/knowledge/k-url/url-refresh?status=pending_review&limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.listInput == nil ||
		service.listInput.ActorID != "u-reviewer" ||
		service.listInput.TenantID != 90 ||
		service.listInput.KBID != "kb-url" ||
		service.listInput.KnowledgeID != "k-url" ||
		service.listInput.Status != wikaurlrefresh.JobStatusPendingReview ||
		service.listInput.Limit != 20 {
		t.Fatalf("unexpected list input: %+v", service.listInput)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"jobs"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"schedules"`)) {
		t.Fatalf("expected jobs and schedules response, got %s", w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("待确认正文不应在列表泄露")) || bytes.Contains(w.Body.Bytes(), []byte("fetched_content")) {
		t.Fatalf("list response must not expose fetched content, got %s", w.Body.String())
	}
}

func TestWikaURLRefreshCreateJobPassesActorTenantKnowledgeAndURL(t *testing.T) {
	service := &stubWikaURLRefreshService{}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/k-url/url-refresh", bytes.NewBufferString(`{"source_url":"https://example.com/doc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.createInput == nil ||
		service.createInput.ActorID != "u-reviewer" ||
		service.createInput.TenantID != 90 ||
		service.createInput.KBID != "kb-url" ||
		service.createInput.KnowledgeID != "k-url" ||
		service.createInput.SourceURL != "https://example.com/doc" {
		t.Fatalf("unexpected create input: %+v", service.createInput)
	}
}

func TestWikaURLRefreshCreateSchedulePassesCronAndEnabled(t *testing.T) {
	service := &stubWikaURLRefreshService{}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/k-url/url-refresh", bytes.NewBufferString(`{"source_url":"https://example.com/doc","schedule":{"enabled":true,"cron_expr":"0 * * * *"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.scheduleInput == nil ||
		service.scheduleInput.ActorID != "u-reviewer" ||
		service.scheduleInput.TenantID != 90 ||
		service.scheduleInput.KBID != "kb-url" ||
		service.scheduleInput.KnowledgeID != "k-url" ||
		service.scheduleInput.SourceURL != "https://example.com/doc" ||
		service.scheduleInput.CronExpr != "0 * * * *" ||
		!service.scheduleInput.Enabled {
		t.Fatalf("unexpected schedule input: %+v", service.scheduleInput)
	}
	if service.createInput != nil {
		t.Fatalf("schedule request should not create immediate job: %+v", service.createInput)
	}
}

func TestWikaURLRefreshUpdateScheduleParsesIDCronAndEnabled(t *testing.T) {
	service := &stubWikaURLRefreshService{}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/knowledge/k-url/url-refresh/schedules/21", bytes.NewBufferString(`{"enabled":true,"cron_expr":"0 */2 * * *"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.updateInput == nil ||
		service.updateInput.ActorID != "u-reviewer" ||
		service.updateInput.ScheduleID != 21 ||
		service.updateInput.CronExpr != "0 */2 * * *" ||
		!service.updateInput.Enabled {
		t.Fatalf("unexpected update input: %+v", service.updateInput)
	}
}

func TestWikaURLRefreshDisableScheduleParsesID(t *testing.T) {
	service := &stubWikaURLRefreshService{}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/wika/knowledge/k-url/url-refresh/schedules/21", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.disableInput == nil ||
		service.disableInput.ActorID != "u-reviewer" ||
		service.disableInput.ScheduleID != 21 {
		t.Fatalf("unexpected disable input: %+v", service.disableInput)
	}
}

func TestWikaURLRefreshReviewParsesDecisionCommentAndID(t *testing.T) {
	service := &stubWikaURLRefreshService{}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/url-refresh/11/review", bytes.NewBufferString(`{"decision":"apply","comment":"确认采用"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.reviewInput == nil ||
		service.reviewInput.ActorID != "u-reviewer" ||
		service.reviewInput.JobID != 11 ||
		service.reviewInput.Decision != wikaurlrefresh.ReviewDecisionApply ||
		service.reviewInput.Comment != "确认采用" {
		t.Fatalf("unexpected review input: %+v", service.reviewInput)
	}
}

func TestWikaURLRefreshFeatureDisabledReturnsNotFound(t *testing.T) {
	service := &stubWikaURLRefreshService{reviewErr: wikaurlrefresh.ErrFeatureDisabled}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/url-refresh/11/review", bytes.NewBufferString(`{"decision":"apply"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaURLRefreshCreateUnsafeURLReturnsBadRequest(t *testing.T) {
	service := &stubWikaURLRefreshService{createErr: wikaurlrefresh.ErrUnsafeSourceURL}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/k-url/url-refresh", bytes.NewBufferString(`{"source_url":"http://127.0.0.1/admin"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaURLRefreshCreateUnsafeScheduleReturnsBadRequest(t *testing.T) {
	service := &stubWikaURLRefreshService{scheduleErr: wikaurlrefresh.ErrUnsafeSourceURL}
	r := newWikaURLRefreshTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/knowledge/k-url/url-refresh", bytes.NewBufferString(`{"source_url":"http://127.0.0.1/admin","schedule":{"enabled":true,"cron_expr":"0 * * * *"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}
