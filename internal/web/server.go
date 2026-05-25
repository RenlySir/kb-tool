package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/RenlySir/kb-tool/internal/store"
)

//go:embed static/*
var staticFiles embed.FS

type Config struct {
	APIToken      string
	AdminUser     string
	AdminPassword string
}

type Repository interface {
	ListDocuments(ctx context.Context, filter store.DocumentFilter) ([]store.DocumentRecord, error)
	GetDocument(ctx context.Context, id int64) (store.DocumentRecord, error)
	GetAsset(ctx context.Context, id int64) (store.AssetRecord, error)
	ListTags(ctx context.Context) ([]store.TagRecord, error)
	AddTags(ctx context.Context, documentID int64, tags []string) error
	Search(ctx context.Context, query string, limit int) ([]store.SearchResult, error)
}

type Ingester interface {
	Ingest(ctx context.Context, input string) (IngestResult, error)
}

type IngesterFunc func(ctx context.Context, input string) (IngestResult, error)

func (f IngesterFunc) Ingest(ctx context.Context, input string) (IngestResult, error) {
	return f(ctx, input)
}

type IngestResult struct {
	Documents int `json:"documents"`
	Tags      int `json:"tags"`
}

type IngestSourceResult struct {
	Source     string `json:"source"`
	SourceType string `json:"source_type"`
	Documents  int    `json:"documents"`
	Tags       int    `json:"tags"`
	Error      string `json:"error,omitempty"`
}

type Server struct {
	config   Config
	repo     Repository
	ingester Ingester
	mux      *http.ServeMux
}

func NewServer(config Config, repo Repository, ingester Ingester) *Server {
	server := &Server{config: config, repo: repo, ingester: ingester, mux: http.NewServeMux()}
	server.routes()
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") && !s.publicAPI(r.URL.Path) && !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/session", s.handleSession)
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/documents", s.handleDocuments)
	s.mux.HandleFunc("/api/documents/tags", s.handleBatchTags)
	s.mux.HandleFunc("/api/documents/", s.handleDocument)
	s.mux.HandleFunc("/api/tags", s.handleTags)
	s.mux.HandleFunc("/api/search", s.handleSearch)
	s.mux.HandleFunc("/api/ingest", s.handleIngest)

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	s.mux.Handle("/", http.FileServer(http.FS(sub)))
}

func (s *Server) publicAPI(path string) bool {
	switch path {
	case "/api/health", "/api/session", "/api/login":
		return true
	default:
		return false
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if s.config.APIToken == "" {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+s.config.APIToken {
		return true
	}
	return strings.HasSuffix(r.URL.Path, "/asset") && r.URL.Query().Get("access_token") == s.config.APIToken
}

func (s *Server) adminUser() string {
	if strings.TrimSpace(s.config.AdminUser) == "" {
		return "admin"
	}
	return s.config.AdminUser
}

func (s *Server) adminPassword() string {
	if s.config.AdminPassword == "" {
		return "admin123"
	}
	return s.config.AdminPassword
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"auth_required": s.config.APIToken != "",
		"username":      s.adminUser(),
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if request.Username != s.adminUser() || request.Password != s.adminPassword() {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"token":    s.config.APIToken,
		"username": s.adminUser(),
	})
}

func (s *Server) handleDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
	docs, err := s.repo.ListDocuments(r.Context(), store.DocumentFilter{
		Query: r.URL.Query().Get("q"),
		Limit: limit,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documents": docs})
}

func (s *Server) handleBatchTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var request struct {
		DocumentIDs []int64  `json:"document_ids"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	tags := cleanTags(request.Tags)
	if len(request.DocumentIDs) == 0 || len(tags) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "document_ids and tags are required"})
		return
	}
	for _, id := range request.DocumentIDs {
		if id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "document_ids must be positive"})
			return
		}
		if err := s.repo.AddTags(r.Context(), id, tags); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"document_ids": request.DocumentIDs, "tags": tags})
}

func (s *Server) handleDocument(w http.ResponseWriter, r *http.Request) {
	id, rest, err := parseDocumentPath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	switch {
	case rest == "" && r.Method == http.MethodGet:
		doc, err := s.repo.GetDocument(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	case rest == "asset" && r.Method == http.MethodGet:
		asset, err := s.repo.GetAsset(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", asset.MimeType)
		w.Header().Set("Content-Disposition", "inline")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(asset.Bytes)
	case rest == "tags" && r.Method == http.MethodPost:
		var request struct {
			Tags []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		tags := cleanTags(request.Tags)
		if len(tags) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tags are required"})
			return
		}
		if err := s.repo.AddTags(r.Context(), id, tags); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "tags": tags})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	tags, err := s.repo.ListTags(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 20)
	results, err := s.repo.Search(r.Context(), r.URL.Query().Get("q"), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if s.ingester == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ingest service unavailable"})
		return
	}
	var request struct {
		SourceType string   `json:"source_type"`
		Sources    []string `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	sourceType, err := normalizeSourceType(request.SourceType)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var results []IngestSourceResult
	for _, input := range request.Sources {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		result := IngestSourceResult{Source: input, SourceType: sourceType}
		if err := validateIngestSource(sourceType, input); err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		ingestResult, err := s.ingester.Ingest(r.Context(), input)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Documents = ingestResult.Documents
			result.Tags = ingestResult.Tags
		}
		results = append(results, result)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func cleanTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	cleaned := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		cleaned = append(cleaned, tag)
	}
	return cleaned
}

func normalizeSourceType(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		value = "auto"
	}
	switch value {
	case "auto", "file", "github", "gitlab", "office", "image":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported source_type %q", raw)
	}
}

func validateIngestSource(sourceType string, input string) error {
	lower := strings.ToLower(input)
	switch sourceType {
	case "github":
		if strings.Contains(lower, "github.com/") || strings.HasPrefix(lower, "git@github.com:") {
			return nil
		}
		return fmt.Errorf("source is not a GitHub repository")
	case "gitlab":
		if strings.Contains(lower, "gitlab.") || strings.Contains(lower, "gitlab.com/") || strings.HasPrefix(lower, "git@gitlab.") {
			return nil
		}
		return fmt.Errorf("source is not a GitLab repository")
	case "office":
		ext := strings.ToLower(filepath.Ext(strings.TrimSuffix(input, ".git")))
		if ext == ".doc" || ext == ".docx" || ext == ".xls" || ext == ".xlsx" || ext == ".ppt" || ext == ".pptx" {
			return nil
		}
		return fmt.Errorf("source is not an Office document")
	case "image":
		ext := strings.ToLower(filepath.Ext(input))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" {
			return nil
		}
		return fmt.Errorf("source is not a supported image")
	default:
		return nil
	}
}

func parseDocumentPath(path string) (int64, string, error) {
	trimmed := strings.TrimPrefix(path, "/api/documents/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if parts[0] == "" {
		return 0, "", errors.New("missing document id")
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid document id")
	}
	if len(parts) == 1 {
		return id, "", nil
	}
	return id, strings.Join(parts[1:], "/"), nil
}

func parseIntDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}
