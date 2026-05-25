package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RenlySir/kb-tool/internal/store"
	"github.com/RenlySir/kb-tool/internal/web"
)

func TestServerListsSearchesAndTagsDocuments(t *testing.T) {
	repo := &fakeRepository{
		documents: []store.DocumentRecord{{
			ID:         1,
			SourceType: "file",
			SourceURI:  "./docs",
			Path:       "README.md",
			Title:      "README.md",
			Language:   "markdown",
			Content:    "# KB Tool",
			MimeType:   "text/markdown; charset=utf-8",
			SizeBytes:  9,
			Tags:       []string{"documentation"},
		}},
	}
	server := web.NewServer(web.Config{}, repo, nil)

	list := httptest.NewRecorder()
	server.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/documents", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}
	var listBody struct {
		Documents []store.DocumentRecord `json:"documents"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listBody); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listBody.Documents) != 1 || listBody.Documents[0].Title != "README.md" {
		t.Fatalf("unexpected list response: %#v", listBody)
	}

	search := httptest.NewRecorder()
	server.ServeHTTP(search, httptest.NewRequest(http.MethodGet, "/api/search?q=tidb", nil))
	if search.Code != http.StatusOK {
		t.Fatalf("search status = %d, body = %s", search.Code, search.Body.String())
	}
	if repo.lastSearch != "tidb" {
		t.Fatalf("expected search query tidb, got %q", repo.lastSearch)
	}

	body := bytes.NewBufferString(`{"tags":["ai","tidb"]}`)
	addTags := httptest.NewRecorder()
	server.ServeHTTP(addTags, httptest.NewRequest(http.MethodPost, "/api/documents/1/tags", body))
	if addTags.Code != http.StatusOK {
		t.Fatalf("tag status = %d, body = %s", addTags.Code, addTags.Body.String())
	}
	if len(repo.addedTags) != 2 || repo.addedTags[0] != "ai" || repo.addedTags[1] != "tidb" {
		t.Fatalf("unexpected added tags: %#v", repo.addedTags)
	}
}

func TestServerReturnsImageAssetInline(t *testing.T) {
	png := []byte{0x89, 0x50, 0x4e, 0x47}
	repo := &fakeRepository{
		asset: store.AssetRecord{
			ID:       2,
			MimeType: "image/png",
			Bytes:    png,
		},
	}
	server := web.NewServer(web.Config{}, repo, nil)

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/documents/2/asset", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected image/png content type, got %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != "inline" {
		t.Fatalf("expected inline disposition, got %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), png) {
		t.Fatalf("asset body mismatch: %#v", rec.Body.Bytes())
	}
}

func TestServerIngestsBatchSources(t *testing.T) {
	repo := &fakeRepository{}
	ingester := web.IngesterFunc(func(ctx context.Context, input string) (web.IngestResult, error) {
		return web.IngestResult{Documents: 1, Tags: 2}, nil
	})
	server := web.NewServer(web.Config{}, repo, ingester)

	body := bytes.NewBufferString(`{"sources":["./docs","https://github.com/RenlySir/kb-tool.git"]}`)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/ingest", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Results []web.IngestSourceResult `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode ingest response: %v", err)
	}
	if len(response.Results) != 2 || response.Results[0].Documents != 1 || response.Results[1].Tags != 2 {
		t.Fatalf("unexpected ingest response: %#v", response)
	}
}

type fakeRepository struct {
	documents  []store.DocumentRecord
	asset      store.AssetRecord
	addedTags  []string
	lastSearch string
}

func (r *fakeRepository) ListDocuments(ctx context.Context, filter store.DocumentFilter) ([]store.DocumentRecord, error) {
	return r.documents, nil
}

func (r *fakeRepository) GetDocument(ctx context.Context, id int64) (store.DocumentRecord, error) {
	for _, doc := range r.documents {
		if doc.ID == id {
			return doc, nil
		}
	}
	return store.DocumentRecord{ID: id}, nil
}

func (r *fakeRepository) GetAsset(ctx context.Context, id int64) (store.AssetRecord, error) {
	return r.asset, nil
}

func (r *fakeRepository) ListTags(ctx context.Context) ([]store.TagRecord, error) {
	return []store.TagRecord{{Name: "documentation", Count: 1}}, nil
}

func (r *fakeRepository) AddTags(ctx context.Context, documentID int64, tags []string) error {
	r.addedTags = tags
	return nil
}

func (r *fakeRepository) Search(ctx context.Context, query string, limit int) ([]store.SearchResult, error) {
	r.lastSearch = query
	return []store.SearchResult{{ID: 1, Title: "README.md", Tags: []string{"documentation"}}}, nil
}
