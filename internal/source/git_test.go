package source_test

import (
	"context"
	"strings"
	"testing"

	"github.com/RenlySir/kb-tool/internal/source"
)

func TestParseGitRemoteRecognizesGitHubAndGitLabURLs(t *testing.T) {
	cases := []struct {
		raw      string
		provider source.Provider
		owner    string
		name     string
	}{
		{"https://github.com/RenlySir/kb-tool", source.ProviderGitHub, "RenlySir", "kb-tool"},
		{"https://github.com/RenlySir/kb-tool.git", source.ProviderGitHub, "RenlySir", "kb-tool"},
		{"git@gitlab.com:group/subgroup/project.git", source.ProviderGitLab, "group/subgroup", "project"},
		{"https://gitlab.example.com/platform/kb-tool.git", source.ProviderGitLab, "platform", "kb-tool"},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			remote, err := source.ParseGitRemote(tc.raw)
			if err != nil {
				t.Fatalf("ParseGitRemote returned error: %v", err)
			}
			if remote.Provider != tc.provider || remote.Owner != tc.owner || remote.Name != tc.name {
				t.Fatalf("unexpected remote: %#v", remote)
			}
		})
	}
}

func TestGitCollectorClonesRemoteThenCollectsFiles(t *testing.T) {
	var cloneArgs []string
	runner := source.CommandRunnerFunc(func(ctx context.Context, name string, args ...string) error {
		cloneArgs = append([]string{name}, args...)
		return nil
	})

	var collectedPath string
	fileCollector := source.FileCollectorFunc(func(path string) ([]source.Document, error) {
		collectedPath = path
		return []source.Document{{Path: "README.md", Content: "hello"}}, nil
	})

	collector := source.NewGitCollector(runner, fileCollector)
	docs, err := collector.Collect(context.Background(), "https://github.com/RenlySir/kb-tool.git")
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(docs) != 1 {
		t.Fatalf("expected one document, got %d", len(docs))
	}
	if strings.Join(cloneArgs, " ") != "git clone --depth 1 https://github.com/RenlySir/kb-tool.git "+collectedPath {
		t.Fatalf("unexpected clone args: %v, collected path: %s", cloneArgs, collectedPath)
	}
	if collectedPath == "" {
		t.Fatal("expected collector to receive cloned path")
	}
}
