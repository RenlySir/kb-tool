package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/RenlySir/kb-tool/internal/ingest"
	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

func DefaultConfig() Config {
	return Config{
		Host:     "127.0.0.1",
		Port:     4000,
		User:     "root",
		Database: "kb",
	}
}

func BuildDSN(cfg Config) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local&interpolateParams=true",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
	)
}

func buildServerDSN(cfg Config) string {
	params := url.Values{}
	params.Set("charset", "utf8mb4")
	params.Set("parseTime", "true")
	params.Set("loc", "Local")
	params.Set("interpolateParams", "true")
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?%s", cfg.User, cfg.Password, cfg.Host, cfg.Port, params.Encode())
}

type TiDBStore struct {
	db *sql.DB
}

func Open(ctx context.Context, cfg Config) (*TiDBStore, error) {
	serverDB, err := sql.Open("mysql", buildServerDSN(cfg))
	if err != nil {
		return nil, err
	}
	defer serverDB.Close()

	if _, err := serverDB.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+quoteIdent(cfg.Database)+" DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin"); err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", BuildDSN(cfg))
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &TiDBStore{db: db}, nil
}

func (s *TiDBStore) Close() error {
	return s.db.Close()
}

func (s *TiDBStore) Migrate(ctx context.Context) error {
	for _, stmt := range MigrationStatements() {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *TiDBStore) Save(ctx context.Context, doc ingest.TaggedDocument) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO kb_documents
  (content_hash, source_type, source_uri, path, title, language, content, size_bytes, mime_type, is_binary, asset)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  source_type = VALUES(source_type),
  source_uri = VALUES(source_uri),
  title = VALUES(title),
  language = VALUES(language),
  content = VALUES(content),
  size_bytes = VALUES(size_bytes),
  mime_type = VALUES(mime_type),
  is_binary = VALUES(is_binary),
  asset = VALUES(asset),
  updated_at = CURRENT_TIMESTAMP`,
		doc.ContentHash,
		doc.SourceType,
		doc.SourceURI,
		doc.Path,
		doc.Title,
		doc.Language,
		doc.Content,
		doc.SizeBytes,
		doc.MimeType,
		doc.IsBinary,
		doc.Asset,
	)
	if err != nil {
		return err
	}

	var documentID int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM kb_documents WHERE content_hash = ? AND path = ?", doc.ContentHash, doc.Path).Scan(&documentID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM kb_document_tags WHERE document_id = ?", documentID); err != nil {
		return err
	}
	for _, tag := range doc.Tags {
		normalized := strings.TrimSpace(strings.ToLower(tag))
		if normalized == "" {
			continue
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO kb_tags (name) VALUES (?) ON DUPLICATE KEY UPDATE name = VALUES(name)", normalized)
		if err != nil {
			return err
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM kb_tags WHERE name = ?", normalized).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT IGNORE INTO kb_document_tags (document_id, tag_id) VALUES (?, ?)", documentID, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *TiDBStore) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  d.id, d.source_type, d.source_uri, d.path, d.title, d.language,
  LEFT(d.content, 240) AS snippet,
  COALESCE(tag_names.names, '') AS tags
FROM kb_documents d
LEFT JOIN (
  SELECT dt.document_id, GROUP_CONCAT(t.name ORDER BY t.name SEPARATOR ',') AS names
  FROM kb_document_tags dt
  JOIN kb_tags t ON t.id = dt.tag_id
  GROUP BY dt.document_id
) tag_names ON tag_names.document_id = d.id
WHERE d.title LIKE ? OR d.path LIKE ? OR d.content LIKE ?
ORDER BY d.updated_at DESC
LIMIT ?`, like(query), like(query), like(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var result SearchResult
		var tagCSV string
		if err := rows.Scan(&result.ID, &result.SourceType, &result.SourceURI, &result.Path, &result.Title, &result.Language, &result.Snippet, &tagCSV); err != nil {
			return nil, err
		}
		if tagCSV != "" {
			result.Tags = strings.Split(tagCSV, ",")
		}
		results = append(results, result)
	}
	return results, rows.Err()
}

func (s *TiDBStore) ListDocuments(ctx context.Context, filter DocumentFilter) ([]DocumentRecord, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query := strings.TrimSpace(filter.Query)
	rows, err := s.db.QueryContext(ctx, `
SELECT
  d.id, d.source_type, d.source_uri, d.path, d.title, d.language,
  d.content, d.content_hash, d.size_bytes, d.mime_type, d.is_binary,
  COALESCE(tag_names.names, '') AS tags
FROM kb_documents d
LEFT JOIN (
  SELECT dt.document_id, GROUP_CONCAT(t.name ORDER BY t.name SEPARATOR ',') AS names
  FROM kb_document_tags dt
  JOIN kb_tags t ON t.id = dt.tag_id
  GROUP BY dt.document_id
) tag_names ON tag_names.document_id = d.id
WHERE (? = '' OR d.title LIKE ? OR d.path LIKE ? OR d.content LIKE ?)
ORDER BY d.updated_at DESC
LIMIT ?`, query, like(query), like(query), like(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocumentRows(rows)
}

func (s *TiDBStore) GetDocument(ctx context.Context, id int64) (DocumentRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  d.id, d.source_type, d.source_uri, d.path, d.title, d.language,
  d.content, d.content_hash, d.size_bytes, d.mime_type, d.is_binary,
  COALESCE(tag_names.names, '') AS tags
FROM kb_documents d
LEFT JOIN (
  SELECT dt.document_id, GROUP_CONCAT(t.name ORDER BY t.name SEPARATOR ',') AS names
  FROM kb_document_tags dt
  JOIN kb_tags t ON t.id = dt.tag_id
  GROUP BY dt.document_id
) tag_names ON tag_names.document_id = d.id
WHERE d.id = ?`, id)
	if err != nil {
		return DocumentRecord{}, err
	}
	defer rows.Close()
	docs, err := scanDocumentRows(rows)
	if err != nil {
		return DocumentRecord{}, err
	}
	if len(docs) == 0 {
		return DocumentRecord{}, sql.ErrNoRows
	}
	return docs[0], nil
}

func (s *TiDBStore) GetAsset(ctx context.Context, id int64) (AssetRecord, error) {
	var asset AssetRecord
	err := s.db.QueryRowContext(ctx, "SELECT id, mime_type, asset FROM kb_documents WHERE id = ? AND is_binary = TRUE", id).Scan(&asset.ID, &asset.MimeType, &asset.Bytes)
	return asset, err
}

func (s *TiDBStore) ListTags(ctx context.Context) ([]TagRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.name, COUNT(dt.document_id) AS count
FROM kb_tags t
LEFT JOIN kb_document_tags dt ON dt.tag_id = t.id
GROUP BY t.id, t.name
ORDER BY t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []TagRecord
	for rows.Next() {
		var tag TagRecord
		if err := rows.Scan(&tag.Name, &tag.Count); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *TiDBStore) AddTags(ctx context.Context, documentID int64, tags []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, tag := range tags {
		normalized := strings.TrimSpace(strings.ToLower(tag))
		if normalized == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO kb_tags (name) VALUES (?) ON DUPLICATE KEY UPDATE name = VALUES(name)", normalized); err != nil {
			return err
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, "SELECT id FROM kb_tags WHERE name = ?", normalized).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT IGNORE INTO kb_document_tags (document_id, tag_id) VALUES (?, ?)", documentID, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type SearchResult struct {
	ID         int64
	SourceType string
	SourceURI  string
	Path       string
	Title      string
	Language   string
	Snippet    string
	Tags       []string
}

type DocumentFilter struct {
	Query string
	Limit int
}

type DocumentRecord struct {
	ID          int64    `json:"id"`
	SourceType  string   `json:"source_type"`
	SourceURI   string   `json:"source_uri"`
	Path        string   `json:"path"`
	Title       string   `json:"title"`
	Language    string   `json:"language"`
	Content     string   `json:"content,omitempty"`
	ContentHash string   `json:"content_hash"`
	SizeBytes   int64    `json:"size_bytes"`
	MimeType    string   `json:"mime_type"`
	IsBinary    bool     `json:"is_binary"`
	Tags        []string `json:"tags"`
}

type TagRecord struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type AssetRecord struct {
	ID       int64
	MimeType string
	Bytes    []byte
}

func MigrationStatements() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS kb_documents (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  content_hash CHAR(64) NOT NULL,
  source_type VARCHAR(32) NOT NULL,
  source_uri TEXT NOT NULL,
  path VARCHAR(1024) NOT NULL,
  title VARCHAR(512) NOT NULL,
  language VARCHAR(64) NOT NULL,
  content LONGTEXT NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0,
  mime_type VARCHAR(128) NOT NULL DEFAULT 'text/plain; charset=utf-8',
  is_binary BOOLEAN NOT NULL DEFAULT FALSE,
  asset LONGBLOB NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_document_hash_path (content_hash, path),
  KEY idx_source_type (source_type),
  KEY idx_language (language)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
		`CREATE TABLE IF NOT EXISTS kb_tags (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_tag_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
		`CREATE TABLE IF NOT EXISTS kb_document_tags (
  document_id BIGINT NOT NULL,
  tag_id BIGINT NOT NULL,
  PRIMARY KEY (document_id, tag_id),
  KEY idx_tag_id (tag_id),
  CONSTRAINT fk_kb_document_tags_document FOREIGN KEY (document_id) REFERENCES kb_documents(id) ON DELETE CASCADE,
  CONSTRAINT fk_kb_document_tags_tag FOREIGN KEY (tag_id) REFERENCES kb_tags(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`,
	}
}

func scanDocumentRows(rows *sql.Rows) ([]DocumentRecord, error) {
	var docs []DocumentRecord
	for rows.Next() {
		var doc DocumentRecord
		var tagCSV string
		if err := rows.Scan(
			&doc.ID,
			&doc.SourceType,
			&doc.SourceURI,
			&doc.Path,
			&doc.Title,
			&doc.Language,
			&doc.Content,
			&doc.ContentHash,
			&doc.SizeBytes,
			&doc.MimeType,
			&doc.IsBinary,
			&tagCSV,
		); err != nil {
			return nil, err
		}
		if tagCSV != "" {
			doc.Tags = strings.Split(tagCSV, ",")
		}
		docs = append(docs, doc)
	}
	return docs, rows.Err()
}

func like(query string) string {
	return "%" + query + "%"
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
