package intake

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeIntakeStore struct {
	defaultKB     DefaultKB
	defaultKBErr  error
	existingID    string
	findKey       string
	savedState    *types.WikaKnowledgeState
	saveCallCount int
	getCallCount  int
}

func (s *fakeIntakeStore) GetPersonalDefaultKB(ctx context.Context, userID string) (DefaultKB, error) {
	s.getCallCount++
	if s.defaultKBErr != nil {
		err := s.defaultKBErr
		s.defaultKBErr = nil
		return DefaultKB{}, err
	}
	return s.defaultKB, nil
}

func (s *fakeIntakeStore) FindKnowledgeIDByIdempotencyKey(ctx context.Context, tenantID uint64, kbID, key string) (string, error) {
	s.findKey = key
	return s.existingID, nil
}

func (s *fakeIntakeStore) SaveKnowledgeState(ctx context.Context, state *types.WikaKnowledgeState) error {
	s.saveCallCount++
	copied := *state
	s.savedState = &copied
	return nil
}

type fakeManualKnowledgeCreator struct {
	kbID    string
	payload *types.ManualKnowledgePayload
	channel string
	calls   int
	resp    *types.Knowledge
}

func (c *fakeManualKnowledgeCreator) CreateKnowledgeFromManual(ctx context.Context, kbID string, payload *types.ManualKnowledgePayload, channel string) (*types.Knowledge, error) {
	c.calls++
	c.kbID = kbID
	c.payload = payload
	c.channel = channel
	if c.resp != nil {
		return c.resp, nil
	}
	return &types.Knowledge{ID: "knowledge-1"}, nil
}

type fakePersonalSpaceEnsurer struct {
	calls       int
	userID      string
	displayName string
	err         error
}

func (e *fakePersonalSpaceEnsurer) GetOrCreatePersonalSpace(ctx context.Context, userID, displayName string) (*types.Tenant, error) {
	e.calls++
	e.userID = userID
	e.displayName = displayName
	if e.err != nil {
		return nil, e.err
	}
	return &types.Tenant{ID: 70, SpaceType: types.SpaceTypePersonal}, nil
}

func TestPushKnowledgePersistsManualKnowledgeAndState(t *testing.T) {
	expiresAt := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	store := &fakeIntakeStore{defaultKB: DefaultKB{TenantID: 70, KBID: "kb-personal"}}
	creator := &fakeManualKnowledgeCreator{}
	svc := &Service{store: store, knowledge: creator}

	got, err := svc.PushKnowledge(context.Background(), PushKnowledgeInput{
		UserID:         "u-test",
		TenantID:       7,
		Title:          "排查记录",
		Content:        "先看日志，再看指标。",
		Source:         "incident-42",
		Tags:           []string{"debug", "runbook"},
		Evidence:       "线上日志片段",
		ExpiresAt:      &expiresAt,
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("PushKnowledge returned error: %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("expected manual knowledge create once, got %d", creator.calls)
	}
	if creator.kbID != "kb-personal" || creator.channel != types.ChannelAPI {
		t.Fatalf("unexpected manual create target: kb=%q channel=%q", creator.kbID, creator.channel)
	}
	if creator.payload == nil {
		t.Fatal("expected manual payload")
	}
	if creator.payload.Title != "排查记录" || creator.payload.Status != types.ManualKnowledgeStatusPublish {
		t.Fatalf("unexpected manual payload metadata: %+v", creator.payload)
	}
	if !strings.Contains(creator.payload.Content, "先看日志，再看指标。") {
		t.Fatalf("manual payload lost content: %q", creator.payload.Content)
	}
	if got.KnowledgeID != "knowledge-1" || got.Status != "created" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if store.savedState == nil {
		t.Fatal("expected Wika knowledge state to be saved")
	}
	if store.savedState.KnowledgeID != "knowledge-1" ||
		store.savedState.TenantID != 70 ||
		store.savedState.KBID != "kb-personal" ||
		store.savedState.IdempotencyKey != "idem-1" ||
		store.savedState.ExpiresAt == nil ||
		!store.savedState.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("unexpected saved state: %+v", store.savedState)
	}
}

func TestPushKnowledgeCreatesPersonalSpaceWhenDefaultKBMissing(t *testing.T) {
	store := &fakeIntakeStore{
		defaultKB:    DefaultKB{TenantID: 70, KBID: "kb-personal"},
		defaultKBErr: ErrPersonalDefaultKBNotFound,
	}
	creator := &fakeManualKnowledgeCreator{}
	spaces := &fakePersonalSpaceEnsurer{}
	svc := &Service{store: store, knowledge: creator, spaces: spaces}

	got, err := svc.PushKnowledge(context.Background(), PushKnowledgeInput{
		UserID:  "u-test",
		Title:   "首次入库",
		Content: "首次使用 Wika 时应自动补齐个人空间和默认知识库。",
	})
	if err != nil {
		t.Fatalf("PushKnowledge returned error: %v", err)
	}
	if spaces.calls != 1 || spaces.userID != "u-test" {
		t.Fatalf("expected personal space ensure once for u-test, got calls=%d user=%q", spaces.calls, spaces.userID)
	}
	if store.getCallCount != 2 {
		t.Fatalf("expected default KB lookup before and after ensure, got %d", store.getCallCount)
	}
	if creator.kbID != "kb-personal" || got.KnowledgeID != "knowledge-1" {
		t.Fatalf("expected knowledge created in personal default KB, kb=%q result=%+v", creator.kbID, got)
	}
}

func TestPushKnowledgeReturnsOriginalMissingDefaultKBErrorWithoutSpaceEnsurer(t *testing.T) {
	store := &fakeIntakeStore{defaultKBErr: ErrPersonalDefaultKBNotFound}
	creator := &fakeManualKnowledgeCreator{}
	svc := &Service{store: store, knowledge: creator}

	_, err := svc.PushKnowledge(context.Background(), PushKnowledgeInput{
		UserID:  "u-test",
		Title:   "首次入库",
		Content: "缺少个人空间初始化器时不应吞掉原始错误。",
	})
	if !errors.Is(err, ErrPersonalDefaultKBNotFound) {
		t.Fatalf("expected ErrPersonalDefaultKBNotFound, got %v", err)
	}
	if creator.calls != 0 {
		t.Fatalf("missing default KB must not create knowledge, calls=%d", creator.calls)
	}
}

func TestPushKnowledgeIdempotencyKeyReturnsExistingKnowledge(t *testing.T) {
	store := &fakeIntakeStore{
		defaultKB:  DefaultKB{TenantID: 70, KBID: "kb-personal"},
		existingID: "knowledge-existing",
	}
	creator := &fakeManualKnowledgeCreator{}
	svc := &Service{store: store, knowledge: creator}

	got, err := svc.PushKnowledge(context.Background(), PushKnowledgeInput{
		UserID:         "u-test",
		Title:          "排查记录",
		Content:        "先看日志，再看指标。",
		IdempotencyKey: "idem-1",
	})
	if err != nil {
		t.Fatalf("PushKnowledge returned error: %v", err)
	}
	if got.KnowledgeID != "knowledge-existing" || got.Status != "existing" {
		t.Fatalf("expected existing result, got %+v", got)
	}
	if store.findKey != "idem-1" {
		t.Fatalf("expected idempotency lookup, got %q", store.findKey)
	}
	if creator.calls != 0 || store.saveCallCount != 0 {
		t.Fatalf("idempotency hit must not create or save, create=%d save=%d", creator.calls, store.saveCallCount)
	}
}
