package types

import "time"

// WikaGraphEntity 保存知识图谱实体读模型。
type WikaGraphEntity struct {
	ID                 uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID           uint64     `json:"tenant_id" gorm:"not null;index:idx_wika_graph_entities_type_name;uniqueIndex:ux_wika_graph_entities_key"`
	KBID               string     `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_wika_graph_entities_type_name;uniqueIndex:ux_wika_graph_entities_key"`
	EntityKey          string     `json:"entity_key" gorm:"type:varchar(255);not null;uniqueIndex:ux_wika_graph_entities_key"`
	Name               string     `json:"name" gorm:"type:varchar(255);not null;index:idx_wika_graph_entities_type_name"`
	EntityType         string     `json:"entity_type" gorm:"type:varchar(64);not null;index:idx_wika_graph_entities_type_name"`
	Summary            string     `json:"summary,omitempty" gorm:"type:text"`
	SourceKnowledgeIDs JSON       `json:"source_knowledge_ids" gorm:"type:jsonb;not null;default:'[]'"`
	ConfidenceScore    float64    `json:"confidence_score" gorm:"type:numeric(5,4);not null;default:0"`
	LastExtractedAt    *time.Time `json:"last_extracted_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (WikaGraphEntity) TableName() string {
	return "wika_graph_entities"
}

// WikaGraphEdge 保存知识图谱关系读模型。
type WikaGraphEdge struct {
	ID                  uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	TenantID            uint64    `json:"tenant_id" gorm:"not null;index:idx_wika_graph_edges_source;index:idx_wika_graph_edges_target"`
	KBID                string    `json:"kb_id" gorm:"type:varchar(36);not null;index:idx_wika_graph_edges_source;index:idx_wika_graph_edges_target"`
	SourceEntityID      uint64    `json:"source_entity_id" gorm:"not null;index:idx_wika_graph_edges_source"`
	TargetEntityID      uint64    `json:"target_entity_id" gorm:"not null;index:idx_wika_graph_edges_target"`
	RelationType        string    `json:"relation_type" gorm:"type:varchar(64);not null"`
	EvidenceKnowledgeID string    `json:"evidence_knowledge_id,omitempty" gorm:"type:varchar(36);index:idx_wika_graph_edges_evidence_knowledge"`
	EvidenceChunkID     string    `json:"evidence_chunk_id,omitempty" gorm:"type:varchar(36)"`
	EvidenceText        string    `json:"evidence_text,omitempty" gorm:"type:text"`
	ConfidenceScore     float64   `json:"confidence_score" gorm:"type:numeric(5,4);not null;default:0"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (WikaGraphEdge) TableName() string {
	return "wika_graph_edges"
}
