package version

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

type fakeVersionStore struct {
	input RecordVersionInput
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
