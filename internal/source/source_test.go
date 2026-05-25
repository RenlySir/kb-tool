package source_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RenlySir/kb-tool/internal/source"
)

func TestFileCollectorCollectsTextFilesAndSkipsNoise(t *testing.T) {
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "README.md"), "# KB Tool\nTiDB ingestion")
	writeFile(t, filepath.Join(root, "src", "main.go"), "package main\nfunc main() {}")
	writeFile(t, filepath.Join(root, "node_modules", "dep.js"), "ignored dependency")
	writeFile(t, filepath.Join(root, ".git", "config"), "ignored git metadata")
	writeFile(t, filepath.Join(root, "image.png"), string([]byte{0x89, 0x50, 0x4e, 0x47, 0x00}))

	collector := source.NewFileCollector(source.FileCollectorOptions{MaxBytes: 1 << 20})
	docs, err := collector.Collect(root)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected 2 text documents, got %d: %#v", len(docs), docs)
	}

	byName := map[string]source.Document{}
	for _, doc := range docs {
		byName[filepath.Base(doc.Path)] = doc
		if doc.SourceType != source.SourceTypeFile {
			t.Fatalf("expected source type %q, got %q", source.SourceTypeFile, doc.SourceType)
		}
		if doc.ContentHash == "" {
			t.Fatalf("expected content hash for %s", doc.Path)
		}
	}

	if byName["README.md"].Title != "README.md" {
		t.Fatalf("expected README title, got %q", byName["README.md"].Title)
	}
	if byName["main.go"].Language != "go" {
		t.Fatalf("expected go language, got %q", byName["main.go"].Language)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
