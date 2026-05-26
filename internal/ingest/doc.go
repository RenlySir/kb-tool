package ingest

import (
	"context"

	"github.com/RenlySir/kb-tool/internal/source"
	"github.com/RenlySir/kb-tool/internal/splitter"
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
	Tags   []string
	Chunks []splitter.Chunk
}

type Result struct {
	Documents int
	Tags      int
	Chunks    int
}

type Service struct {
	collector source.Collector
	tagger    Tagger
	splitter  splitter.Splitter
	store     Store
}

type Option func(*Service)

func WithSplitter(textSplitter splitter.Splitter) Option {
	return func(s *Service) {
		if textSplitter != nil {
			s.splitter = textSplitter
		}
	}
}

func NewService(collector source.Collector, tagger Tagger, store Store, options ...Option) *Service {
	service := &Service{
		collector: collector,
		tagger:    tagger,
		splitter:  splitter.NewRecursive(splitter.Options{}),
		store:     store,
	}
	for _, option := range options {
		option(service)
	}
	return service
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
		chunks := s.splitter.Split(doc.Content)
		if err := s.store.Save(ctx, TaggedDocument{Document: doc, Tags: tags, Chunks: chunks}); err != nil {
			return Result{}, err
		}
		result.Documents++
		result.Tags += len(tags)
		result.Chunks += len(chunks)
	}
	return result, nil
}
