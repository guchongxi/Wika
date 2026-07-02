package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	wikaversion "github.com/Tencent/WeKnora/internal/wika/governance/version"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

type createKnowledgeFileRepoStub struct {
	interfaces.KnowledgeRepository

	createCalls      int
	createErr        error
	createdKnowledge *types.Knowledge
}

func (r *createKnowledgeFileRepoStub) CheckKnowledgeExists(
	ctx context.Context,
	tenantID uint64,
	kbID string,
	params *types.KnowledgeCheckParams,
) (bool, *types.Knowledge, error) {
	return false, nil, nil
}

func (r *createKnowledgeFileRepoStub) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) error {
	r.createCalls++
	copied := *knowledge
	r.createdKnowledge = &copied
	return r.createErr
}

type createKnowledgeFileKBServiceStub struct {
	interfaces.KnowledgeBaseService

	kb *types.KnowledgeBase
}

func (s *createKnowledgeFileKBServiceStub) GetKnowledgeBaseByID(
	ctx context.Context,
	id string,
) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

type createKnowledgeFileServiceStub struct {
	saveErr              error
	saveCalls            int
	savedWithKnowledgeID string
	deleteCalls          int
	deletedPath          string
}

func (s *createKnowledgeFileServiceStub) CheckConnectivity(ctx context.Context) error {
	return nil
}

func (s *createKnowledgeFileServiceStub) SaveFile(
	ctx context.Context,
	file *multipart.FileHeader,
	tenantID uint64,
	knowledgeID string,
) (string, error) {
	s.saveCalls++
	s.savedWithKnowledgeID = knowledgeID
	if s.saveErr != nil {
		return "", s.saveErr
	}
	return "stored/" + knowledgeID, nil
}

func (s *createKnowledgeFileServiceStub) SaveBytes(
	ctx context.Context,
	data []byte,
	tenantID uint64,
	fileName string,
	temp bool,
) (string, error) {
	return "", errors.New("not implemented")
}

func (s *createKnowledgeFileServiceStub) GetFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *createKnowledgeFileServiceStub) GetFileURL(ctx context.Context, filePath string) (string, error) {
	return "", errors.New("not implemented")
}

func (s *createKnowledgeFileServiceStub) DeleteFile(ctx context.Context, filePath string) error {
	s.deleteCalls++
	s.deletedPath = filePath
	return nil
}

func (s *createKnowledgeFileServiceStub) CopyFile(ctx context.Context, srcPath string, tenantID uint64, knowledgeID string) (string, error) {
	return "", errors.New("not implemented")
}

type createKnowledgeTaskEnqueuerStub struct {
	calls int
}

func (s *createKnowledgeTaskEnqueuerStub) Enqueue(
	task *asynq.Task,
	opts ...asynq.Option,
) (*asynq.TaskInfo, error) {
	s.calls++
	return &asynq.TaskInfo{ID: "task-1", Queue: "default"}, nil
}

type manualKnowledgeRepoStub struct {
	interfaces.KnowledgeRepository

	existing      *types.Knowledge
	created       *types.Knowledge
	updated       *types.Knowledge
	currentTags   map[string][]*types.KnowledgeTag
	setTagCalls   int
	setTagIDs     []string
	nextCreatedID string
}

func (r *manualKnowledgeRepoStub) CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) error {
	if knowledge.ID == "" {
		if r.nextCreatedID != "" {
			knowledge.ID = r.nextCreatedID
		} else {
			knowledge.ID = "k-created"
		}
	}
	copied := *knowledge
	r.created = &copied
	return nil
}

func (r *manualKnowledgeRepoStub) SetKnowledgeTags(ctx context.Context, knowledgeID string, tagIDs []string) error {
	r.setTagCalls++
	r.setTagIDs = append([]string(nil), tagIDs...)
	if r.currentTags != nil {
		tags := make([]*types.KnowledgeTag, 0, len(tagIDs))
		for _, tagID := range tagIDs {
			tags = append(tags, &types.KnowledgeTag{ID: tagID, KnowledgeBaseID: r.existing.KnowledgeBaseID})
		}
		r.currentTags[knowledgeID] = tags
	}
	return nil
}

func (r *manualKnowledgeRepoStub) GetKnowledgeTags(ctx context.Context, knowledgeIDs []string) (map[string][]*types.KnowledgeTag, error) {
	result := map[string][]*types.KnowledgeTag{}
	for _, id := range knowledgeIDs {
		result[id] = append([]*types.KnowledgeTag(nil), r.currentTags[id]...)
	}
	return result, nil
}

func (r *manualKnowledgeRepoStub) GetKnowledgeByID(ctx context.Context, tenantID uint64, id string) (*types.Knowledge, error) {
	if r.existing == nil {
		return nil, errors.New("knowledge not found")
	}
	copied := *r.existing
	return &copied, nil
}

func (r *manualKnowledgeRepoStub) UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) error {
	copied := *knowledge
	r.updated = &copied
	return nil
}

type manualTagRepoStub struct {
	interfaces.KnowledgeTagRepository

	tags map[string]*types.KnowledgeTag
}

func (r *manualTagRepoStub) GetByID(ctx context.Context, tenantID uint64, id string) (*types.KnowledgeTag, error) {
	if tag, ok := r.tags[id]; ok {
		return tag, nil
	}
	return nil, errors.New("tag not found")
}

type manualVersionRecorderStub struct {
	records []wikaversion.RecordVersionInput
}

func (r *manualVersionRecorderStub) RecordVersion(ctx context.Context, input wikaversion.RecordVersionInput) (*types.WikaKnowledgeVersion, error) {
	r.records = append(r.records, input)
	return &types.WikaKnowledgeVersion{
		ID:          uint64(len(r.records)),
		KnowledgeID: input.KnowledgeID,
		TenantID:    input.TenantID,
		KBID:        input.KBID,
		VersionNo:   len(r.records),
	}, nil
}

func TestCreateKnowledgeFromFileDoesNotPersistWhenStorageSaveFails(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	fileSvc := &createKnowledgeFileServiceStub{saveErr: errors.New("storage unavailable")}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		nil,
		"",
		nil,
	)

	require.Error(t, err)
	require.Nil(t, knowledge)
	require.Equal(t, 1, fileSvc.saveCalls)
	require.Zero(t, repo.createCalls)
}

func TestCreateKnowledgeFromFilePersistsStoredFilePathOnCreate(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	fileSvc := &createKnowledgeFileServiceStub{}
	task := &createKnowledgeTaskEnqueuerStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
		task:      task,
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		nil,
		"",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, knowledge)
	require.Equal(t, 1, fileSvc.saveCalls)
	require.NotEmpty(t, fileSvc.savedWithKnowledgeID)
	require.Equal(t, fileSvc.savedWithKnowledgeID, knowledge.ID)
	require.Equal(t, 1, repo.createCalls)
	require.NotNil(t, repo.createdKnowledge)
	require.Equal(t, "stored/"+knowledge.ID, repo.createdKnowledge.FilePath)
	require.Equal(t, 1, task.calls)
}

func TestCreateKnowledgeFromFileDeletesStoredFileWhenCreateFails(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{createErr: errors.New("database unavailable")}
	fileSvc := &createKnowledgeFileServiceStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		nil,
		nil,
		"",
		nil,
		"",
		nil,
	)

	require.EqualError(t, err, "database unavailable")
	require.Nil(t, knowledge)
	require.Equal(t, 1, fileSvc.saveCalls)
	require.Equal(t, 1, repo.createCalls)
	require.Equal(t, 1, fileSvc.deleteCalls)
	require.Equal(t, "stored/"+fileSvc.savedWithKnowledgeID, fileSvc.deletedPath)
}

func TestCreateKnowledgeFromFile_PersistsProcessOverrides(t *testing.T) {
	t.Parallel()

	repo := &createKnowledgeFileRepoStub{}
	fileSvc := &createKnowledgeFileServiceStub{}
	task := &createKnowledgeTaskEnqueuerStub{}
	svc := &knowledgeService{
		repo:      repo,
		kbService: &createKnowledgeFileKBServiceStub{kb: &types.KnowledgeBase{ID: "kb-1"}},
		fileSvc:   fileSvc,
		task:      task,
	}

	chunkSize := 512
	overrides := &types.KnowledgeProcessOverrides{
		ChunkingConfig: &types.ChunkingConfig{ChunkSize: chunkSize},
	}

	knowledge, err := svc.CreateKnowledgeFromFile(
		newCreateKnowledgeFileContext(),
		"kb-1",
		newMultipartFileHeader(t, "doc.txt", "hello"),
		map[string]string{"source": "test"},
		nil,
		"",
		nil,
		"",
		overrides,
	)

	require.NoError(t, err)
	require.NotNil(t, knowledge)
	require.Equal(t, 1, repo.createCalls)
	require.NotNil(t, repo.createdKnowledge)

	parsed, err := repo.createdKnowledge.ProcessOverrides()
	require.NoError(t, err)
	require.NotNil(t, parsed)
	require.NotNil(t, parsed.ChunkingConfig)
	require.Equal(t, chunkSize, parsed.ChunkingConfig.ChunkSize)

	metadataMap, err := repo.createdKnowledge.Metadata.Map()
	require.NoError(t, err)
	require.Equal(t, "test", metadataMap["source"])
}

func TestCreateKnowledgeFromManualRecordsInitialVersion(t *testing.T) {
	t.Parallel()

	recorder := &manualVersionRecorderStub{}
	repo := &manualKnowledgeRepoStub{nextCreatedID: "k-manual"}
	kb := &types.KnowledgeBase{ID: "kb-1", EmbeddingModelID: "embedding-1"}
	kb.SetStorageProvider("local")
	svc := &knowledgeService{
		repo:            repo,
		kbService:       &createKnowledgeFileKBServiceStub{kb: kb},
		versionRecorder: recorder,
	}

	knowledge, err := svc.CreateKnowledgeFromManual(
		newCreateKnowledgeFileContext(),
		"kb-1",
		&types.ManualKnowledgePayload{
			Title:   "手工知识",
			Content: "第一版内容",
			Status:  types.ManualKnowledgeStatusDraft,
			TagIDs:  []string{"tag-a"},
		},
		types.ChannelWeb,
	)

	require.NoError(t, err)
	require.Equal(t, "k-manual", knowledge.ID)
	require.Len(t, recorder.records, 1)
	require.Equal(t, "k-manual", recorder.records[0].KnowledgeID)
	require.Equal(t, uint64(1), recorder.records[0].TenantID)
	require.Equal(t, "kb-1", recorder.records[0].KBID)
	require.Equal(t, "手工知识", recorder.records[0].Title)
	require.Equal(t, "第一版内容", recorder.records[0].Content)
	require.Equal(t, types.JSON([]byte(`["tag-a"]`)), recorder.records[0].Tags)
	require.Equal(t, types.ManualKnowledgeStatusDraft, recorder.records[0].Status)
	require.Equal(t, "manual_create", recorder.records[0].ChangeReason)
	require.Equal(t, "u-owner", recorder.records[0].ActorID)
	require.NotEmpty(t, recorder.records[0].Metadata)
}

func TestCreateKnowledgeFromManualCanSkipAutomaticVersionRecord(t *testing.T) {
	t.Parallel()

	recorder := &manualVersionRecorderStub{}
	repo := &manualKnowledgeRepoStub{nextCreatedID: "k-manual"}
	kb := &types.KnowledgeBase{ID: "kb-1", EmbeddingModelID: "embedding-1"}
	kb.SetStorageProvider("local")
	svc := &knowledgeService{
		repo:            repo,
		kbService:       &createKnowledgeFileKBServiceStub{kb: kb},
		versionRecorder: recorder,
	}

	knowledge, err := svc.CreateKnowledgeFromManual(
		newCreateKnowledgeFileContext(),
		"kb-1",
		&types.ManualKnowledgePayload{
			Title:             "治理写入",
			Content:           "由 suggestion apply 统一记录版本",
			Status:            types.ManualKnowledgeStatusDraft,
			SkipVersionRecord: true,
		},
		types.ChannelAPI,
	)

	require.NoError(t, err)
	require.Equal(t, "k-manual", knowledge.ID)
	require.Empty(t, recorder.records)
}

func TestUpdateManualKnowledgeRecordsBaselineAndNewVersion(t *testing.T) {
	t.Parallel()

	old := &types.Knowledge{
		ID:              "k-manual",
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Title:           "旧标题",
		Type:            types.KnowledgeTypeManual,
		FileType:        types.KnowledgeTypeManual,
		EnableStatus:    types.ManualKnowledgeStatusDraft,
		ParseStatus:     types.ManualKnowledgeStatusDraft,
	}
	require.NoError(t, old.SetManualMetadata(types.NewManualKnowledgeMetadata("旧内容", types.ManualKnowledgeStatusDraft, 1)))

	recorder := &manualVersionRecorderStub{}
	kb := &types.KnowledgeBase{ID: "kb-1", EmbeddingModelID: "embedding-1"}
	kb.SetStorageProvider("local")
	svc := &knowledgeService{
		repo:            &manualKnowledgeRepoStub{existing: old},
		kbService:       &createKnowledgeFileKBServiceStub{kb: kb},
		versionRecorder: recorder,
	}

	_, err := svc.UpdateManualKnowledge(
		newCreateKnowledgeFileContext(),
		"k-manual",
		&types.ManualKnowledgePayload{
			Title:   "新标题",
			Content: "新内容",
			Status:  types.ManualKnowledgeStatusDraft,
		},
	)

	require.NoError(t, err)
	require.Len(t, recorder.records, 2)
	require.Equal(t, "旧标题", recorder.records[0].Title)
	require.Equal(t, "旧内容", recorder.records[0].Content)
	require.Equal(t, "baseline", recorder.records[0].ChangeReason)
	require.Equal(t, "新标题", recorder.records[1].Title)
	require.Equal(t, "新内容", recorder.records[1].Content)
	require.Equal(t, "manual_update", recorder.records[1].ChangeReason)
}

func TestUpdateKnowledgeRecordsLegacyAPIUpdateVersion(t *testing.T) {
	t.Parallel()

	old := &types.Knowledge{
		ID:              "k-manual",
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Title:           "旧标题",
		Description:     "旧描述",
		Type:            types.KnowledgeTypeManual,
		FileType:        types.KnowledgeTypeManual,
		EnableStatus:    types.ManualKnowledgeStatusDraft,
		ParseStatus:     types.ManualKnowledgeStatusDraft,
	}
	require.NoError(t, old.SetManualMetadata(types.NewManualKnowledgeMetadata("旧内容", types.ManualKnowledgeStatusDraft, 1)))

	recorder := &manualVersionRecorderStub{}
	repo := &manualKnowledgeRepoStub{existing: old}
	svc := &knowledgeService{repo: repo, versionRecorder: recorder}

	err := svc.UpdateKnowledge(newCreateKnowledgeFileContext(), &types.Knowledge{
		ID:          "k-manual",
		Title:       "新标题",
		Description: "新描述",
	})

	require.NoError(t, err)
	require.NotNil(t, repo.updated)
	require.Equal(t, "新标题", repo.updated.Title)
	require.Len(t, recorder.records, 2)
	require.Equal(t, "旧标题", recorder.records[0].Title)
	require.Equal(t, "旧内容", recorder.records[0].Content)
	require.Equal(t, "baseline", recorder.records[0].ChangeReason)
	require.Equal(t, "新标题", recorder.records[1].Title)
	require.Equal(t, "旧内容", recorder.records[1].Content)
	require.Equal(t, types.ManualKnowledgeStatusDraft, recorder.records[1].Status)
	require.Equal(t, "legacy_api_update", recorder.records[1].ChangeReason)
	require.Equal(t, "u-owner", recorder.records[1].ActorID)
}

func TestUpdateKnowledgeTagRecordsTagUpdateVersion(t *testing.T) {
	t.Parallel()

	old := &types.Knowledge{
		ID:              "k-manual",
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Title:           "知识标题",
		Type:            types.KnowledgeTypeManual,
		FileType:        types.KnowledgeTypeManual,
		EnableStatus:    types.ManualKnowledgeStatusDraft,
		ParseStatus:     types.ManualKnowledgeStatusDraft,
	}
	require.NoError(t, old.SetManualMetadata(types.NewManualKnowledgeMetadata("内容", types.ManualKnowledgeStatusDraft, 1)))

	recorder := &manualVersionRecorderStub{}
	repo := &manualKnowledgeRepoStub{
		existing: old,
		currentTags: map[string][]*types.KnowledgeTag{
			"k-manual": []*types.KnowledgeTag{{ID: "tag-old", KnowledgeBaseID: "kb-1"}},
		},
	}
	tagRepo := &manualTagRepoStub{tags: map[string]*types.KnowledgeTag{
		"tag-new": {ID: "tag-new", KnowledgeBaseID: "kb-1", TenantID: 1},
	}}
	svc := &knowledgeService{repo: repo, tagRepo: tagRepo, versionRecorder: recorder}

	err := svc.UpdateKnowledgeTag(newCreateKnowledgeFileContext(), "k-manual", []string{"tag-new"})

	require.NoError(t, err)
	require.Equal(t, []string{"tag-new"}, repo.setTagIDs)
	require.Len(t, recorder.records, 2)
	require.Equal(t, types.JSON([]byte(`["tag-old"]`)), recorder.records[0].Tags)
	require.Equal(t, "baseline", recorder.records[0].ChangeReason)
	require.Equal(t, types.JSON([]byte(`["tag-new"]`)), recorder.records[1].Tags)
	require.Equal(t, "tag_update", recorder.records[1].ChangeReason)
	require.Equal(t, "u-owner", recorder.records[1].ActorID)
}

func TestUpdateManualKnowledgeCanSkipAutomaticVersionRecord(t *testing.T) {
	t.Parallel()

	old := &types.Knowledge{
		ID:              "k-manual",
		TenantID:        1,
		KnowledgeBaseID: "kb-1",
		Title:           "旧标题",
		Type:            types.KnowledgeTypeManual,
		FileType:        types.KnowledgeTypeManual,
		EnableStatus:    types.ManualKnowledgeStatusDraft,
		ParseStatus:     types.ManualKnowledgeStatusDraft,
	}
	require.NoError(t, old.SetManualMetadata(types.NewManualKnowledgeMetadata("旧内容", types.ManualKnowledgeStatusDraft, 1)))

	recorder := &manualVersionRecorderStub{}
	kb := &types.KnowledgeBase{ID: "kb-1", EmbeddingModelID: "embedding-1"}
	kb.SetStorageProvider("local")
	svc := &knowledgeService{
		repo:            &manualKnowledgeRepoStub{existing: old},
		kbService:       &createKnowledgeFileKBServiceStub{kb: kb},
		versionRecorder: recorder,
	}

	_, err := svc.UpdateManualKnowledge(
		newCreateKnowledgeFileContext(),
		"k-manual",
		&types.ManualKnowledgePayload{
			Title:             "URL 抓取更新",
			Content:           "URL 抓取内容",
			Status:            types.ManualKnowledgeStatusDraft,
			SkipVersionRecord: true,
		},
	)

	require.NoError(t, err)
	require.Empty(t, recorder.records)
}

func newCreateKnowledgeFileContext() context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(1))
	ctx = context.WithValue(ctx, types.TenantInfoContextKey, &types.Tenant{})
	ctx = context.WithValue(ctx, types.UserIDContextKey, "u-owner")
	return ctx
}

func newMultipartFileHeader(t *testing.T, filename string, content string) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, req.ParseMultipartForm(1024))
	return req.MultipartForm.File["file"][0]
}
