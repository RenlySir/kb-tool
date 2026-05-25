package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/RenlySir/kb-tool/internal/ingest"
	"github.com/RenlySir/kb-tool/internal/source"
	"github.com/RenlySir/kb-tool/internal/store"
	"github.com/RenlySir/kb-tool/internal/tagger"
)

type config struct {
	Command string
	Input   string
	Query   string
	Limit   int
	TiDB    store.Config
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
	kbStore, err := store.Open(ctx, cfg.TiDB)
	if err != nil {
		return err
	}
	defer kbStore.Close()

	switch cfg.Command {
	case "migrate":
		if err := kbStore.Migrate(ctx); err != nil {
			return err
		}
		fmt.Println("migration complete")
		return nil
	case "ingest":
		service := ingest.NewService(source.NewAutoCollector(), tagger.NewRuleBasedTagger(), kbStore)
		result, err := service.Ingest(ctx, cfg.Input)
		if err != nil {
			return err
		}
		fmt.Printf("ingested %d documents with %d tag assignments\n", result.Documents, result.Tags)
		return nil
	case "search":
		results, err := kbStore.Search(ctx, cfg.Query, cfg.Limit)
		if err != nil {
			return err
		}
		for _, result := range results {
			fmt.Printf("[%d] %s %s tags=%s\n", result.ID, result.SourceType, result.Path, strings.Join(result.Tags, ","))
			fmt.Printf("    %s\n", strings.ReplaceAll(result.Snippet, "\n", " "))
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q", cfg.Command)
	}
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		TiDB:  store.DefaultConfig(),
		Limit: 20,
	}
	cfg.TiDB.Host = getenv("TIDB_HOST", cfg.TiDB.Host)
	cfg.TiDB.User = getenv("TIDB_USER", cfg.TiDB.User)
	cfg.TiDB.Password = getenv("TIDB_PASSWORD", cfg.TiDB.Password)
	cfg.TiDB.Database = getenv("TIDB_DATABASE", cfg.TiDB.Database)
	cfg.TiDB.Port = getenvInt("TIDB_PORT", cfg.TiDB.Port)

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

func usage(out *os.File) {
	fmt.Fprintln(out, `kb-tool ingests files and Git repositories into a TiDB-backed knowledge base.

Usage:
  kb-tool migrate [flags]
  kb-tool ingest [flags] <file|directory|github-url|gitlab-url>
  kb-tool search [flags] <query>

TiDB flags can also be set with TIDB_HOST, TIDB_PORT, TIDB_USER, TIDB_PASSWORD, and TIDB_DATABASE.`)
}
