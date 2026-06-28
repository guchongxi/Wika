package graph

import "errors"

var ErrGraphEntityNotFound = errors.New("graph entity not found")

// Overview 是单个 KB 的图谱读模型摘要。
type Overview struct {
	TenantID    uint64 `json:"tenant_id"`
	KBID        string `json:"kb_id"`
	EntityCount int64  `json:"entity_count"`
	EdgeCount   int64  `json:"edge_count"`
}

type OverviewInput struct {
	TenantID      uint64
	KBID          string
	SystemAdmin   bool
	PersonalScope bool
}

type ListEntitiesInput struct {
	TenantID      uint64
	KBID          string
	EntityType    string
	Query         string
	Limit         int
	Offset        int
	SystemAdmin   bool
	PersonalScope bool
}

type GetEntityInput struct {
	TenantID      uint64
	KBID          string
	EntityID      uint64
	SystemAdmin   bool
	PersonalScope bool
}

type ListEdgesInput struct {
	TenantID          uint64
	KBID              string
	SourceEntityID    uint64
	TargetEntityID    uint64
	EvidenceKnowledge string
	Limit             int
	Offset            int
	SystemAdmin       bool
	PersonalScope     bool
}
