package version

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeVersionStore struct {
	input    RecordVersionInput
	versions map[uint64]*types.WikaKnowledgeVersion
}

func (s *fakeVersionStore) RecordVersion(ctx context.Context, input RecordVersionInput) (*types.WikaKnowledgeVersion, error) {
	s.input = input
	return &types.WikaKnowledgeVersion{
		ID:          9,
		KnowledgeID: input.KnowledgeID,
		TenantID:    input.TenantID,
		KBID:        input.KBID,
		VersionNo:   1,
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
