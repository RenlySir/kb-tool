package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestServerManagesDatabaseConnections(t *testing.T) {
	manager := &fakeConnectionManager{
		connections: []web.ConnectionView{{
			ID:          "default",
			Name:        "默认 TiDB",
			Host:        "tidb",
			Port:        4000,
			User:        "root",
			Database:    "kb",
			Active:      true,
			HasPassword: false,
		}},
	}
	server := web.NewServer(web.Config{}, &fakeRepository{}, nil, web.WithConnectionManager(manager))

	list := httptest.NewRecorder()
	server.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/connections", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list connections status = %d, body = %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), `"password":`) {
		t.Fatalf("connection list leaked password field: %s", list.Body.String())
	}

	create := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name":"生产 TiDB","host":"10.0.0.8","port":4000,"user":"kb","password":"secret","database":"kb_prod"}`)
	server.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/connections", body))
	if create.Code != http.StatusOK {
		t.Fatalf("create connection status = %d, body = %s", create.Code, create.Body.String())
	}
	if manager.saved.Password != "secret" {
		t.Fatalf("expected manager to receive password, got %#v", manager.saved)
	}
	if strings.Contains(create.Body.String(), "secret") || strings.Contains(create.Body.String(), `"password":`) {
		t.Fatalf("create response leaked password: %s", create.Body.String())
	}

	activate := httptest.NewRecorder()
	server.ServeHTTP(activate, httptest.NewRequest(http.MethodPost, "/api/connections/prod/activate", nil))
	if activate.Code != http.StatusOK {
		t.Fatalf("activate connection status = %d, body = %s", activate.Code, activate.Body.String())
	}
	if manager.activatedID != "prod" {
		t.Fatalf("expected prod activation, got %q", manager.activatedID)
	}
}

func TestServerReturnsKnowledgeOverview(t *testing.T) {
	repo := &fakeRepository{
		documents: []store.DocumentRecord{
			{
				ID:         11,
				SourceType: "github",
				Path:       "README.md",
				Title:      "README.md",
				Language:   "markdown",
				MimeType:   "text/markdown; charset=utf-8",
				Tags:       []string{"文档"},
			},
			{
				ID:         12,
				SourceType: "file",
				Path:       "images/arch.png",
				Title:      "arch.png",
				MimeType:   "image/png",
				IsBinary:   true,
				Tags:       []string{"架构"},
			},
			{
				ID:         13,
				SourceType: "file",
				Path:       "spec.doc",
				Title:      "spec.doc",
				MimeType:   "application/msword",
				IsBinary:   true,
			},
		},
		tags: []store.TagRecord{
			{Name: "文档", Count: 1},
			{Name: "架构", Count: 1},
		},
	}
	server := web.NewServer(web.Config{}, repo, nil)

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/overview", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("overview status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Overview web.Overview `json:"overview"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if response.Overview.TotalDocuments != 3 || response.Overview.TotalTags != 2 {
		t.Fatalf("unexpected totals: %#v", response.Overview)
	}
	if response.Overview.BinaryDocuments != 2 || response.Overview.ImageDocuments != 1 {
		t.Fatalf("unexpected asset totals: %#v", response.Overview)
	}
	if len(response.Overview.SourceTypes) == 0 || response.Overview.SourceTypes[0].Name != "file" || response.Overview.SourceTypes[0].Count != 2 {
		t.Fatalf("unexpected source type counts: %#v", response.Overview.SourceTypes)
	}
	if len(response.Overview.RecentDocuments) != 3 || response.Overview.RecentDocuments[0].ID != 11 {
		t.Fatalf("unexpected recent documents: %#v", response.Overview.RecentDocuments)
	}
}

type addTagCall struct {
	documentID int64
	tags       []string
}

type fakeRepository struct {
	documents   []store.DocumentRecord
	tags        []store.TagRecord
	asset       store.AssetRecord
	addTagCalls []addTagCall
	lastSearch  string
}

type fakeConnectionManager struct {
	connections []web.ConnectionView
	saved       web.ConnectionInput
	activatedID string
}

func (m *fakeConnectionManager) ListConnections(ctx context.Context) ([]web.ConnectionView, error) {
	return m.connections, nil
}

func (m *fakeConnectionManager) CurrentConnection(ctx context.Context) (web.ConnectionView, error) {
	for _, conn := range m.connections {
		if conn.Active {
			return conn, nil
		}
	}
	return web.ConnectionView{}, nil
}

func (m *fakeConnectionManager) SaveConnection(ctx context.Context, input web.ConnectionInput) (web.ConnectionView, error) {
	m.saved = input
	return web.ConnectionView{
		ID:          "prod",
		Name:        input.Name,
		Host:        input.Host,
		Port:        input.Port,
		User:        input.User,
		Database:    input.Database,
		HasPassword: input.Password != "",
	}, nil
}

func (m *fakeConnectionManager) TestConnection(ctx context.Context, input web.ConnectionInput) error {
	return nil
}

func (m *fakeConnectionManager) ActivateConnection(ctx context.Context, id string) (web.ConnectionView, error) {
	m.activatedID = id
	return web.ConnectionView{ID: id, Active: true}, nil
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
	if r.tags != nil {
		return r.tags, nil
	}
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
