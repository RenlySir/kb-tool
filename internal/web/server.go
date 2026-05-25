package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/RenlySir/kb-tool/internal/store"
)

//go:embed static/*
var staticFiles embed.FS

type Config struct {
	APIToken string
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
	Source    string `json:"source"`
	Documents int    `json:"documents"`
	Tags      int    `json:"tags"`
	Error     string `json:"error,omitempty"`
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
	if strings.HasPrefix(r.URL.Path, "/api/") && !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/documents", s.handleDocuments)
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

func (s *Server) authorized(r *http.Request) bool {
	if s.config.APIToken == "" {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+s.config.APIToken {
		return true
	}
	return strings.HasSuffix(r.URL.Path, "/asset") && r.URL.Query().Get("access_token") == s.config.APIToken
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
		if err := s.repo.AddTags(r.Context(), id, request.Tags); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "tags": request.Tags})
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
		Sources []string `json:"sources"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	var results []IngestSourceResult
	for _, input := range request.Sources {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		result := IngestSourceResult{Source: input}
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
