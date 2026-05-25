package kb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/RenlySir/kb-tool/internal/ingest"
	"github.com/RenlySir/kb-tool/internal/store"
)

type Store interface {
	Close() error
	Migrate(ctx context.Context) error
	Save(ctx context.Context, doc ingest.TaggedDocument) error
	ListDocuments(ctx context.Context, filter store.DocumentFilter) ([]store.DocumentRecord, error)
	GetDocument(ctx context.Context, id int64) (store.DocumentRecord, error)
	GetAsset(ctx context.Context, id int64) (store.AssetRecord, error)
	ListTags(ctx context.Context) ([]store.TagRecord, error)
	AddTags(ctx context.Context, documentID int64, tags []string) error
	Search(ctx context.Context, query string, limit int) ([]store.SearchResult, error)
}

type OpenStoreFunc func(ctx context.Context, cfg store.Config) (Store, error)

type Config struct {
	ConnectionsFile string
	Default         store.Config
	OpenStore       OpenStoreFunc
}

type ConnectionInput struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	Database string `json:"database"`
}

type ConnectionView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Database    string `json:"database"`
	Active      bool   `json:"active"`
	HasPassword bool   `json:"has_password"`
}

type connectionRecord struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	Database string `json:"database"`
}

type persistedConfig struct {
	ActiveID    string             `json:"active_id"`
	Connections []connectionRecord `json:"connections"`
}

type Manager struct {
	mu            sync.RWMutex
	file          string
	connections   []connectionRecord
	activeID      string
	current       Store
	currentConfig store.Config
	defaultConfig store.Config
	OpenStore     OpenStoreFunc
}

func NewManager(ctx context.Context, cfg Config) (*Manager, error) {
	if cfg.OpenStore == nil {
		cfg.OpenStore = func(ctx context.Context, cfg store.Config) (Store, error) {
			return store.Open(ctx, cfg)
		}
	}
	if cfg.Default.Host == "" {
		cfg.Default = store.DefaultConfig()
	}
	if cfg.ConnectionsFile == "" {
		cfg.ConnectionsFile = "data/connections.json"
	}

	manager := &Manager{
		file:          cfg.ConnectionsFile,
		defaultConfig: cfg.Default,
		OpenStore:     cfg.OpenStore,
	}
	if err := manager.load(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return nil
	}
	return m.current.Close()
}

func (m *Manager) Store() Store {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *Manager) Migrate(ctx context.Context) error {
	return m.Store().Migrate(ctx)
}

func (m *Manager) Save(ctx context.Context, doc ingest.TaggedDocument) error {
	return m.Store().Save(ctx, doc)
}

func (m *Manager) ListDocuments(ctx context.Context, filter store.DocumentFilter) ([]store.DocumentRecord, error) {
	return m.Store().ListDocuments(ctx, filter)
}

func (m *Manager) GetDocument(ctx context.Context, id int64) (store.DocumentRecord, error) {
	return m.Store().GetDocument(ctx, id)
}

func (m *Manager) GetAsset(ctx context.Context, id int64) (store.AssetRecord, error) {
	return m.Store().GetAsset(ctx, id)
}

func (m *Manager) ListTags(ctx context.Context) ([]store.TagRecord, error) {
	return m.Store().ListTags(ctx)
}

func (m *Manager) AddTags(ctx context.Context, documentID int64, tags []string) error {
	return m.Store().AddTags(ctx, documentID, tags)
}

func (m *Manager) Search(ctx context.Context, query string, limit int) ([]store.SearchResult, error) {
	return m.Store().Search(ctx, query, limit)
}

func (m *Manager) ListConnections(ctx context.Context) ([]ConnectionView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	views := make([]ConnectionView, 0, len(m.connections))
	for _, conn := range m.connections {
		views = append(views, conn.view(conn.ID == m.activeID))
	}
	return views, nil
}

func (m *Manager) CurrentConnection(ctx context.Context) (ConnectionView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, conn := range m.connections {
		if conn.ID == m.activeID {
			return conn.view(true), nil
		}
	}
	return ConnectionView{}, errors.New("active connection not found")
}

func (m *Manager) SaveConnection(ctx context.Context, input ConnectionInput) (ConnectionView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, err := m.connectionRecordFromInput(input)
	if err != nil {
		return ConnectionView{}, err
	}

	index := -1
	for i, conn := range m.connections {
		if conn.ID == record.ID {
			index = i
			if record.Password == "" {
				record.Password = conn.Password
			}
			break
		}
	}
	if index >= 0 {
		m.connections[index] = record
	} else {
		m.connections = append(m.connections, record)
	}
	if err := m.persistLocked(); err != nil {
		return ConnectionView{}, err
	}
	return record.view(record.ID == m.activeID), nil
}

func (m *Manager) TestConnection(ctx context.Context, input ConnectionInput) error {
	record, err := m.resolveInput(input)
	if err != nil {
		return err
	}
	opened, err := m.OpenStore(ctx, record.storeConfig())
	if err != nil {
		return err
	}
	return opened.Close()
}

func (m *Manager) ActivateConnection(ctx context.Context, id string) (ConnectionView, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ConnectionView{}, errors.New("connection id is required")
	}

	m.mu.RLock()
	record, ok := m.findLocked(id)
	m.mu.RUnlock()
	if !ok {
		return ConnectionView{}, fmt.Errorf("connection %q not found", id)
	}

	opened, err := m.OpenStore(ctx, record.storeConfig())
	if err != nil {
		return ConnectionView{}, err
	}
	if err := opened.Migrate(ctx); err != nil {
		_ = opened.Close()
		return ConnectionView{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.current
	m.current = opened
	m.currentConfig = record.storeConfig()
	m.activeID = record.ID
	if err := m.persistLocked(); err != nil {
		m.current = old
		_ = opened.Close()
		return ConnectionView{}, err
	}
	if old != nil {
		_ = old.Close()
	}
	return record.view(true), nil
}

func (m *Manager) load(ctx context.Context) error {
	persisted := persistedConfig{}
	bytes, err := os.ReadFile(m.file)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		persisted = persistedConfig{
			ActiveID:    "default",
			Connections: []connectionRecord{recordFromConfig("default", "默认 TiDB", m.defaultConfig)},
		}
	} else if err := json.Unmarshal(bytes, &persisted); err != nil {
		return err
	}
	if len(persisted.Connections) == 0 {
		persisted.Connections = []connectionRecord{recordFromConfig("default", "默认 TiDB", m.defaultConfig)}
	}
	if persisted.ActiveID == "" {
		persisted.ActiveID = persisted.Connections[0].ID
	}
	m.connections = persisted.Connections
	m.activeID = persisted.ActiveID

	record, ok := m.findLocked(m.activeID)
	if !ok {
		record = m.connections[0]
		m.activeID = record.ID
	}
	opened, err := m.OpenStore(ctx, record.storeConfig())
	if err != nil {
		return err
	}
	m.current = opened
	m.currentConfig = record.storeConfig()
	if errors.Is(err, os.ErrNotExist) {
		return m.persistLocked()
	}
	return nil
}

func (m *Manager) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.file), 0o700); err != nil {
		return err
	}
	payload := persistedConfig{ActiveID: m.activeID, Connections: m.connections}
	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.file, append(bytes, '\n'), 0o600)
}

func (m *Manager) resolveInput(input ConnectionInput) (connectionRecord, error) {
	if strings.TrimSpace(input.ID) != "" && input.Host == "" && input.Database == "" {
		m.mu.RLock()
		defer m.mu.RUnlock()
		record, ok := m.findLocked(input.ID)
		if !ok {
			return connectionRecord{}, fmt.Errorf("connection %q not found", input.ID)
		}
		return record, nil
	}
	record, err := m.connectionRecordFromInput(input)
	if err != nil {
		return connectionRecord{}, err
	}
	if record.Password == "" && strings.TrimSpace(input.ID) != "" {
		m.mu.RLock()
		existing, ok := m.findLocked(input.ID)
		m.mu.RUnlock()
		if ok {
			record.Password = existing.Password
		}
	}
	return record, nil
}

func (m *Manager) connectionRecordFromInput(input ConnectionInput) (connectionRecord, error) {
	name := strings.TrimSpace(input.Name)
	host := strings.TrimSpace(input.Host)
	user := strings.TrimSpace(input.User)
	database := strings.TrimSpace(input.Database)
	if name == "" {
		name = database
	}
	if host == "" || user == "" || database == "" {
		return connectionRecord{}, errors.New("name, host, user, and database are required")
	}
	port := input.Port
	if port <= 0 {
		port = 4000
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = randomID()
	}
	return connectionRecord{
		ID:       id,
		Name:     name,
		Host:     host,
		Port:     port,
		User:     user,
		Password: input.Password,
		Database: database,
	}, nil
}

func (m *Manager) findLocked(id string) (connectionRecord, bool) {
	for _, conn := range m.connections {
		if conn.ID == id {
			return conn, true
		}
	}
	return connectionRecord{}, false
}

func (r connectionRecord) storeConfig() store.Config {
	return store.Config{
		Host:     r.Host,
		Port:     r.Port,
		User:     r.User,
		Password: r.Password,
		Database: r.Database,
	}
}

func (r connectionRecord) view(active bool) ConnectionView {
	return ConnectionView{
		ID:          r.ID,
		Name:        r.Name,
		Host:        r.Host,
		Port:        r.Port,
		User:        r.User,
		Database:    r.Database,
		Active:      active,
		HasPassword: r.Password != "",
	}
}

func recordFromConfig(id string, name string, cfg store.Config) connectionRecord {
	return connectionRecord{
		ID:       id,
		Name:     name,
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Password: cfg.Password,
		Database: cfg.Database,
	}
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "conn"
	}
	return hex.EncodeToString(b[:])
}
