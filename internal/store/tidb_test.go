package store_test

import (
	"strings"
	"testing"

	"github.com/RenlySir/kb-tool/internal/store"
)

func TestBuildDSNUsesTiDBParameters(t *testing.T) {
	cfg := store.Config{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Password: "secret",
		Database: "kb",
	}

	dsn := store.BuildDSN(cfg)

	want := "root:secret@tcp(127.0.0.1:4000)/kb?charset=utf8mb4&parseTime=true&loc=Local&interpolateParams=true"
	if dsn != want {
		t.Fatalf("expected %q, got %q", want, dsn)
	}
}

func TestMigrationStatementsCreateKnowledgeBaseTables(t *testing.T) {
	stmts := store.MigrationStatements()
	if len(stmts) < 3 {
		t.Fatalf("expected migration statements, got %d", len(stmts))
	}

	joined := strings.Join(stmts, "\n")
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS kb_documents", "CREATE TABLE IF NOT EXISTS kb_tags", "CREATE TABLE IF NOT EXISTS kb_document_tags"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("expected migration to contain %q in:\n%s", fragment, joined)
		}
	}
	for _, fragment := range []string{"mime_type", "is_binary", "asset LONGBLOB"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("expected migration to contain asset fragment %q in:\n%s", fragment, joined)
		}
	}
	if !strings.Contains(joined, "path_hash CHAR(64)") {
		t.Fatalf("expected migration to contain path_hash for long path indexing:\n%s", joined)
	}
	if !strings.Contains(joined, "UNIQUE KEY uk_document_hash_path (content_hash, path_hash)") {
		t.Fatalf("expected migration to use path_hash in unique key:\n%s", joined)
	}
	if strings.Contains(joined, "UNIQUE KEY uk_document_hash_path (content_hash, path)") {
		t.Fatalf("migration still uses path directly in unique key:\n%s", joined)
	}
}
