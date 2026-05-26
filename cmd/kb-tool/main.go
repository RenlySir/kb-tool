package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/RenlySir/kb-tool/internal/ingest"
	"github.com/RenlySir/kb-tool/internal/kb"
	"github.com/RenlySir/kb-tool/internal/source"
	"github.com/RenlySir/kb-tool/internal/splitter"
	"github.com/RenlySir/kb-tool/internal/store"
	"github.com/RenlySir/kb-tool/internal/tagger"
	"github.com/RenlySir/kb-tool/internal/web"
)

type config struct {
	Command         string
	Input           string
	Query           string
	Limit           int
	Addr            string
	APIToken        string
	AdminUser       string
	AdminPassword   string
	ConnectionsFile string
	ChunkSize       int
	ChunkOverlap    int
	TiDB            store.Config
	File            source.FileCollectorOptions
}

func main() {
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage(os.Stderr)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	switch cfg.Command {
	case "migrate":
		kbStore, err := store.Open(ctx, cfg.TiDB)
		if err != nil {
			return err
		}
		defer kbStore.Close()
		if err := kbStore.Migrate(ctx); err != nil {
			return err
		}
		fmt.Println("migration complete")
		return nil
	case "ingest":
		kbStore, err := store.Open(ctx, cfg.TiDB)
		if err != nil {
			return err
		}
		defer kbStore.Close()
		service := newIngestService(cfg, kbStore)
		result, err := service.Ingest(ctx, cfg.Input)
		if err != nil {
			return err
		}
		fmt.Printf("ingested %d documents with %d chunks and %d tag assignments\n", result.Documents, result.Chunks, result.Tags)
		return nil
	case "search":
		kbStore, err := store.Open(ctx, cfg.TiDB)
		if err != nil {
			return err
		}
		defer kbStore.Close()
		results, err := kbStore.Search(ctx, cfg.Query, cfg.Limit)
		if err != nil {
			return err
		}
		for _, result := range results {
			fmt.Printf("[%d] %s %s tags=%s\n", result.ID, result.SourceType, result.Path, strings.Join(result.Tags, ","))
			fmt.Printf("    %s\n", strings.ReplaceAll(result.Snippet, "\n", " "))
		}
		return nil
	case "server":
		manager, err := kb.NewManager(ctx, kb.Config{
			ConnectionsFile: cfg.ConnectionsFile,
			Default:         cfg.TiDB,
		})
		if err != nil {
			return err
		}
		defer manager.Close()
		service := newIngestService(cfg, manager)
		server := web.NewServer(web.Config{
			APIToken:      cfg.APIToken,
			AdminUser:     cfg.AdminUser,
			AdminPassword: cfg.AdminPassword,
		}, manager, web.IngesterFunc(func(ctx context.Context, input string) (web.IngestResult, error) {
			result, err := service.Ingest(ctx, input)
			return web.IngestResult{Documents: result.Documents, Tags: result.Tags}, err
		}), web.WithConnectionManager(manager))
		fmt.Printf("kb-tool web server listening on http://%s\n", cfg.Addr)
		return http.ListenAndServe(cfg.Addr, server)
	default:
		return fmt.Errorf("unknown command %q", cfg.Command)
	}
}

func newIngestService(cfg config, storage ingest.Store) *ingest.Service {
	return ingest.NewService(
		source.NewAutoCollectorWithOptions(cfg.File),
		tagger.NewRuleBasedTagger(),
		storage,
		ingest.WithSplitter(splitter.NewRecursive(splitter.Options{
			ChunkSize: cfg.ChunkSize,
			Overlap:   cfg.ChunkOverlap,
		})),
	)
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		TiDB:  store.DefaultConfig(),
		Limit: 20,
		Addr:  "127.0.0.1:8080",
	}
	cfg.TiDB.Host = getenv("TIDB_HOST", cfg.TiDB.Host)
	cfg.TiDB.User = getenv("TIDB_USER", cfg.TiDB.User)
	cfg.TiDB.Password = getenv("TIDB_PASSWORD", cfg.TiDB.Password)
	cfg.TiDB.Database = getenv("TIDB_DATABASE", cfg.TiDB.Database)
	cfg.TiDB.Port = getenvInt("TIDB_PORT", cfg.TiDB.Port)
	cfg.APIToken = getenv("KB_TOOL_API_TOKEN", cfg.APIToken)
	cfg.AdminUser = getenv("KB_TOOL_ADMIN_USER", "admin")
	cfg.AdminPassword = getenv("KB_TOOL_ADMIN_PASSWORD", "admin123")
	cfg.ConnectionsFile = getenv("KB_TOOL_CONNECTIONS_FILE", "data/connections.json")
	cfg.File.MaxBytes = getenvBytes("KB_TOOL_MAX_FILE_BYTES", source.DefaultMaxFileBytes)
	cfg.File.MaxTextBytes = getenvBytes("KB_TOOL_MAX_TEXT_BYTES", source.DefaultMaxTextBytes)
	cfg.ChunkSize = getenvInt("KB_TOOL_CHUNK_SIZE", 1024)
	cfg.ChunkOverlap = getenvInt("KB_TOOL_CHUNK_OVERLAP", 128)

	if len(args) == 0 {
		return cfg, errors.New("command is required")
	}
	cfg.Command = args[0]

	fs := flag.NewFlagSet(cfg.Command, flag.ContinueOnError)
	fs.StringVar(&cfg.TiDB.Host, "tidb-host", cfg.TiDB.Host, "TiDB host")
	fs.IntVar(&cfg.TiDB.Port, "tidb-port", cfg.TiDB.Port, "TiDB port")
	fs.StringVar(&cfg.TiDB.User, "tidb-user", cfg.TiDB.User, "TiDB user")
	fs.StringVar(&cfg.TiDB.Password, "tidb-password", cfg.TiDB.Password, "TiDB password")
	fs.StringVar(&cfg.TiDB.Database, "tidb-database", cfg.TiDB.Database, "TiDB database")
	fs.IntVar(&cfg.Limit, "limit", cfg.Limit, "search result limit")
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "server listen address")
	fs.StringVar(&cfg.APIToken, "api-token", cfg.APIToken, "API bearer token")
	fs.StringVar(&cfg.AdminUser, "admin-user", cfg.AdminUser, "web admin username")
	fs.StringVar(&cfg.AdminPassword, "admin-password", cfg.AdminPassword, "web admin password")
	fs.StringVar(&cfg.ConnectionsFile, "connections-file", cfg.ConnectionsFile, "web database connection config file")
	fs.IntVar(&cfg.ChunkSize, "chunk-size", cfg.ChunkSize, "text chunk size used before future embedding")
	fs.IntVar(&cfg.ChunkOverlap, "chunk-overlap", cfg.ChunkOverlap, "text chunk overlap used before future embedding")
	fs.Func("max-file-bytes", "maximum file size to collect, for example 512MiB or 536870912", func(value string) error {
		parsed, err := parseBytes(value)
		if err != nil {
			return err
		}
		cfg.File.MaxBytes = parsed
		return nil
	})
	fs.Func("max-text-bytes", "maximum extracted text bytes stored per document, for example 2MiB", func(value string) error {
		parsed, err := parseBytes(value)
		if err != nil {
			return err
		}
		cfg.File.MaxTextBytes = parsed
		return nil
	})
	if err := fs.Parse(args[1:]); err != nil {
		return cfg, err
	}

	switch cfg.Command {
	case "ingest":
		if fs.NArg() < 1 {
			return cfg, errors.New("input is required for ingest")
		}
		cfg.Input = fs.Arg(0)
	case "search":
		if fs.NArg() < 1 {
			return cfg, errors.New("query is required for search")
		}
		cfg.Query = fs.Arg(0)
	case "migrate":
		if fs.NArg() > 0 {
			return cfg, errors.New("migrate does not accept positional arguments")
		}
	case "server":
		if fs.NArg() > 0 {
			return cfg, errors.New("server does not accept positional arguments")
		}
		if bindsExternally(cfg.Addr) && cfg.APIToken == "" {
			return cfg, errors.New("api-token is required when binding server to a non-local address")
		}
	default:
		return cfg, fmt.Errorf("unknown command %q", cfg.Command)
	}
	return cfg, nil
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBytes(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := parseBytes(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseBytes(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, errors.New("byte size is empty")
	}
	lower := strings.ToLower(value)
	multipliers := []struct {
		suffix     string
		multiplier int64
	}{
		{"mib", 1024 * 1024},
		{"mb", 1000 * 1000},
		{"kib", 1024},
		{"kb", 1000},
		{"gib", 1024 * 1024 * 1024},
		{"gb", 1000 * 1000 * 1000},
		{"b", 1},
	}
	multiplier := int64(1)
	for _, item := range multipliers {
		if strings.HasSuffix(lower, item.suffix) {
			multiplier = item.multiplier
			lower = strings.TrimSpace(strings.TrimSuffix(lower, item.suffix))
			break
		}
	}
	parsed, err := strconv.ParseInt(lower, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid byte size %q", raw)
	}
	if parsed > (1<<63-1)/multiplier {
		return 0, fmt.Errorf("byte size %q is too large", raw)
	}
	return parsed * multiplier, nil
}

func usage(out *os.File) {
	fmt.Fprintln(out, `kb-tool ingests files and Git repositories into a TiDB-backed knowledge base.

Usage:
  kb-tool migrate [flags]
  kb-tool ingest [flags] <file|directory|github-url|gitlab-url>
  kb-tool search [flags] <query>
  kb-tool server [flags]

TiDB flags can also be set with TIDB_HOST, TIDB_PORT, TIDB_USER, TIDB_PASSWORD, and TIDB_DATABASE.
File limits can also be set with KB_TOOL_MAX_FILE_BYTES and KB_TOOL_MAX_TEXT_BYTES.
Chunking can also be set with KB_TOOL_CHUNK_SIZE and KB_TOOL_CHUNK_OVERLAP.`)
}

func bindsExternally(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return host == "" || host == "0.0.0.0" || host == "::"
}
