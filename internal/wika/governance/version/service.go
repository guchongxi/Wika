package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type auditLogger interface {
	Log(ctx context.Context, entry *types.AuditLog) error
}

type knowledgeUpdater interface {
	UpdateManualKnowledge(ctx context.Context, knowledgeID string, payload *types.ManualKnowledgePayload) (*types.Knowledge, error)
}

type featureGate interface {
	GetBool(ctx context.Context, key string, envName string, def bool) bool
}

type Service struct {
	store     Store
	audit     auditLogger
	knowledge knowledgeUpdater
	flags     featureGate
}

const versionFeatureFlagKey = "wika.governance.version.enabled"

func NewService(store *GormStore, audit interfaces.AuditLogService, knowledge interfaces.KnowledgeService, flags interfaces.SystemSettingService) *Service {
	return &Service{store: store, audit: audit, knowledge: knowledge, flags: flags}
}

func (s *Service) RecordVersion(ctx context.Context, input RecordVersionInput) (*types.WikaKnowledgeVersion, error) {
	if s.store == nil {
		return nil, nil
	}
	if strings.TrimSpace(input.ContentHash) == "" {
		input.ContentHash = hashContent(input.Content)
	}
	version, err := s.store.RecordVersion(ctx, input)
	if err != nil {
		return nil, err
	}
	if s.audit != nil && version != nil {
		if err := s.audit.Log(ctx, &types.AuditLog{
			TenantID:    input.TenantID,
			ActorUserID: strings.TrimSpace(input.ActorID),
			Action:      types.AuditActionWikaVersionRecorded,
			TargetType:  "wika_knowledge_version",
			TargetID:    strconv.FormatUint(version.ID, 10),
		}); err != nil {
			return nil, err
		}
	}
	return version, nil
}

func (s *Service) ListVersions(ctx context.Context, input ListVersionsInput) ([]*types.WikaKnowledgeVersion, error) {
	if s.store == nil {
		return nil, nil
	}
	return s.store.ListVersions(ctx, input)
}

func (s *Service) Diff(ctx context.Context, input DiffInput) (*DiffResult, error) {
	if s.store == nil {
		return nil, ErrVersionNotFound
	}
	from, err := s.store.GetVersion(ctx, GetVersionInput{
		ActorID:     input.ActorID,
		TenantID:    input.TenantID,
		KnowledgeID: input.KnowledgeID,
		VersionID:   input.FromVersion,
		SystemAdmin: input.SystemAdmin,
	})
	if err != nil {
		return nil, err
	}
	to, err := s.store.GetVersion(ctx, GetVersionInput{
		ActorID:     input.ActorID,
		TenantID:    input.TenantID,
		KnowledgeID: input.KnowledgeID,
		VersionID:   input.ToVersion,
		SystemAdmin: input.SystemAdmin,
	})
	if err != nil {
		return nil, err
	}
	return &DiffResult{
		FromVersionNo:  from.VersionNo,
		ToVersionNo:    to.VersionNo,
		TitleChanged:   from.Title != to.Title,
		ContentChanged: from.Content != to.Content,
		TagsChanged:    string(from.Tags) != string(to.Tags),
	}, nil
}

func (s *Service) Restore(ctx context.Context, input RestoreInput) (*RestoreResult, error) {
	if !s.versionFeatureEnabled(ctx) {
		return nil, ErrFeatureDisabled
	}
	if s.store == nil {
		return nil, ErrVersionNotFound
	}
	if s.knowledge == nil {
		return nil, errors.New("knowledge updater unavailable")
	}
	version, err := s.store.GetVersion(ctx, GetVersionInput{
		ActorID:     input.ActorID,
		TenantID:    input.TenantID,
		KnowledgeID: input.KnowledgeID,
		VersionID:   input.VersionID,
		SystemAdmin: input.SystemAdmin,
	})
	if err != nil {
		return nil, err
	}
	updated, err := s.knowledge.UpdateManualKnowledge(ctx, input.KnowledgeID, &types.ManualKnowledgePayload{
		Title:   version.Title,
		Content: version.Content,
		Status:  version.Status,
		Channel: types.ChannelWeb,
	})
	if err != nil {
		return nil, err
	}
	recorded, err := s.RecordVersion(ctx, RecordVersionInput{
		KnowledgeID:  input.KnowledgeID,
		TenantID:     version.TenantID,
		KBID:         version.KBID,
		Title:        version.Title,
		Content:      version.Content,
		Tags:         version.Tags,
		Status:       version.Status,
		ReviewStatus: version.ReviewStatus,
		Metadata:     version.Metadata,
		ChangeReason: "restore",
		ActorID:      input.ActorID,
	})
	if err != nil {
		return nil, err
	}
	if s.audit != nil && recorded != nil {
		if err := s.audit.Log(ctx, &types.AuditLog{
			TenantID:    version.TenantID,
			ActorUserID: strings.TrimSpace(input.ActorID),
			Action:      types.AuditActionWikaVersionRestored,
			TargetType:  "knowledge",
			TargetID:    input.KnowledgeID,
		}); err != nil {
			return nil, err
		}
	}
	status := version.Status
	if updated != nil && strings.TrimSpace(updated.EnableStatus) != "" {
		status = updated.EnableStatus
	}
	return &RestoreResult{
		RestoredFromVersionID: version.ID,
		NewVersionID:          recorded.ID,
		KnowledgeID:           input.KnowledgeID,
		Status:                status,
	}, nil
}

func (s *Service) versionFeatureEnabled(ctx context.Context) bool {
	if s.flags == nil {
		return false
	}
	return s.flags.GetBool(ctx, versionFeatureFlagKey, "", false)
}

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
