package container

import (
	"os"
	"strings"
	"testing"

	wikascope "github.com/Tencent/WeKnora/internal/wika/scope"
	"go.uber.org/dig"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestContainerRegistersWikaSuggestionComponents(t *testing.T) {
	raw, err := os.ReadFile("container.go")
	if err != nil {
		t.Fatalf("read container.go: %v", err)
	}
	source := string(raw)
	required := []string{
		"wikasuggestion.NewGormStore",
		"wikasuggestion.NewService",
		"handler.NewWikaSuggestionHandler",
		"wikaeval.NewGormStore",
		"wikaeval.NewService",
		"handler.NewWikaEvaluationHandler",
		"wikafreshness.NewGormStore",
		"wikafreshness.NewService",
		"handler.NewWikaFreshnessHandler",
		"wikaconflict.NewGormStore",
		"wikaconflict.NewNoopCandidateGenerator",
		"wikaconflict.NewService",
		"handler.NewWikaConflictHandler",
		"wikaversion.NewGormStore",
		"wikaversion.NewService",
		"handler.NewWikaVersionHandler",
		"wikaurlrefresh.NewGormStore",
		"initWikaURLRefreshService",
		"handler.NewWikaURLRefreshHandler",
		"wikaevalschedule.NewGormStore",
		"initWikaEvalScheduleService",
		"handler.NewWikaEvalScheduleHandler",
		"wikaorgshare.NewGormStore",
		"wikaorgshare.NewTenantMemberAdminChecker",
		"initWikaOrgShareService",
		"wikaorgshare.WithAuditLogger",
		"handler.NewWikaOrgShareHandler",
		"wikascope.NewGormStore",
		"initWikaScopeResolver",
		"initWikaOrgShareFeatureGate",
		"wikagraph.NewGormStore",
		"wikagraph.NewService",
		"handler.NewWikaGraphHandler",
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("expected container registration %q", fragment)
		}
	}
}

func TestContainerCanResolveWikaScopeResolver(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	c := dig.New()
	if err := c.Provide(func() *gorm.DB { return db }); err != nil {
		t.Fatalf("provide db: %v", err)
	}
	if err := c.Provide(wikascope.NewGormStore); err != nil {
		t.Fatalf("provide wika scope store: %v", err)
	}
	if err := c.Provide(initWikaScopeResolver); err != nil {
		t.Fatalf("provide wika scope resolver: %v", err)
	}
	var resolved *wikascope.Resolver
	if err := c.Invoke(func(r *wikascope.Resolver) { resolved = r }); err != nil {
		t.Fatalf("resolve wika scope resolver: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected wika scope resolver")
	}
}
