# kb-tool

`kb-tool` is a Go command-line tool for building a small TiDB-backed knowledge base from local files, directories, GitHub repositories, and GitLab repositories. It collects text content, generates deterministic rule-based tags, stores documents and tag mappings in TiDB, and provides a CLI search command.

The current implementation is the first usable core. It is intentionally lightweight: TiDB is the storage backend, Git is used for repository ingestion, and the tagger is rule based so the tool works offline. The architecture leaves room for later Unstructured parsing, AI metadata extraction, embeddings, MinIO, Dify, Airweave, and MCP integrations.

## What It Does Today

- Ingest a single text file, a directory tree, a GitHub repository URL, or a GitLab repository URL.
- Shallow-clone Git repositories with `git clone --depth 1`.
- Skip binary files and noisy folders such as `.git`, `node_modules`, `vendor`, `dist`, `build`, and `target`.
- Infer a simple language value from file extension.
- Generate stable tags from language, path, title, and content keywords.
- Create the TiDB schema automatically.
- Upsert documents by `(content_hash, path)` to avoid duplicates on repeated ingestion.
- Search document title, path, and content from the CLI.

## Roadmap

Planned extensions are documented in [docs/operation-manual.md](docs/operation-manual.md):

- Unstructured-based parsing for PDF, Word, PPT, Markdown, TXT, images, and video-derived text.
- LangExtract-style structured metadata extraction for authors, dates, entities, and relations.
- Manual tags, rule tags, and LLM-generated AI tags with review status, confidence, and evidence.
- Chunking, embedding, and TiDB vector search.
- MinIO for raw object storage.
- Dify Knowledge Pipeline and Airweave adapters.
- Web UI for batch import, tag editing, knowledge-base browsing, and image preview.
- REST API for external systems to query and ingest knowledge.
- MCP server for AI agents.

## Requirements

- Go 1.24 or newer.
- Git CLI for GitHub/GitLab repository ingestion.
- TiDB reachable through the MySQL protocol.
- Docker Compose, only if you want to use the included local TiDB service.

## Quick Start

Start a local TiDB:

```bash
docker compose up -d tidb
```

Build the CLI:

```bash
go build ./cmd/kb-tool
```

Create the schema:

```bash
./kb-tool migrate
```

Ingest a local directory:

```bash
./kb-tool ingest ./docs
```

Ingest a GitHub repository:

```bash
./kb-tool ingest https://github.com/RenlySir/kb-tool.git
```

Search:

```bash
./kb-tool search tidb
```

## Configuration

The CLI reads TiDB settings from environment variables and command flags. Flags override environment variables.

| Setting | Environment Variable | Flag | Default |
| --- | --- | --- | --- |
| TiDB host | `TIDB_HOST` | `-tidb-host` | `127.0.0.1` |
| TiDB port | `TIDB_PORT` | `-tidb-port` | `4000` |
| TiDB user | `TIDB_USER` | `-tidb-user` | `root` |
| TiDB password | `TIDB_PASSWORD` | `-tidb-password` | empty |
| TiDB database | `TIDB_DATABASE` | `-tidb-database` | `kb` |
| Search limit | none | `-limit` | `20` |

Example:

```bash
TIDB_HOST=127.0.0.1 \
TIDB_PORT=4000 \
TIDB_USER=root \
TIDB_DATABASE=kb \
./kb-tool migrate
```

The same configuration with flags:

```bash
./kb-tool migrate \
  -tidb-host 127.0.0.1 \
  -tidb-port 4000 \
  -tidb-user root \
  -tidb-database kb
```

## Commands

### `migrate`

Creates the TiDB database if it does not exist, then creates the knowledge-base tables.

```bash
./kb-tool migrate
```

### `ingest`

Collects documents from a file, directory, GitHub URL, or GitLab URL, generates tags, and writes everything to TiDB.

```bash
./kb-tool ingest <file|directory|github-url|gitlab-url>
```

Examples:

```bash
./kb-tool ingest ./README.md
./kb-tool ingest ./docs
./kb-tool ingest https://github.com/RenlySir/kb-tool.git
./kb-tool ingest git@gitlab.com:group/project.git
```

### `search`

Searches title, path, and content with a SQL `LIKE` query and prints matching snippets.

```bash
./kb-tool search "knowledge base"
./kb-tool search -limit 5 tidb
```

### Planned `server`

The next major mode will expose the knowledge base through Web UI, REST API, and MCP from one process:

```bash
./kb-tool server -addr 127.0.0.1:8080
```

Planned access surfaces:

- Web UI: batch import local paths and Git repositories, inspect documents, edit tags, filter by tags, and preview images directly in the browser.
- REST API: external systems can ingest sources, list documents, search, read document detail, fetch image assets, and manage tags.
- MCP tools: AI agents can call `search_knowledge`, `get_document`, `list_documents`, `list_tags`, `ingest_source`, `generate_document_tags`, and `add_document_tags`.
- LLM tagger: connect to OpenAI-compatible APIs, Ollama, or an internal model gateway to generate tags from document text and metadata.

Planned LLM configuration:

```bash
KB_TOOL_LLM_PROVIDER=openai-compatible
KB_TOOL_LLM_BASE_URL=https://api.openai.com/v1
KB_TOOL_LLM_API_KEY=<token>
KB_TOOL_LLM_MODEL=gpt-4.1-mini
```

For local models:

```bash
KB_TOOL_LLM_PROVIDER=ollama
KB_TOOL_LLM_BASE_URL=http://127.0.0.1:11434
KB_TOOL_LLM_MODEL=qwen2.5:7b
```

AI tag output should be stored with source, confidence, and evidence so humans can review or reject model-generated labels.

Planned image behavior:

- Image files are stored as assets with `image/png`, `image/jpeg`, `image/gif`, or `image/webp` MIME types.
- Images are not forced into text fields.
- Browser preview uses an asset endpoint that returns the correct `Content-Type`, so images render visually instead of appearing as garbled binary text.

## Schema

The current version creates three tables:

- `kb_documents`: document metadata and full text.
- `kb_tags`: normalized tag names.
- `kb_document_tags`: many-to-many document/tag links.

Documents are upserted by `(content_hash, path)`, so repeated ingestion updates existing rows instead of duplicating unchanged files.

## Architecture

```text
Input source
  ├─ local file
  ├─ local directory
  ├─ GitHub repository
  └─ GitLab repository
        ↓
source collector
        ↓
rule-based tagger
        ↓
ingest service
        ↓
TiDB store
        ↓
CLI search
```

Planned server architecture:

```text
Browser Web UI ─┐
External REST ──┼─ kb-tool server ─ TiDB knowledge store
MCP clients  ───┘
                  ├─ ingest service
                  ├─ document/tag/search API
                  └─ asset endpoint for images and binary files
```

Main packages:

- `cmd/kb-tool`: CLI parsing and command execution.
- `internal/source`: local file collector, Git remote parser, and Git repository collector.
- `internal/tagger`: deterministic rule-based tag generator.
- `internal/ingest`: orchestration of collect, tag, migrate, and save.
- `internal/store`: TiDB schema, upsert, tag linking, and search.

Planned packages:

- `internal/llm`: provider abstraction for OpenAI-compatible APIs, Ollama, and internal model gateways.
- `internal/aitagger`: prompt construction, JSON tag parsing, confidence/evidence handling, and review-state defaults.

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build ./cmd/kb-tool
```

Format:

```bash
gofmt -w cmd internal
```

## Current Limits

- No PDF, Word, PPT, image, or video parsing yet.
- No chunking or embedding yet.
- No TiDB vector search yet.
- No MinIO object storage yet.
- No Web UI, REST API, or MCP server yet.
- No browser image preview endpoint yet.
- No LLM connection or AI tagger yet.
- No manual tag review workflow yet.
- Git repository ingestion uses a temporary shallow clone and requires local Git credentials for private repositories.

See [docs/operation-manual.md](docs/operation-manual.md) for operating steps, troubleshooting, and the recommended expansion plan.
