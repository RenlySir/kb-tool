package ingest_test

import (
	"context"
	"testing"

	"github.com/RenlySir/kb-tool/internal/ingest"
	"github.com/RenlySir/kb-tool/internal/source"
)

func TestServiceMigratesTagsAndStoresCollectedDocuments(t *testing.T) {
	store := &fakeStore{}
	collector := source.CollectorFunc(func(ctx context.Context, input string) ([]source.Document, error) {
		if input != "docs" {
			t.Fatalf("unexpected input: %s", input)
		}
		return []source.Document{{
			SourceType:  source.SourceTypeFile,
			SourceURI:   "docs",
			Path:        "docs/README.md",
			Title:       "README.md",
			Language:    "markdown",
			Content:     "# TiDB knowledge base",
			ContentHash: "abc123",
		}}, nil
	})
	tagger := ingest.TaggerFunc(func(doc source.Document) []string {
		return []string{"tidb", "knowledge-base"}
	})

	service := ingest.NewService(collector, tagger, store)
	result, err := service.Ingest(context.Background(), "docs")
	if err != nil {
		t.Fatalf("Ingest returned error: %v", err)
	}

	if !store.migrated {
		t.Fatal("expected migration before ingest")
	}
	if result.Documents != 1 || result.Tags != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected one saved document, got %d", len(store.saved))
	}
	if store.saved[0].Tags[0] != "tidb" || store.saved[0].Tags[1] != "knowledge-base" {
		t.Fatalf("unexpected saved tags: %v", store.saved[0].Tags)
	}
}

type fakeStore struct {
	migrated bool
	saved    []ingest.TaggedDocument
}

func (s *fakeStore) Migrate(ctx context.Context) error {
	s.migrated = true
	return nil
}

func (s *fakeStore) Save(ctx context.Context, doc ingest.TaggedDocument) error {
	s.saved = append(s.saved, doc)
	return nil
}
