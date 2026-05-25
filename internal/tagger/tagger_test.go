package tagger_test

import (
	"testing"

	"github.com/RenlySir/kb-tool/internal/source"
	"github.com/RenlySir/kb-tool/internal/tagger"
)

func TestRuleBasedTaggerReturnsStableDeduplicatedTags(t *testing.T) {
	doc := source.Document{
		Path:     "/repo/internal/store/tidb.go",
		Title:    "TiDB Store",
		Language: "go",
		Content:  "package store\n// mysql compatible TiDB knowledge base storage with GitHub ingestion",
	}

	tags := tagger.NewRuleBasedTagger().Tag(doc)

	want := []string{"backend", "database", "github", "go", "knowledge-base", "mysql", "tidb"}
	if len(tags) != len(want) {
		t.Fatalf("expected %v, got %v", want, tags)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, tags)
		}
	}
}
