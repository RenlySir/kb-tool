package intelligence

import "context"

type ParseInput struct {
	Path     string
	MimeType string
	Bytes    []byte
}

type ParseResult struct {
	Text     string
	Metadata map[string]string
}

type Parser interface {
	Parse(ctx context.Context, input ParseInput) (ParseResult, error)
}

type ParserFunc func(ctx context.Context, input ParseInput) (ParseResult, error)

func (f ParserFunc) Parse(ctx context.Context, input ParseInput) (ParseResult, error) {
	return f(ctx, input)
}

type TagInput struct {
	Title    string
	Path     string
	MimeType string
	Text     string
	Metadata map[string]string
}

type TagProposal struct {
	Name       string
	Confidence float64
	Evidence   string
}

type TagGenerator interface {
	GenerateTags(ctx context.Context, input TagInput) ([]TagProposal, error)
}

type TagGeneratorFunc func(ctx context.Context, input TagInput) ([]TagProposal, error)

func (f TagGeneratorFunc) GenerateTags(ctx context.Context, input TagInput) ([]TagProposal, error) {
	return f(ctx, input)
}

type EmbedInput struct {
	Model string
	Text  string
}

type Embedding struct {
	Model      string
	Dimensions int
	Vector     []float32
}

type Embedder interface {
	Embed(ctx context.Context, input EmbedInput) (Embedding, error)
}

type EmbedderFunc func(ctx context.Context, input EmbedInput) (Embedding, error)

func (f EmbedderFunc) Embed(ctx context.Context, input EmbedInput) (Embedding, error) {
	return f(ctx, input)
}
