package source_test

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

func TestFileCollectorCollectsImagesAsAssets(t *testing.T) {
	root := t.TempDir()
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	if err := os.WriteFile(filepath.Join(root, "diagram.png"), png, 0o644); err != nil {
		t.Fatalf("write image: %v", err)
	}

	collector := source.NewFileCollector(source.FileCollectorOptions{MaxBytes: 1 << 20, IncludeBinary: true})
	docs, err := collector.Collect(root)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected one image document, got %d", len(docs))
	}
	doc := docs[0]
	if !doc.IsBinary {
		t.Fatal("expected image to be marked binary")
	}
	if doc.MimeType != "image/png" {
		t.Fatalf("expected image/png, got %q", doc.MimeType)
	}
	if string(doc.Asset) != string(png) {
		t.Fatalf("expected asset bytes to be preserved")
	}
	if doc.Content != "" {
		t.Fatalf("expected binary content to stay empty, got %q", doc.Content)
	}
}

func TestFileCollectorDefaultsToLargeFileLimitAndTruncatesTextContent(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("a", 140)
	writeFile(t, filepath.Join(root, "large.md"), content)

	collector := source.NewFileCollector(source.FileCollectorOptions{MaxTextBytes: 64})
	docs, err := collector.Collect(root)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected large text document to be collected, got %d", len(docs))
	}
	doc := docs[0]
	if doc.SizeBytes != int64(len(content)) {
		t.Fatalf("expected original size %d, got %d", len(content), doc.SizeBytes)
	}
	if len(doc.Content) != 64 {
		t.Fatalf("expected truncated content length 64, got %d", len(doc.Content))
	}
	if len(doc.ContentHash) != 64 {
		t.Fatalf("expected 64-character sha256 content hash, got %q", doc.ContentHash)
	}
}

func TestFileCollectorHonorsConfigurableMaxFileBytes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "too-large.txt"), strings.Repeat("x", 80))

	collector := source.NewFileCollector(source.FileCollectorOptions{MaxBytes: 64})
	docs, err := collector.Collect(root)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(docs) != 0 {
		t.Fatalf("expected file over configured limit to be skipped, got %#v", docs)
	}
}

func TestFileCollectorExtractsOfficeOpenXMLWithoutStoringOriginalBytes(t *testing.T) {
	root := t.TempDir()
	docxPath := filepath.Join(root, "report.docx")
	writeZip(t, docxPath, map[string]string{
		"word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>TiDB vector search report</w:t></w:r></w:p></w:body></w:document>`,
	})
	xlsxPath := filepath.Join(root, "metrics.xlsx")
	writeZip(t, xlsxPath, map[string]string{
		"xl/sharedStrings.xml": `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Quarterly revenue</t></si><si><t>Knowledge base</t></si></sst>`,
	})
	pptxPath := filepath.Join(root, "deck.pptx")
	writeZip(t, pptxPath, map[string]string{
		"ppt/slides/slide1.xml": `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Roadmap phase one</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
	})

	collector := source.NewFileCollector(source.FileCollectorOptions{MaxTextBytes: 1024})
	docs, err := collector.Collect(root)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	byName := map[string]source.Document{}
	for _, doc := range docs {
		byName[filepath.Base(doc.Path)] = doc
	}
	for name, want := range map[string]string{
		"report.docx":  "TiDB vector search report",
		"metrics.xlsx": "Quarterly revenue Knowledge base",
		"deck.pptx":    "Roadmap phase one",
	} {
		doc, ok := byName[name]
		if !ok {
			t.Fatalf("expected %s to be collected; got %#v", name, docs)
		}
		if doc.IsBinary {
			t.Fatalf("expected %s to be searchable text, not binary", name)
		}
		if doc.Asset != nil {
			t.Fatalf("expected %s original bytes not to be stored", name)
		}
		if !strings.Contains(doc.Content, want) {
			t.Fatalf("expected %s content to contain %q, got %q", name, want, doc.Content)
		}
	}
}

func TestFileCollectorRecordsLegacyOfficeAsAssetMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "legacy.doc"), []byte{0xd0, 0xcf, 0x11, 0xe0, 0x00, 0x00}, 0o644); err != nil {
		t.Fatalf("write legacy doc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "legacy.xls"), []byte{0xd0, 0xcf, 0x11, 0xe0, 0x00, 0x01}, 0o644); err != nil {
		t.Fatalf("write legacy xls: %v", err)
	}

	collector := source.NewFileCollector(source.FileCollectorOptions{})
	docs, err := collector.Collect(root)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(docs) != 2 {
		t.Fatalf("expected two legacy Office asset records, got %d: %#v", len(docs), docs)
	}
	for _, doc := range docs {
		if !doc.IsBinary {
			t.Fatalf("expected %s to be binary asset metadata", doc.Path)
		}
		if doc.Asset != nil {
			t.Fatalf("expected legacy Office asset bytes to be omitted for %s", doc.Path)
		}
		if doc.Content != "" {
			t.Fatalf("expected legacy Office content to be empty, got %q", doc.Content)
		}
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

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}
