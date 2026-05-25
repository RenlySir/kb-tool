package source

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	DefaultMaxFileBytes = 512 * 1024 * 1024
	DefaultMaxTextBytes = 2 * 1024 * 1024
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
	MimeType    string
	IsBinary    bool
	Asset       []byte
}

type Collector interface {
	Collect(ctx context.Context, input string) ([]Document, error)
}

type CollectorFunc func(ctx context.Context, input string) ([]Document, error)

func (f CollectorFunc) Collect(ctx context.Context, input string) ([]Document, error) {
	return f(ctx, input)
}

type FileCollectorOptions struct {
	MaxBytes      int64
	MaxTextBytes  int64
	IncludeBinary bool
}

type FileCollector struct {
	maxBytes      int64
	maxTextBytes  int64
	includeBinary bool
}

func NewFileCollector(opts FileCollectorOptions) *FileCollector {
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFileBytes
	}
	maxTextBytes := opts.MaxTextBytes
	if maxTextBytes <= 0 {
		maxTextBytes = DefaultMaxTextBytes
	}
	return &FileCollector{maxBytes: maxBytes, maxTextBytes: maxTextBytes, includeBinary: opts.IncludeBinary}
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

	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		rel = filepath.Base(path)
	}
	rel = filepath.ToSlash(rel)

	base := Document{
		SourceType: SourceTypeFile,
		SourceURI:  root,
		Path:       rel,
		Title:      filepath.Base(path),
		Language:   languageForPath(path),
		SizeBytes:  info.Size(),
	}

	if isOfficeOpenXMLPath(path) {
		content, digest, err := c.extractOfficeOpenXML(path)
		if err != nil {
			return Document{}, false, err
		}
		base.Content = content
		base.ContentHash = digest
		base.MimeType = mimeTypeForPath(path, nil)
		return base, true, nil
	}

	sample, err := readSample(path, 8192)
	if err != nil {
		return Document{}, false, err
	}
	mimeType := mimeTypeForPath(path, sample)
	base.MimeType = mimeType
	if isBinary(sample) {
		if (!c.includeBinary && !isMetadataOnlyAssetMime(mimeType)) || !isSupportedAssetMime(mimeType) {
			return Document{}, false, nil
		}
		digest, asset, err := c.collectAsset(path, mimeType)
		if err != nil {
			return Document{}, false, err
		}
		base.ContentHash = digest
		base.IsBinary = true
		base.Asset = asset
		return base, true, nil
	}
	if strings.HasPrefix(mimeType, "image/") {
		return Document{}, false, nil
	}
	content, digest, err := c.readTextContent(path)
	if err != nil {
		return Document{}, false, err
	}
	base.Content = content
	base.ContentHash = digest
	return base, true, nil
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
	return NewAutoCollectorWithOptions(FileCollectorOptions{})
}

func NewAutoCollectorWithOptions(opts FileCollectorOptions) *AutoCollector {
	files := NewFileCollector(opts)
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

func readSample(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (c *FileCollector) readTextContent(path string) (string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	hasher := sha256.New()
	limit := c.maxTextBytes
	var builder strings.Builder
	buf := make([]byte, 64*1024)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if _, err := hasher.Write(chunk); err != nil {
				return "", "", err
			}
			if limit > 0 {
				remaining := limit - int64(builder.Len())
				if remaining > 0 {
					if int64(len(chunk)) > remaining {
						chunk = chunk[:remaining]
					}
					builder.Write(chunk)
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", "", readErr
		}
	}
	return builder.String(), digestString(hasher), nil
}

func (c *FileCollector) collectAsset(path string, mimeType string) (string, []byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	hasher := sha256.New()
	if isMetadataOnlyAssetMime(mimeType) {
		if _, err := io.Copy(hasher, file); err != nil {
			return "", nil, err
		}
		return digestString(hasher), nil, nil
	}
	buf := make([]byte, 64*1024)
	out := make([]byte, 0)
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if _, err := hasher.Write(chunk); err != nil {
				return "", nil, err
			}
			out = append(out, chunk...)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", nil, readErr
		}
	}
	return digestString(hasher), out, nil
}

func (c *FileCollector) extractOfficeOpenXML(path string) (string, string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", "", err
	}
	defer reader.Close()

	hasher, err := hashFile(path)
	if err != nil {
		return "", "", err
	}
	var builder strings.Builder
	for _, file := range reader.File {
		if !shouldReadOfficeXML(path, file.Name) {
			continue
		}
		if int64(builder.Len()) >= c.maxTextBytes {
			break
		}
		if err := appendXMLText(&builder, file, c.maxTextBytes); err != nil {
			return "", "", err
		}
	}
	return strings.TrimSpace(builder.String()), hasher, nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return digestString(hasher), nil
}

func digestString(hasher hash.Hash) string {
	return hex.EncodeToString(hasher.Sum(nil))
}

func appendXMLText(builder *strings.Builder, file *zip.File, maxTextBytes int64) error {
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	decoder := xml.NewDecoder(rc)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		charData, ok := token.(xml.CharData)
		if !ok {
			continue
		}
		text := strings.TrimSpace(string(charData))
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			writeLimited(builder, " ", maxTextBytes)
		}
		writeLimited(builder, text, maxTextBytes)
		if int64(builder.Len()) >= maxTextBytes {
			return nil
		}
	}
}

func writeLimited(builder *strings.Builder, text string, maxTextBytes int64) {
	if maxTextBytes <= 0 {
		return
	}
	remaining := maxTextBytes - int64(builder.Len())
	if remaining <= 0 {
		return
	}
	if int64(len(text)) > remaining {
		text = text[:remaining]
	}
	builder.WriteString(text)
}

func isOfficeOpenXMLPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".docx", ".xlsx", ".pptx":
		return true
	default:
		return false
	}
}

func shouldReadOfficeXML(path string, name string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".docx":
		return name == "word/document.xml" || strings.HasPrefix(name, "word/header") || strings.HasPrefix(name, "word/footer")
	case ".xlsx":
		return name == "xl/sharedStrings.xml" || strings.HasPrefix(name, "xl/worksheets/sheet")
	case ".pptx":
		return strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml")
	default:
		return false
	}
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
	case ".docx", ".doc":
		return "word"
	case ".xlsx", ".xls":
		return "excel"
	case ".pptx", ".ppt":
		return "powerpoint"
	default:
		return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
}

func mimeTypeForPath(path string, data []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return "text/markdown; charset=utf-8"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".go":
		return "text/x-go; charset=utf-8"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".doc":
		return "application/msword"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	default:
		if utf8.Valid(data) {
			return "text/plain; charset=utf-8"
		}
		return "application/octet-stream"
	}
}

func isSupportedAssetMime(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp",
		"application/msword", "application/vnd.ms-excel", "application/vnd.ms-powerpoint",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return true
	default:
		return false
	}
}

func isMetadataOnlyAssetMime(mimeType string) bool {
	switch mimeType {
	case "application/msword", "application/vnd.ms-excel", "application/vnd.ms-powerpoint",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return true
	default:
		return false
	}
}
