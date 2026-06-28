package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type auditLogger interface {
	Log(ctx context.Context, entry *types.AuditLog) error
}

type Service struct {
	store Store
	audit auditLogger
}

func NewService(store *GormStore, audit interfaces.AuditLogService) *Service {
	return &Service{store: store, audit: audit}
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

func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
