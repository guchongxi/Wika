package router

import (
	"os"
	"strings"
	"testing"
)

func TestWikaOrgShareDirectKnowledgeRoutesUseHandlerLevelAccess(t *testing.T) {
	raw, err := os.ReadFile("router.go")
	if err != nil {
		t.Fatalf("read router.go: %v", err)
	}
	source := string(raw)
	required := []string{
		`k.GET("/:id", g.Viewer(), handler.GetKnowledge)`,
		`k.GET("/:id/download", g.Viewer(), handler.DownloadKnowledgeFile)`,
		`k.GET("/:id/preview", g.Viewer(), handler.PreviewKnowledgeFile)`,
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("expected direct route to use handler-level access: %s", fragment)
		}
	}
	guardedSpans := []string{
		`k.GET("/:id/stages", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledgeSpans)`,
		`k.GET("/:id/spans", g.Viewer(), g.KBAccessReadFromKnowledgeIDParam("id"), handler.GetKnowledgeSpans)`,
	}
	for _, fragment := range guardedSpans {
		if !strings.Contains(source, fragment) {
			t.Fatalf("expected non-direct route to keep KBAccess guard: %s", fragment)
		}
	}
}
