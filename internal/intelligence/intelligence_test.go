package intelligence_test

import (
	"context"
	"testing"

	"github.com/RenlySir/kb-tool/internal/intelligence"
)

func TestFuncAdaptersExposeExternalIntelligenceBoundaries(t *testing.T) {
	ctx := context.Background()

	parser := intelligence.ParserFunc(func(ctx context.Context, input intelligence.ParseInput) (intelligence.ParseResult, error) {
		if input.Path != "guide.pdf" {
			t.Fatalf("unexpected parse input: %#v", input)
		}
		return intelligence.ParseResult{
			Text:     "TiDB vector search guide",
			Metadata: map[string]string{"author": "docs"},
		}, nil
	})
	parsed, err := parser.Parse(ctx, intelligence.ParseInput{Path: "guide.pdf", MimeType: "application/pdf"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Text != "TiDB vector search guide" || parsed.Metadata["author"] != "docs" {
		t.Fatalf("unexpected parse result: %#v", parsed)
	}

	tagger := intelligence.TagGeneratorFunc(func(ctx context.Context, input intelligence.TagInput) ([]intelligence.TagProposal, error) {
		if input.Title != "guide.pdf" || input.Text == "" {
			t.Fatalf("unexpected tag input: %#v", input)
		}
		return []intelligence.TagProposal{{Name: "tidb", Confidence: 0.91, Evidence: "vector search"}}, nil
	})
	tags, err := tagger.GenerateTags(ctx, intelligence.TagInput{Title: "guide.pdf", Text: parsed.Text})
	if err != nil {
		t.Fatalf("GenerateTags returned error: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "tidb" || tags[0].Confidence <= 0 {
		t.Fatalf("unexpected tags: %#v", tags)
	}

	embedder := intelligence.EmbedderFunc(func(ctx context.Context, input intelligence.EmbedInput) (intelligence.Embedding, error) {
		if input.Model != "bge-m3" || input.Text == "" {
			t.Fatalf("unexpected embed input: %#v", input)
		}
		return intelligence.Embedding{Model: input.Model, Dimensions: 3, Vector: []float32{0.1, 0.2, 0.3}}, nil
	})
	embedding, err := embedder.Embed(ctx, intelligence.EmbedInput{Model: "bge-m3", Text: parsed.Text})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if embedding.Dimensions != 3 || len(embedding.Vector) != 3 {
		t.Fatalf("unexpected embedding: %#v", embedding)
	}
}
