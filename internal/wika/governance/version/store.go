package version

import (
	"context"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store interface {
	RecordVersion(ctx context.Context, input RecordVersionInput) (*types.WikaKnowledgeVersion, error)
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) *GormStore {
	return &GormStore{db: db}
}

func (s *GormStore) RecordVersion(ctx context.Context, input RecordVersionInput) (*types.WikaKnowledgeVersion, error) {
	var created types.WikaKnowledgeVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var last types.WikaKnowledgeVersion
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("knowledge_id = ?", input.KnowledgeID).
			Order("version_no DESC").
			First(&last).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		now := input.Now
		if now.IsZero() {
			now = time.Now()
		}
		created = types.WikaKnowledgeVersion{
			KnowledgeID:  input.KnowledgeID,
			TenantID:     input.TenantID,
			KBID:         input.KBID,
			VersionNo:    last.VersionNo + 1,
			Title:        input.Title,
			Content:      input.Content,
			Tags:         jsonOrDefault(input.Tags, `[]`),
			Status:       input.Status,
			ReviewStatus: input.ReviewStatus,
			Metadata:     jsonOrDefault(input.Metadata, `{}`),
			ContentHash:  input.ContentHash,
			ChangeReason: input.ChangeReason,
			CreatedBy:    input.ActorID,
			CreatedAt:    now,
		}
		return tx.Create(&created).Error
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

func jsonOrDefault(value types.JSON, fallback string) types.JSON {
	if len(value) > 0 {
		return value
	}
	return types.JSON([]byte(fallback))
}
