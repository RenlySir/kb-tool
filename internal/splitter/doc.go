package splitter

import "strings"

const (
	defaultChunkSize = 1024
	defaultOverlap   = 128
)

type Chunk struct {
	Index   int    `json:"index"`
	Content string `json:"content"`
}

type Splitter interface {
	Split(text string) []Chunk
}

type Func func(text string) []Chunk

func (f Func) Split(text string) []Chunk {
	return f(text)
}

type Options struct {
	ChunkSize  int
	Overlap    int
	Separators []string
}

type Recursive struct {
	chunkSize  int
	overlap    int
	separators []string
}

func NewRecursive(options Options) Recursive {
	chunkSize := options.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	overlap := options.Overlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}
	separators := options.Separators
	if len(separators) == 0 {
		separators = []string{"\n\n", "\n", "。", ".", " ", ""}
	}
	return Recursive{chunkSize: chunkSize, overlap: overlap, separators: append([]string(nil), separators...)}
}

func (s Recursive) Split(text string) []Chunk {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	parts := s.splitToSize(text, s.separators)
	var chunks []Chunk
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if len(chunks) > 0 && s.overlap > 0 {
			part = suffix(chunks[len(chunks)-1].Content, s.overlap) + part
			if runeLen(part) > s.chunkSize {
				part = trimRunes(part, s.chunkSize)
			}
		}
		chunks = append(chunks, Chunk{Index: len(chunks), Content: part})
	}
	return chunks
}

func (s Recursive) splitToSize(text string, separators []string) []string {
	if runeLen(text) <= s.chunkSize {
		return []string{text}
	}
	if len(separators) == 0 {
		return splitRunes(text, s.chunkSize)
	}

	separator := separators[0]
	remaining := separators[1:]
	if separator == "" {
		return splitRunes(text, s.chunkSize)
	}
	if !strings.Contains(text, separator) {
		return s.splitToSize(text, remaining)
	}

	raw := strings.Split(text, separator)
	var chunks []string
	var current string
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		candidate := item
		if current != "" {
			candidate = current + separator + item
		}
		if runeLen(candidate) <= s.chunkSize {
			current = candidate
			continue
		}
		if current != "" {
			chunks = append(chunks, s.splitToSize(current, remaining)...)
		}
		current = ""
		chunks = append(chunks, s.splitToSize(item, remaining)...)
	}
	if current != "" {
		chunks = append(chunks, s.splitToSize(current, remaining)...)
	}
	return chunks
}

func runeLen(value string) int {
	return len([]rune(value))
}

func splitRunes(text string, size int) []string {
	runes := []rune(text)
	var chunks []string
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks
}

func suffix(value string, count int) string {
	runes := []rune(value)
	if len(runes) <= count {
		return value
	}
	return string(runes[len(runes)-count:])
}

func trimRunes(value string, size int) string {
	runes := []rune(value)
	if len(runes) <= size {
		return value
	}
	return string(runes[:size])
}
