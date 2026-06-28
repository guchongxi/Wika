package version

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeVersionStore struct {
	input    RecordVersionInput
	records  []RecordVersionInput
	versions map[uint64]*types.WikaKnowledgeVersion
	getCalls int
}

func (s *fakeVersionStore) RecordVersion(ctx context.Context, input RecordVersionInput) (*types.WikaKnowledgeVersion, error) {
	s.input = input
	s.records = append(s.records, input)
	return &types.WikaKnowledgeVersion{
		ID:          uint64(8 + len(s.records)),
		KnowledgeID: input.KnowledgeID,
		TenantID:    input.TenantID,
		KBID:        input.KBID,
		VersionNo:   len(s.records),
		Title:       input.Title,
		Content:     input.Content,
		ContentHash: input.ContentHash,
		CreatedBy:   input.ActorID,
	}, nil
}

func (s *fakeVersionStore) ListVersions(ctx context.Context, input ListVersionsInput) ([]*types.WikaKnowledgeVersion, error) {
	result := make([]*types.WikaKnowledgeVersion, 0, len(s.versions))
	for _, item := range s.versions {
		if item.KnowledgeID == input.KnowledgeID && item.TenantID == input.TenantID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *fakeVersionStore) GetVersion(ctx context.Context, input GetVersionInput) (*types.WikaKnowledgeVersion, error) {
	s.getCalls++
	if item, ok := s.versions[input.VersionID]; ok {
		return item, nil
	}
	return nil, ErrVersionNotFound
}

type fakeVersionAudit struct {
	entries []*types.AuditLog
}

func (a *fakeVersionAudit) Log(ctx context.Context, entry *types.AuditLog) error {
	a.entries = append(a.entries, entry)
	return nil
}

type fakeKnowledgeUpdater struct {
	knowledgeID string
	payload     *types.ManualKnowledgePayload
	current     *types.Knowledge
}

func (u *fakeKnowledgeUpdater) GetKnowledgeByID(ctx context.Context, knowledgeID string) (*types.Knowledge, error) {
	if u.current == nil {
		return &types.Knowledge{
			ID:              knowledgeID,
			TenantID:        80,
			KnowledgeBaseID: "kb-team",
			Type:            types.KnowledgeTypeManual,
			Title:           "当前标题",
			EnableStatus:    "publish",
		}, nil
	}
	return u.current, nil
}

func (u *fakeKnowledgeUpdater) UpdateManualKnowledge(ctx context.Context, knowledgeID string, payload *types.ManualKnowledgePayload) (*types.Knowledge, error) {
	u.knowledgeID = knowledgeID
	u.payload = payload
	return &types.Knowledge{
		ID:              knowledgeID,
		TenantID:        80,
		KnowledgeBaseID: "kb-team",
		Title:           payload.Title,
		EnableStatus:    payload.Status,
	}, nil
}

type fakeFeatureGate struct {
	enabled bool
}

func (g fakeFeatureGate) GetBool(ctx context.Context, key string, envName string, def bool) bool {
	return g.enabled
}

func TestVersionServiceRecordVersionComputesHashAndWritesAudit(t *testing.T) {
	store := &fakeVersionStore{}
	audit := &fakeVersionAudit{}
	svc := &Service{store: store, audit: audit}

	got, err := svc.RecordVersion(context.Background(), RecordVersionInput{
		KnowledgeID:  "k-1",
		TenantID:     80,
		KBID:         "kb-team",
		Title:        "手册",
		Content:      "版本内容",
		ChangeReason: "manual_update",
		ActorID:      "u-owner",
	})
	if err != nil {
		t.Fatalf("RecordVersion returned error: %v", err)
	}
	if got.VersionNo != 1 || store.input.ContentHash == "" {
		t.Fatalf("unexpected version result: got=%+v input=%+v", got, store.input)
	}
	if len(audit.entries) != 1 ||
		audit.entries[0].Action != types.AuditActionWikaVersionRecorded ||
		audit.entries[0].TenantID != 80 ||
		audit.entries[0].ActorUserID != "u-owner" ||
		audit.entries[0].TargetID != "9" {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

func TestVersionServiceRestoreUpdatesKnowledgeAndRecordsNewVersion(t *testing.T) {
	store := &fakeVersionStore{versions: map[uint64]*types.WikaKnowledgeVersion{
		7: {
			ID:          7,
			KnowledgeID: "k-1",
			TenantID:    80,
			KBID:        "kb-team",
			VersionNo:   2,
			Title:       "旧标题",
			Content:     "旧内容",
			Status:      "enabled",
			Tags:        types.JSON([]byte(`["tag-a"]`)),
			Metadata:    types.JSON([]byte(`{"source":"manual"}`)),
		},
	}}
	updater := &fakeKnowledgeUpdater{}
	audit := &fakeVersionAudit{}
	svc := &Service{store: store, audit: audit, knowledge: updater, flags: fakeFeatureGate{enabled: true}}

	result, err := svc.Restore(context.Background(), RestoreInput{
		ActorID:     "u-admin",
		TenantID:    80,
		KnowledgeID: "k-1",
		VersionID:   7,
		Reason:      "误操作恢复",
	})
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if updater.knowledgeID != "k-1" || updater.payload == nil ||
		updater.payload.Title != "旧标题" ||
		updater.payload.Content != "旧内容" ||
		updater.payload.Status != types.ManualKnowledgeStatusPublish {
		t.Fatalf("restore did not call knowledge updater with version snapshot: id=%s payload=%+v", updater.knowledgeID, updater.payload)
	}
	if len(store.records) != 1 ||
		store.records[0].KnowledgeID != "k-1" ||
		store.records[0].Title != "旧标题" ||
		store.records[0].Content != "旧内容" ||
		store.records[0].ChangeReason != "restore" ||
		store.records[0].ActorID != "u-admin" {
		t.Fatalf("restore did not record restored version snapshot: %+v", store.records)
	}
	if result.RestoredFromVersionID != 7 || result.NewVersionID == 0 || result.KnowledgeID != "k-1" {
		t.Fatalf("unexpected restore result: %+v", result)
	}
	if len(audit.entries) == 0 || audit.entries[len(audit.entries)-1].Action != types.AuditActionWikaVersionRestored {
		t.Fatalf("expected restore audit event, got %+v", audit.entries)
	}
}

func TestVersionServiceRestoreUsesManualPublishStatusForEnabledSnapshot(t *testing.T) {
	metadata, err := types.NewManualKnowledgeMetadata("旧内容", types.ManualKnowledgeStatusPublish, 2).ToJSON()
	if err != nil {
		t.Fatalf("failed to prepare manual metadata: %v", err)
	}
	store := &fakeVersionStore{versions: map[uint64]*types.WikaKnowledgeVersion{
		7: {
			ID:          7,
			KnowledgeID: "k-1",
			TenantID:    80,
			KBID:        "kb-team",
			VersionNo:   2,
			Title:       "旧标题",
			Content:     "旧内容",
			Status:      "enabled",
			Metadata:    metadata,
		},
	}}
	updater := &fakeKnowledgeUpdater{}
	svc := &Service{store: store, knowledge: updater, flags: fakeFeatureGate{enabled: true}}

	_, err = svc.Restore(context.Background(), RestoreInput{
		ActorID:     "u-admin",
		TenantID:    80,
		KnowledgeID: "k-1",
		VersionID:   7,
		Reason:      "误操作恢复",
	})
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if updater.payload == nil || updater.payload.Status != types.ManualKnowledgeStatusPublish {
		t.Fatalf("expected restore payload status publish, got %+v", updater.payload)
	}
}

func TestVersionServiceRestorePassesVersionTagsToKnowledgeUpdater(t *testing.T) {
	store := &fakeVersionStore{versions: map[uint64]*types.WikaKnowledgeVersion{
		7: {
			ID:          7,
			KnowledgeID: "k-1",
			TenantID:    80,
			KBID:        "kb-team",
			VersionNo:   2,
			Title:       "旧标题",
			Content:     "旧内容",
			Status:      types.ManualKnowledgeStatusPublish,
			Tags:        types.JSON([]byte(`["tag-a","tag-b"]`)),
		},
	}}
	updater := &fakeKnowledgeUpdater{}
	svc := &Service{store: store, knowledge: updater, flags: fakeFeatureGate{enabled: true}}

	_, err := svc.Restore(context.Background(), RestoreInput{
		ActorID:     "u-admin",
		TenantID:    80,
		KnowledgeID: "k-1",
		VersionID:   7,
		Reason:      "恢复标签",
	})
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if updater.payload == nil || len(updater.payload.TagIDs) != 2 ||
		updater.payload.TagIDs[0] != "tag-a" ||
		updater.payload.TagIDs[1] != "tag-b" {
		t.Fatalf("expected tag ids restored from version snapshot, got %+v", updater.payload)
	}
}

func TestVersionServiceRestoreReturnsFeatureDisabledBeforeSideEffects(t *testing.T) {
	store := &fakeVersionStore{versions: map[uint64]*types.WikaKnowledgeVersion{
		7: {
			ID:          7,
			KnowledgeID: "k-1",
			TenantID:    80,
			KBID:        "kb-team",
			VersionNo:   2,
			Title:       "旧标题",
			Content:     "旧内容",
			Status:      "enabled",
		},
	}}
	updater := &fakeKnowledgeUpdater{}
	audit := &fakeVersionAudit{}
	svc := &Service{store: store, audit: audit, knowledge: updater, flags: fakeFeatureGate{enabled: false}}

	_, err := svc.Restore(context.Background(), RestoreInput{
		ActorID:     "u-admin",
		TenantID:    80,
		KnowledgeID: "k-1",
		VersionID:   7,
		Reason:      "误操作恢复",
	})
	if err != ErrFeatureDisabled {
		t.Fatalf("expected ErrFeatureDisabled, got %v", err)
	}
	if store.getCalls != 0 || updater.payload != nil || len(store.records) != 0 || len(audit.entries) != 0 {
		t.Fatalf("feature-disabled restore produced side effects: getCalls=%d payload=%+v records=%+v audit=%+v", store.getCalls, updater.payload, store.records, audit.entries)
	}
}

func TestVersionServiceRestoreReturnsNoopWhenTargetMatchesCurrentKnowledge(t *testing.T) {
	current := &types.Knowledge{
		ID:              "k-1",
		TenantID:        80,
		KnowledgeBaseID: "kb-team",
		Type:            types.KnowledgeTypeManual,
		Title:           "旧标题",
		EnableStatus:    "publish",
	}
	if err := current.SetManualMetadata(types.NewManualKnowledgeMetadata("旧内容", "publish", 3)); err != nil {
		t.Fatalf("failed to prepare manual metadata: %v", err)
	}
	store := &fakeVersionStore{versions: map[uint64]*types.WikaKnowledgeVersion{
		7: {
			ID:          7,
			KnowledgeID: "k-1",
			TenantID:    80,
			KBID:        "kb-team",
			VersionNo:   2,
			Title:       "旧标题",
			Content:     "旧内容",
			Status:      "publish",
		},
	}}
	updater := &fakeKnowledgeUpdater{current: current}
	audit := &fakeVersionAudit{}
	svc := &Service{store: store, audit: audit, knowledge: updater, flags: fakeFeatureGate{enabled: true}}

	result, err := svc.Restore(context.Background(), RestoreInput{
		ActorID:     "u-admin",
		TenantID:    80,
		KnowledgeID: "k-1",
		VersionID:   7,
		Reason:      "重复点击恢复",
	})
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if result == nil || !result.Noop || result.NewVersionID != 0 || result.RestoredFromVersionID != 7 {
		t.Fatalf("expected stable noop result, got %+v", result)
	}
	if updater.payload != nil || len(store.records) != 0 || len(audit.entries) != 0 {
		t.Fatalf("noop restore produced side effects: payload=%+v records=%+v audit=%+v", updater.payload, store.records, audit.entries)
	}
}

func TestVersionServiceDiffComparesSnapshots(t *testing.T) {
	store := &fakeVersionStore{versions: map[uint64]*types.WikaKnowledgeVersion{
		1: {ID: 1, KnowledgeID: "k-1", TenantID: 80, KBID: "kb-team", VersionNo: 1, Title: "旧标题", Content: "旧内容"},
		2: {ID: 2, KnowledgeID: "k-1", TenantID: 80, KBID: "kb-team", VersionNo: 2, Title: "新标题", Content: "新内容"},
	}}
	svc := &Service{store: store}

	diff, err := svc.Diff(context.Background(), DiffInput{
		TenantID:    80,
		KnowledgeID: "k-1",
		FromVersion: 1,
		ToVersion:   2,
	})
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if diff.FromVersionNo != 1 || diff.ToVersionNo != 2 || !diff.TitleChanged || !diff.ContentChanged {
		t.Fatalf("unexpected diff: %+v", diff)
	}
}
