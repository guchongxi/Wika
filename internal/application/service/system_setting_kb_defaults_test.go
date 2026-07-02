package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type memorySystemSettingRepo struct {
	rows map[string]*types.SystemSetting
}

func (r *memorySystemSettingRepo) Get(_ context.Context, key string) (*types.SystemSetting, error) {
	if r.rows == nil {
		return nil, nil
	}
	return r.rows[key], nil
}

func (r *memorySystemSettingRepo) List(_ context.Context) ([]*types.SystemSetting, error) {
	out := make([]*types.SystemSetting, 0, len(r.rows))
	for _, row := range r.rows {
		out = append(out, row)
	}
	return out, nil
}

func (r *memorySystemSettingRepo) Upsert(_ context.Context, row *types.SystemSetting) error {
	if r.rows == nil {
		r.rows = map[string]*types.SystemSetting{}
	}
	row.UpdatedAt = time.Now()
	r.rows[row.Key] = row
	return nil
}

func (r *memorySystemSettingRepo) Delete(_ context.Context, key string) (bool, error) {
	if r.rows == nil {
		return false, nil
	}
	_, existed := r.rows[key]
	delete(r.rows, key)
	return existed, nil
}

func TestSystemSettingUpdatePreservesKBChunkSeparatorWhitespace(t *testing.T) {
	repo := &memorySystemSettingRepo{rows: map[string]*types.SystemSetting{}}
	svc := &systemSettingService{
		repo:       repo,
		instanceID: "test-instance",
		cache:      map[string]*types.SystemSetting{},
	}

	row, err := svc.Update(context.Background(), KBDefaultChunkSeparators, []any{"\n\n", "\n", "。"})
	require.NoError(t, err)

	got, err := row.AsStringList()
	require.NoError(t, err)
	require.Equal(t, []string{"\n\n", "\n", "。"}, got)
}

func TestSystemSettingUpdateAllowsWikaGovernanceFlags(t *testing.T) {
	repo := &memorySystemSettingRepo{rows: map[string]*types.SystemSetting{}}
	svc := &systemSettingService{
		repo:       repo,
		instanceID: "test-instance",
		cache:      map[string]*types.SystemSetting{},
	}

	keys := []string{
		"wika.governance.conflict.enabled",
		"wika.governance.version.enabled",
		"wika.governance.url_refresh.enabled",
		"wika.governance.eval_schedule.enabled",
		"wika.governance.org_share.enabled",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			row, err := svc.Update(context.Background(), key, true)
			require.NoError(t, err)

			got, err := row.AsBool()
			require.NoError(t, err)
			require.True(t, got)
			require.Equal(t, "wika_governance", row.Category)
		})
	}
}
