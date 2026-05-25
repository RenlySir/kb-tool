package kbtool_test

import (
	"os"
	"strings"
	"testing"
)

func TestDockerPackagingFilesDescribeRunnableServer(t *testing.T) {
	dockerfile := readFile(t, "Dockerfile")
	for _, fragment := range []string{
		"FROM golang:1.24",
		"go build",
		"mkdir -p /workspace /data",
		"USER kbtool",
		"EXPOSE 8080",
		`ENTRYPOINT ["/usr/local/bin/kb-tool"]`,
	} {
		if !strings.Contains(dockerfile, fragment) {
			t.Fatalf("Dockerfile missing %q", fragment)
		}
	}

	compose := readFile(t, "docker-compose.yml")
	for _, fragment := range []string{
		"kb-tool:",
		"build:",
		"target: runtime",
		"8080:8080",
		"TIDB_HOST: tidb",
		"server",
		"-addr",
		"0.0.0.0:8080",
		"working_dir: /workspace",
		".:/workspace:ro",
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("docker-compose.yml missing %q", fragment)
		}
	}

	ignore := readFile(t, ".dockerignore")
	for _, fragment := range []string{".git", "kb-tool", ".worktrees"} {
		if !strings.Contains(ignore, fragment) {
			t.Fatalf(".dockerignore missing %q", fragment)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}
