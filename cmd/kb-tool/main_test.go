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
