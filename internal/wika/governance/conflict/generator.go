package conflict

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

const maxDeterministicDuplicateCandidates = 100

type deterministicCandidateGenerator struct {
	db *gorm.DB
}

func NewDeterministicCandidateGenerator(db *gorm.DB) CandidateGenerator {
	return &deterministicCandidateGenerator{db: db}
}

func (g *deterministicCandidateGenerator) GenerateCandidates(ctx context.Context, input GenerateInput) ([]Candidate, error) {
	if g == nil || g.db == nil || input.TenantID == 0 || strings.TrimSpace(input.KBID) == "" {
		return nil, nil
	}
	var knowledges []*types.Knowledge
	if err := g.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ? AND type = ?",
			input.TenantID,
			strings.TrimSpace(input.KBID),
			types.KnowledgeTypeManual,
		).
		Order("id ASC").
		Find(&knowledges).Error; err != nil {
		return nil, err
	}
	return duplicateCandidatesFromManualKnowledges(knowledges), nil
}

type duplicateKnowledgeFingerprint struct {
	knowledge *types.Knowledge
	hash      string
}

func duplicateCandidatesFromManualKnowledges(knowledges []*types.Knowledge) []Candidate {
	byHash := make(map[string][]duplicateKnowledgeFingerprint)
	for _, knowledge := range knowledges {
		if knowledge == nil {
			continue
		}
		meta, err := knowledge.ManualMetadata()
		if err != nil || meta == nil || meta.Status != types.ManualKnowledgeStatusPublish {
			continue
		}
		normalized := normalizeKnowledgeContentForConflict(meta.Content)
		if len([]rune(normalized)) < 16 {
			continue
		}
		hash := hashNormalizedKnowledgeContent(normalized)
		byHash[hash] = append(byHash[hash], duplicateKnowledgeFingerprint{knowledge: knowledge, hash: hash})
	}

	candidates := make([]Candidate, 0)
	for _, group := range byHash {
		if len(group) < 2 {
			continue
		}
		for i := 0; i < len(group)-1; i++ {
			for j := i + 1; j < len(group); j++ {
				candidates = append(candidates, buildDuplicateCandidate(group[i], group[j]))
				if len(candidates) >= maxDeterministicDuplicateCandidates {
					return candidates
				}
			}
		}
	}
	return candidates
}

func buildDuplicateCandidate(left, right duplicateKnowledgeFingerprint) Candidate {
	sourceID, targetID := canonicalKnowledgePair(left.knowledge.ID, right.knowledge.ID)
	evidence := map[string]any{
		"detector":        "deterministic_content_hash",
		"normalized_hash": left.hash,
	}
	rawEvidence, _ := json.Marshal(evidence)
	return Candidate{
		SourceKnowledgeID: sourceID,
		TargetKnowledgeID: targetID,
		ConflictType:      ConflictTypeDuplicate,
		ConfidenceScore:   1,
		Evidence:          types.JSON(rawEvidence),
		AIExplanation:     "检测到两条已发布手工知识的规范化正文一致，请确认是否合并或保留一条。",
	}
}

func normalizeKnowledgeContentForConflict(content string) string {
	fields := strings.Fields(strings.TrimSpace(content))
	return strings.ToLower(strings.Join(fields, " "))
}

func hashNormalizedKnowledgeContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
