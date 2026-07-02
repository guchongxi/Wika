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
	wikaevalschedule "github.com/Tencent/WeKnora/internal/wika/governance/evalschedule"
)

type stubWikaEvalScheduleService struct {
	createInput  *wikaevalschedule.CreateScheduleInput
	createErr    error
	listInput    *wikaevalschedule.ListInput
	updateInput  *wikaevalschedule.UpdateScheduleInput
	updateErr    error
	disableInput *wikaevalschedule.DisableScheduleInput
	disableErr   error
}

func (s *stubWikaEvalScheduleService) CreateSchedule(_ context.Context, input wikaevalschedule.CreateScheduleInput) (*types.WikaEvalSchedule, error) {
	s.createInput = &input
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &types.WikaEvalSchedule{ID: 31, TenantID: input.TenantID, KBID: input.KBID, DatasetID: input.DatasetID, Enabled: input.Enabled, CronExpr: input.CronExpr, CreatedBy: input.ActorID}, nil
}

func (s *stubWikaEvalScheduleService) List(_ context.Context, input wikaevalschedule.ListInput) (*wikaevalschedule.ListResult, error) {
	s.listInput = &input
	return &wikaevalschedule.ListResult{
		Schedules: []*types.WikaEvalSchedule{{
			ID:        31,
			TenantID:  input.TenantID,
			KBID:      input.KBID,
			DatasetID: 11,
			Enabled:   true,
			CronExpr:  "0 * * * *",
			CreatedBy: input.ActorID,
		}},
		Total: 1,
	}, nil
}

func (s *stubWikaEvalScheduleService) UpdateSchedule(_ context.Context, input wikaevalschedule.UpdateScheduleInput) (*types.WikaEvalSchedule, error) {
	s.updateInput = &input
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	return &types.WikaEvalSchedule{ID: input.ScheduleID, Enabled: input.Enabled, CronExpr: input.CronExpr, CreatedBy: input.ActorID}, nil
}

func (s *stubWikaEvalScheduleService) DisableSchedule(_ context.Context, input wikaevalschedule.DisableScheduleInput) (*types.WikaEvalSchedule, error) {
	s.disableInput = &input
	if s.disableErr != nil {
		return nil, s.disableErr
	}
	return &types.WikaEvalSchedule{ID: input.ScheduleID, Enabled: false, CreatedBy: input.ActorID}, nil
}

func newWikaEvalScheduleTestRouter(service *stubWikaEvalScheduleService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.UserIDContextKey.String(), "u-test")
		c.Set(types.TenantIDContextKey.String(), uint64(80))
		c.Next()
	})
	h := &WikaEvalScheduleHandler{service: service}
	r.GET("/api/v1/wika/kb/:id/eval/schedules", h.List)
	r.POST("/api/v1/wika/kb/:id/eval/schedules", h.CreateSchedule)
	r.PUT("/api/v1/wika/kb/:id/eval/schedules/:schedule_id", h.UpdateSchedule)
	r.DELETE("/api/v1/wika/kb/:id/eval/schedules/:schedule_id", h.DisableSchedule)
	return r
}

func TestWikaEvalScheduleListPassesActorTenantKBAndFilters(t *testing.T) {
	service := &stubWikaEvalScheduleService{}
	r := newWikaEvalScheduleTestRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wika/kb/kb-team/eval/schedules?enabled=true&limit=20", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.listInput == nil ||
		service.listInput.ActorID != "u-test" ||
		service.listInput.TenantID != 80 ||
		service.listInput.KBID != "kb-team" ||
		service.listInput.Enabled == nil ||
		!*service.listInput.Enabled ||
		service.listInput.Limit != 20 {
		t.Fatalf("unexpected list input: %+v", service.listInput)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"schedules"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"total"`)) {
		t.Fatalf("expected schedules and total response, got %s", w.Body.String())
	}
}

func TestWikaEvalScheduleCreatePassesActorTenantKBAndCron(t *testing.T) {
	service := &stubWikaEvalScheduleService{}
	r := newWikaEvalScheduleTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/eval/schedules", bytes.NewBufferString(`{"dataset_id":11,"cron_expr":"0 * * * *","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.createInput == nil ||
		service.createInput.ActorID != "u-test" ||
		service.createInput.TenantID != 80 ||
		service.createInput.KBID != "kb-team" ||
		service.createInput.DatasetID != 11 ||
		service.createInput.CronExpr != "0 * * * *" ||
		!service.createInput.Enabled {
		t.Fatalf("unexpected create schedule input: %+v", service.createInput)
	}
}

func TestWikaEvalScheduleInvalidCronReturnsBadRequest(t *testing.T) {
	service := &stubWikaEvalScheduleService{createErr: wikaevalschedule.ErrInvalidSchedule}
	r := newWikaEvalScheduleTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/eval/schedules", bytes.NewBufferString(`{"dataset_id":11,"cron_expr":"*/30 * * * *","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaEvalScheduleDuplicateEnabledScheduleReturnsConflict(t *testing.T) {
	service := &stubWikaEvalScheduleService{createErr: wikaevalschedule.ErrScheduleConflict}
	r := newWikaEvalScheduleTestRouter(service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/wika/kb/kb-team/eval/schedules", bytes.NewBufferString(`{"dataset_id":11,"cron_expr":"0 * * * *","enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestWikaEvalScheduleUpdatePassesScheduleIDAndPayload(t *testing.T) {
	service := &stubWikaEvalScheduleService{}
	r := newWikaEvalScheduleTestRouter(service)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/wika/kb/kb-team/eval/schedules/31", bytes.NewBufferString(`{"cron_expr":"0 3 * * *","enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.updateInput == nil ||
		service.updateInput.ActorID != "u-test" ||
		service.updateInput.ScheduleID != 31 ||
		service.updateInput.CronExpr != "0 3 * * *" ||
		service.updateInput.Enabled {
		t.Fatalf("unexpected update schedule input: %+v", service.updateInput)
	}
}

func TestWikaEvalScheduleDisablePassesScheduleID(t *testing.T) {
	service := &stubWikaEvalScheduleService{}
	r := newWikaEvalScheduleTestRouter(service)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/wika/kb/kb-team/eval/schedules/31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if service.disableInput == nil ||
		service.disableInput.ActorID != "u-test" ||
		service.disableInput.ScheduleID != 31 {
		t.Fatalf("unexpected disable schedule input: %+v", service.disableInput)
	}
}
