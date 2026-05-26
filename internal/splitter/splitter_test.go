package splitter_test

import (
	"strings"
	"testing"

	"github.com/RenlySir/kb-tool/internal/splitter"
)

func TestRecursiveSplitterKeepsChunksWithinSizeAndOverlap(t *testing.T) {
	s := splitter.NewRecursive(splitter.Options{ChunkSize: 18, Overlap: 5})

	chunks := s.Split("alpha beta gamma delta epsilon")

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %#v", chunks)
	}
	for i, chunk := range chunks {
		if chunk.Index != i {
			t.Fatalf("chunk %d has index %d", i, chunk.Index)
		}
		if runeLen(chunk.Content) > 18 {
			t.Fatalf("chunk exceeds size: %q", chunk.Content)
		}
		if strings.TrimSpace(chunk.Content) == "" {
			t.Fatalf("chunk %d is empty", i)
		}
	}
	if !strings.HasPrefix(chunks[1].Content, suffix(chunks[0].Content, 5)) {
		t.Fatalf("expected second chunk to start with overlap %q, got %q", suffix(chunks[0].Content, 5), chunks[1].Content)
	}
}

func TestRecursiveSplitterUsesDefaultsAndIgnoresEmptyText(t *testing.T) {
	s := splitter.NewRecursive(splitter.Options{})

	if chunks := s.Split(" \n\t "); len(chunks) != 0 {
		t.Fatalf("expected no chunks for empty text, got %#v", chunks)
	}

	chunks := s.Split("short text")
	if len(chunks) != 1 || chunks[0].Content != "short text" || chunks[0].Index != 0 {
		t.Fatalf("unexpected default split result: %#v", chunks)
	}
}

func TestFuncAdapterSplitsText(t *testing.T) {
	custom := splitter.Func(func(text string) []splitter.Chunk {
		return []splitter.Chunk{{Index: 0, Content: strings.ToUpper(text)}}
	})

	chunks := custom.Split("abc")

	if len(chunks) != 1 || chunks[0].Content != "ABC" {
		t.Fatalf("unexpected function split result: %#v", chunks)
	}
}

func runeLen(value string) int {
	return len([]rune(value))
}

func suffix(value string, count int) string {
	runes := []rune(value)
	if len(runes) <= count {
		return value
	}
	return string(runes[len(runes)-count:])
}
