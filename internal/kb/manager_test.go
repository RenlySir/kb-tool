package kb_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RenlySir/kb-tool/internal/ingest"
	"github.com/RenlySir/kb-tool/internal/kb"
	"github.com/RenlySir/kb-tool/internal/store"
)

func TestManagerPersistsConnectionsAndHidesPasswords(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "connections.json")
	var opened []store.Config
	manager, err := kb.NewManager(ctx, kb.Config{
		ConnectionsFile: path,
		Default: store.Config{
			Host:     "127.0.0.1",
			Port:     4000,
			User:     "root",
			Password: "default-secret",
			Database: "kb",
		},
		OpenStore: func(ctx context.Context, cfg store.Config) (kb.Store, error) {
			opened = append(opened, cfg)
			return &fakeKBStore{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	view, err := manager.SaveConnection(ctx, kb.ConnectionInput{
		Name:     "Production TiDB",
		Host:     "10.0.0.8",
		Port:     4000,
		User:     "kb_user",
		Password: "prod-secret",
		Database: "kb_prod",
	})
	if err != nil {
		t.Fatalf("SaveConnection returned error: %v", err)
	}
	viewBytes, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	if view.ID == "" || !view.HasPassword || strings.Contains(string(viewBytes), "prod-secret") || strings.Contains(string(viewBytes), `"password":`) {
		t.Fatalf("password leaked or id missing in view: %#v", view)
	}

	if _, err := manager.ActivateConnection(ctx, view.ID); err != nil {
		t.Fatalf("ActivateConnection returned error: %v", err)
	}
	current, err := manager.CurrentConnection(ctx)
	if err != nil {
		t.Fatalf("CurrentConnection returned error: %v", err)
	}
	if current.ID != view.ID || !current.Active {
		t.Fatalf("unexpected current connection: %#v", current)
	}

	manager2, err := kb.NewManager(ctx, kb.Config{
		ConnectionsFile: filepath.Join(filepath.Dir(filepath.Join(t.TempDir(), "unused")), ".."),
	})
	if err == nil && manager2 != nil {
		t.Fatal("expected invalid connection path to fail")
	}

	if len(opened) < 2 {
		t.Fatalf("expected default and activated stores to open, got %#v", opened)
	}
	activated := opened[len(opened)-1]
	if activated.Host != "10.0.0.8" || activated.User != "kb_user" || activated.Password != "prod-secret" || activated.Database != "kb_prod" {
		t.Fatalf("activated wrong config: %#v", activated)
	}

	reloaded, err := kb.NewManager(ctx, kb.Config{
		ConnectionsFile: path,
		Default:         store.DefaultConfig(),
		OpenStore: func(ctx context.Context, cfg store.Config) (kb.Store, error) {
			return &fakeKBStore{}, nil
		},
	})
	if err != nil {
		t.Fatalf("reload NewManager returned error: %v", err)
	}
	reloadedCurrent, err := reloaded.CurrentConnection(ctx)
	if err != nil {
		t.Fatalf("reloaded CurrentConnection returned error: %v", err)
	}
	reloadedBytes, err := json.Marshal(reloadedCurrent)
	if err != nil {
		t.Fatalf("marshal reloaded current: %v", err)
	}
	if reloadedCurrent.ID != view.ID || !reloadedCurrent.HasPassword || strings.Contains(string(reloadedBytes), "prod-secret") || strings.Contains(string(reloadedBytes), `"password":`) {
		t.Fatalf("unexpected reloaded current connection: %#v", reloadedCurrent)
	}
}

func TestManagerPreservesPasswordWhenUpdateOmitsIt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "connections.json")
	manager, err := kb.NewManager(ctx, kb.Config{
		ConnectionsFile: path,
		Default:         store.DefaultConfig(),
		OpenStore: func(ctx context.Context, cfg store.Config) (kb.Store, error) {
			return &fakeKBStore{}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	created, err := manager.SaveConnection(ctx, kb.ConnectionInput{
		Name:     "Team TiDB",
		Host:     "10.0.0.9",
		Port:     4000,
		User:     "team",
		Password: "keep-me",
		Database: "kb_team",
	})
	if err != nil {
		t.Fatalf("SaveConnection returned error: %v", err)
	}
	if _, err := manager.SaveConnection(ctx, kb.ConnectionInput{
		ID:       created.ID,
		Name:     "Team TiDB Updated",
		Host:     "10.0.0.10",
		Port:     4001,
		User:     "team2",
		Database: "kb_team2",
	}); err != nil {
		t.Fatalf("SaveConnection update returned error: %v", err)
	}

	var tested store.Config
	manager.OpenStore = func(ctx context.Context, cfg store.Config) (kb.Store, error) {
		tested = cfg
		return &fakeKBStore{}, nil
	}
	if _, err := manager.ActivateConnection(ctx, created.ID); err != nil {
		t.Fatalf("ActivateConnection returned error: %v", err)
	}
	if tested.Password != "keep-me" {
		t.Fatalf("expected preserved password, got %#v", tested)
	}

	tested = store.Config{}
	if err := manager.TestConnection(ctx, kb.ConnectionInput{
		ID:       created.ID,
		Name:     "Team TiDB Updated",
		Host:     "10.0.0.10",
		Port:     4001,
		User:     "team2",
		Database: "kb_team2",
	}); err != nil {
		t.Fatalf("TestConnection returned error: %v", err)
	}
	if tested.Password != "keep-me" {
		t.Fatalf("expected test connection to preserve password, got %#v", tested)
	}
}

type fakeKBStore struct{}

func (s *fakeKBStore) Close() error { return nil }

func (s *fakeKBStore) Migrate(ctx context.Context) error { return nil }

func (s *fakeKBStore) Save(ctx context.Context, doc ingest.TaggedDocument) error { return nil }

func (s *fakeKBStore) ListDocuments(ctx context.Context, filter store.DocumentFilter) ([]store.DocumentRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeKBStore) GetDocument(ctx context.Context, id int64) (store.DocumentRecord, error) {
	return store.DocumentRecord{}, errors.New("not implemented")
}

func (s *fakeKBStore) GetAsset(ctx context.Context, id int64) (store.AssetRecord, error) {
	return store.AssetRecord{}, errors.New("not implemented")
}

func (s *fakeKBStore) ListTags(ctx context.Context) ([]store.TagRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeKBStore) AddTags(ctx context.Context, documentID int64, tags []string) error {
	return errors.New("not implemented")
}

func (s *fakeKBStore) Search(ctx context.Context, query string, limit int) ([]store.SearchResult, error) {
	return nil, errors.New("not implemented")
}
