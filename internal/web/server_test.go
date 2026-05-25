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
	if len(repo.addTagCalls) != 1 {
		t.Fatalf("expected one tag call, got %#v", repo.addTagCalls)
	}
	if repo.addTagCalls[0].documentID != 1 {
		t.Fatalf("unexpected tag document id: %d", repo.addTagCalls[0].documentID)
	}
	if len(repo.addTagCalls[0].tags) != 2 || repo.addTagCalls[0].tags[0] != "ai" || repo.addTagCalls[0].tags[1] != "tidb" {
		t.Fatalf("unexpected added tags: %#v", repo.addTagCalls[0].tags)
	}
}

func TestServerLoginReturnsConfiguredAPIToken(t *testing.T) {
	server := web.NewServer(web.Config{
		APIToken:      "secret-token",
		AdminUser:     "admin",
		AdminPassword: "admin-pass",
	}, &fakeRepository{}, nil)

	session := httptest.NewRecorder()
	server.ServeHTTP(session, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if session.Code != http.StatusOK {
		t.Fatalf("session status = %d, body = %s", session.Code, session.Body.String())
	}
	var sessionBody struct {
		AuthRequired bool   `json:"auth_required"`
		Username     string `json:"username"`
	}
	if err := json.Unmarshal(session.Body.Bytes(), &sessionBody); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if !sessionBody.AuthRequired || sessionBody.Username != "admin" {
		t.Fatalf("unexpected session body: %#v", sessionBody)
	}

	login := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"username":"admin","password":"admin-pass"}`)
	server.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/login", body))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
	}
	var loginBody struct {
		Token    string `json:"token"`
		Username string `json:"username"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &loginBody); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if loginBody.Token != "secret-token" || loginBody.Username != "admin" {
		t.Fatalf("unexpected login response: %#v", loginBody)
	}

	bad := httptest.NewRecorder()
	server.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"username":"admin","password":"wrong"}`)))
	if bad.Code != http.StatusUnauthorized {
		t.Fatalf("expected bad login to be unauthorized, got %d", bad.Code)
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

func TestServerRequiresAndAcceptsAPIToken(t *testing.T) {
	repo := &fakeRepository{}
	server := web.NewServer(web.Config{APIToken: "secret"}, repo, nil)

	unauthorized := httptest.NewRecorder()
	server.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/documents", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized status, got %d", unauthorized.Code)
	}

	queryToken := httptest.NewRecorder()
	server.ServeHTTP(queryToken, httptest.NewRequest(http.MethodGet, "/api/documents?access_token=secret", nil))
	if queryToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected document query token to stay unauthorized, got %d", queryToken.Code)
	}

	authorized := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
	request.Header.Set("Authorization", "Bearer secret")
	server.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusOK {
		t.Fatalf("expected authorized status, got %d", authorized.Code)
	}

	asset := httptest.NewRecorder()
	server.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/api/documents/1/asset?access_token=secret", nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("expected tokenized asset status, got %d", asset.Code)
	}
}

func TestServerAddsTagsToBatchDocuments(t *testing.T) {
	repo := &fakeRepository{}
	server := web.NewServer(web.Config{}, repo, nil)

	body := bytes.NewBufferString(`{"document_ids":[1,2],"tags":["合同","AI"]}`)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/documents/tags", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("batch tag status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(repo.addTagCalls) != 2 {
		t.Fatalf("expected two tag calls, got %#v", repo.addTagCalls)
	}
	if repo.addTagCalls[0].documentID != 1 || repo.addTagCalls[1].documentID != 2 {
		t.Fatalf("unexpected batch document ids: %#v", repo.addTagCalls)
	}
	if len(repo.addTagCalls[1].tags) != 2 || repo.addTagCalls[1].tags[0] != "合同" || repo.addTagCalls[1].tags[1] != "AI" {
		t.Fatalf("unexpected batch tags: %#v", repo.addTagCalls[1].tags)
	}
}

func TestServerIngestsBatchSources(t *testing.T) {
	repo := &fakeRepository{}
	ingester := web.IngesterFunc(func(ctx context.Context, input string) (web.IngestResult, error) {
		return web.IngestResult{Documents: 1, Tags: 2}, nil
	})
	server := web.NewServer(web.Config{}, repo, ingester)

	body := bytes.NewBufferString(`{"source_type":"github","sources":["https://github.com/RenlySir/kb-tool.git"]}`)
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
	if len(response.Results) != 1 || response.Results[0].Documents != 1 || response.Results[0].Tags != 2 {
		t.Fatalf("unexpected ingest response: %#v", response)
	}
	if response.Results[0].SourceType != "github" {
		t.Fatalf("expected github source type, got %q", response.Results[0].SourceType)
	}
}

type addTagCall struct {
	documentID int64
	tags       []string
}

type fakeRepository struct {
	documents   []store.DocumentRecord
	asset       store.AssetRecord
	addTagCalls []addTagCall
	lastSearch  string
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
	copied := append([]string(nil), tags...)
	r.addTagCalls = append(r.addTagCalls, addTagCall{documentID: documentID, tags: copied})
	return nil
}

func (r *fakeRepository) Search(ctx context.Context, query string, limit int) ([]store.SearchResult, error) {
	r.lastSearch = query
	return []store.SearchResult{{ID: 1, Title: "README.md", Tags: []string{"documentation"}}}, nil
}
