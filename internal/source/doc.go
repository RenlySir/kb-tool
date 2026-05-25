package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type SourceType string

const (
	SourceTypeFile   SourceType = "file"
	SourceTypeGitHub SourceType = "github"
	SourceTypeGitLab SourceType = "gitlab"
	SourceTypeGit    SourceType = "git"
)

type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
	ProviderGit    Provider = "git"
)

type Document struct {
	SourceType  SourceType
	SourceURI   string
	Path        string
	Title       string
	Language    string
	Content     string
	ContentHash string
	SizeBytes   int64
}

type Collector interface {
	Collect(ctx context.Context, input string) ([]Document, error)
}

type CollectorFunc func(ctx context.Context, input string) ([]Document, error)

func (f CollectorFunc) Collect(ctx context.Context, input string) ([]Document, error) {
	return f(ctx, input)
}

type FileCollectorOptions struct {
	MaxBytes int64
}

type FileCollector struct {
	maxBytes int64
}

func NewFileCollector(opts FileCollectorOptions) *FileCollector {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	return &FileCollector{maxBytes: maxBytes}
}

func (c *FileCollector) Collect(path string) ([]Document, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		doc, ok, err := c.collectFile(path, path)
		if err != nil || !ok {
			return nil, err
		}
		return []Document{doc}, nil
	}

	var docs []Document
	err = filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) && current != path {
				return filepath.SkipDir
			}
			return nil
		}

		doc, ok, err := c.collectFile(path, current)
		if err != nil {
			return err
		}
		if ok {
			docs = append(docs, doc)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Path < docs[j].Path
	})
	return docs, nil
}

func (c *FileCollector) collectFile(root string, path string) (Document, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Document{}, false, err
	}
	if info.Size() > c.maxBytes {
		return Document{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, false, err
	}
	if isBinary(data) {
		return Document{}, false, nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		rel = filepath.Base(path)
	}
	rel = filepath.ToSlash(rel)
	hash := sha256.Sum256(data)
	return Document{
		SourceType:  SourceTypeFile,
		SourceURI:   root,
		Path:        rel,
		Title:       filepath.Base(path),
		Language:    languageForPath(path),
		Content:     string(data),
		ContentHash: hex.EncodeToString(hash[:]),
		SizeBytes:   info.Size(),
	}, true, nil
}

type FileCollectorFunc func(path string) ([]Document, error)

func (f FileCollectorFunc) Collect(path string) ([]Document, error) {
	return f(path)
}

type PathCollector interface {
	Collect(path string) ([]Document, error)
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type CommandRunnerFunc func(ctx context.Context, name string, args ...string) error

func (f CommandRunnerFunc) Run(ctx context.Context, name string, args ...string) error {
	return f(ctx, name, args...)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type GitRemote struct {
	Provider Provider
	Host     string
	Owner    string
	Name     string
	URL      string
}

func ParseGitRemote(raw string) (GitRemote, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return GitRemote{}, errors.New("git remote is empty")
	}

	if strings.HasPrefix(trimmed, "git@") {
		return parseSSHRemote(trimmed)
	}
	return parseURLRemote(trimmed)
}

type GitCollector struct {
	runner        CommandRunner
	fileCollector PathCollector
}

func NewGitCollector(runner CommandRunner, fileCollector PathCollector) *GitCollector {
	return &GitCollector{runner: runner, fileCollector: fileCollector}
}

func (c *GitCollector) Collect(ctx context.Context, remoteURL string) ([]Document, error) {
	if c.runner == nil {
		c.runner = ExecCommandRunner{}
	}
	if c.fileCollector == nil {
		c.fileCollector = NewFileCollector(FileCollectorOptions{})
	}

	remote, err := ParseGitRemote(remoteURL)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "kb-tool-repo-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	if err := c.runner.Run(ctx, "git", "clone", "--depth", "1", remoteURL, tmp); err != nil {
		return nil, err
	}
	docs, err := c.fileCollector.Collect(tmp)
	if err != nil {
		return nil, err
	}
	for i := range docs {
		docs[i].SourceURI = remoteURL
		switch remote.Provider {
		case ProviderGitHub:
			docs[i].SourceType = SourceTypeGitHub
		case ProviderGitLab:
			docs[i].SourceType = SourceTypeGitLab
		default:
			docs[i].SourceType = SourceTypeGit
		}
	}
	return docs, nil
}

type AutoCollector struct {
	files PathCollector
	git   Collector
}

func NewAutoCollector() *AutoCollector {
	files := NewFileCollector(FileCollectorOptions{})
	return &AutoCollector{
		files: files,
		git:   NewGitCollector(ExecCommandRunner{}, files),
	}
}

func (c *AutoCollector) Collect(ctx context.Context, input string) ([]Document, error) {
	if looksLikeGitRemote(input) {
		return c.git.Collect(ctx, input)
	}
	return c.files.Collect(input)
}

func parseSSHRemote(raw string) (GitRemote, error) {
	parts := strings.SplitN(strings.TrimPrefix(raw, "git@"), ":", 2)
	if len(parts) != 2 {
		return GitRemote{}, fmt.Errorf("invalid ssh git remote: %s", raw)
	}
	host := parts[0]
	path := strings.TrimSuffix(parts[1], ".git")
	owner, name, err := splitOwnerName(path)
	if err != nil {
		return GitRemote{}, err
	}
	return GitRemote{Provider: providerForHost(host), Host: host, Owner: owner, Name: name, URL: raw}, nil
}

func parseURLRemote(raw string) (GitRemote, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return GitRemote{}, fmt.Errorf("invalid git remote URL: %s", raw)
	}
	path := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
	owner, name, err := splitOwnerName(path)
	if err != nil {
		return GitRemote{}, err
	}
	return GitRemote{Provider: providerForHost(u.Host), Host: u.Host, Owner: owner, Name: name, URL: raw}, nil
}

func splitOwnerName(path string) (string, string, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("git remote path must include owner and repository: %s", path)
	}
	name := parts[len(parts)-1]
	owner := strings.Join(parts[:len(parts)-1], "/")
	if owner == "" || name == "" {
		return "", "", fmt.Errorf("git remote path must include owner and repository: %s", path)
	}
	return owner, name, nil
}

func providerForHost(host string) Provider {
	lower := strings.ToLower(host)
	switch {
	case lower == "github.com":
		return ProviderGitHub
	case strings.Contains(lower, "gitlab"):
		return ProviderGitLab
	default:
		return ProviderGit
	}
}

func looksLikeGitRemote(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	return strings.HasPrefix(lower, "git@") ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://")
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".idea", ".vscode", ".terraform", ".cache", "__pycache__", "node_modules", "vendor", "dist", "build", "target":
		return true
	default:
		return false
	}
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return true
	}
	sampleSize := len(data)
	if sampleSize > 8000 {
		sampleSize = 8000
	}
	sample := data[:sampleSize]
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	return false
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".md", ".markdown":
		return "markdown"
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".java":
		return "java"
	case ".sql":
		return "sql"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".txt":
		return "text"
	default:
		return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
}
