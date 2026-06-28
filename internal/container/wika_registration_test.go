package container

import (
	"os"
	"strings"
	"testing"
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
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("expected container registration %q", fragment)
		}
	}
}
