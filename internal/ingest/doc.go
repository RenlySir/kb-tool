package ingest

import (
	"context"

	"github.com/RenlySir/kb-tool/internal/source"
)

type Tagger interface {
	Tag(doc source.Document) []string
}

type TaggerFunc func(doc source.Document) []string

func (f TaggerFunc) Tag(doc source.Document) []string {
	return f(doc)
}

type Store interface {
	Migrate(ctx context.Context) error
	Save(ctx context.Context, doc TaggedDocument) error
}

type TaggedDocument struct {
	source.Document
	Tags []string
}

type Result struct {
	Documents int
	Tags      int
}

type Service struct {
	collector source.Collector
	tagger    Tagger
	store     Store
}

func NewService(collector source.Collector, tagger Tagger, store Store) *Service {
	return &Service{collector: collector, tagger: tagger, store: store}
}

func (s *Service) Ingest(ctx context.Context, input string) (Result, error) {
	if err := s.store.Migrate(ctx); err != nil {
		return Result{}, err
	}
	docs, err := s.collector.Collect(ctx, input)
	if err != nil {
		return Result{}, err
	}

	result := Result{}
	for _, doc := range docs {
		tags := s.tagger.Tag(doc)
		if err := s.store.Save(ctx, TaggedDocument{Document: doc, Tags: tags}); err != nil {
			return Result{}, err
		}
		result.Documents++
		result.Tags += len(tags)
	}
	return result, nil
}
