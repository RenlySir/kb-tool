package main

import (
	"strings"
	"testing"
)

func TestParseConfigDefaultsToLocalTiDB(t *testing.T) {
	cfg, err := parseConfig([]string{"ingest", "./docs"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	if cfg.TiDB.Host != "127.0.0.1" || cfg.TiDB.Port != 4000 || cfg.TiDB.User != "root" || cfg.TiDB.Database != "kb" {
		t.Fatalf("unexpected TiDB defaults: %#v", cfg.TiDB)
	}
	if cfg.Command != "ingest" || cfg.Input != "./docs" {
		t.Fatalf("unexpected command config: %#v", cfg)
	}
}

func TestParseConfigRequiresInputForIngest(t *testing.T) {
	_, err := parseConfig([]string{"ingest"})
	if err == nil {
		t.Fatal("expected missing input error")
	}
	if !strings.Contains(err.Error(), "input") {
		t.Fatalf("expected input error, got %v", err)
	}
}

func TestParseConfigSupportsServerCommand(t *testing.T) {
	cfg, err := parseConfig([]string{"server", "-addr", "127.0.0.1:9090", "-api-token", "secret", "-admin-user", "root", "-admin-password", "passw0rd"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	if cfg.Command != "server" {
		t.Fatalf("expected server command, got %q", cfg.Command)
	}
	if cfg.Addr != "127.0.0.1:9090" {
		t.Fatalf("unexpected addr: %q", cfg.Addr)
	}
	if cfg.APIToken != "secret" {
		t.Fatalf("unexpected api token: %q", cfg.APIToken)
	}
	if cfg.AdminUser != "root" || cfg.AdminPassword != "passw0rd" {
		t.Fatalf("unexpected admin credentials: %#v", cfg)
	}
}

func TestParseConfigSupportsConnectionsFile(t *testing.T) {
	t.Setenv("KB_TOOL_CONNECTIONS_FILE", "/tmp/from-env.json")

	cfg, err := parseConfig([]string{"server", "-connections-file", "/tmp/from-flag.json"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.ConnectionsFile != "/tmp/from-flag.json" {
		t.Fatalf("unexpected connections file: %q", cfg.ConnectionsFile)
	}
}

func TestParseConfigSupportsLargeFileOptions(t *testing.T) {
	cfg, err := parseConfig([]string{"ingest", "-max-file-bytes", "512MiB", "-max-text-bytes", "2MiB", "./docs"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	if cfg.File.MaxBytes != 512*1024*1024 {
		t.Fatalf("unexpected max file bytes: %d", cfg.File.MaxBytes)
	}
	if cfg.File.MaxTextBytes != 2*1024*1024 {
		t.Fatalf("unexpected max text bytes: %d", cfg.File.MaxTextBytes)
	}
}

func TestParseConfigReadsLargeFileOptionsFromEnv(t *testing.T) {
	t.Setenv("KB_TOOL_MAX_FILE_BYTES", "768MiB")
	t.Setenv("KB_TOOL_MAX_TEXT_BYTES", "3MiB")

	cfg, err := parseConfig([]string{"ingest", "./docs"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	if cfg.File.MaxBytes != 768*1024*1024 {
		t.Fatalf("unexpected max file bytes: %d", cfg.File.MaxBytes)
	}
	if cfg.File.MaxTextBytes != 3*1024*1024 {
		t.Fatalf("unexpected max text bytes: %d", cfg.File.MaxTextBytes)
	}
}

func TestParseConfigSupportsSplitterOptions(t *testing.T) {
	t.Setenv("KB_TOOL_CHUNK_SIZE", "256")
	t.Setenv("KB_TOOL_CHUNK_OVERLAP", "32")

	cfg, err := parseConfig([]string{"ingest", "-chunk-size", "128", "-chunk-overlap", "16", "./docs"})
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	if cfg.ChunkSize != 128 {
		t.Fatalf("unexpected chunk size: %d", cfg.ChunkSize)
	}
	if cfg.ChunkOverlap != 16 {
		t.Fatalf("unexpected chunk overlap: %d", cfg.ChunkOverlap)
	}
}
